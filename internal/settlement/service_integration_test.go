package settlement_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/outbox"
	outboxpostgres "github.com/fanryan/paycore/internal/outbox/adapters/postgres"
	"github.com/fanryan/paycore/internal/payment"
	paymentpostgres "github.com/fanryan/paycore/internal/payment/adapters/postgres"
	"github.com/fanryan/paycore/internal/settlement"
	settlementpostgres "github.com/fanryan/paycore/internal/settlement/adapters/postgres"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestServiceSettlesClaimedPaymentsAndWritesOutboxEvents(t *testing.T) {
	databaseURL := os.Getenv("PAYCORE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PAYCORE_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool := newSettlementTestPool(t, databaseURL)
	prefix := fmt.Sprintf("settlement-service-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupSettlementIntegrationRows(t, pool, prefix)
	})

	merchantID := prefix + "-merchant-1"
	payerID := prefix + "-payer-1"
	paymentID := prefix + "-payment-1"
	createCapturedPaymentFixture(t, pool, merchantID, payerID, paymentID)

	service, err := settlement.NewService(settlement.ServiceConfig{
		Repository: settlementpostgres.NewStore(pool),
		Payments:   paymentpostgres.NewStore(pool),
		Outbox:     outboxpostgres.NewStore(pool),
		Transactor: db.NewPostgresTransactor(pool),
		WorkerID:   "integration-worker",
		ClaimLimit: 10,
		LockTTL:    time.Minute,
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("new settlement service: %v", err)
	}

	result, err := service.CreateBatch(ctx, settlement.CreateBatchInput{
		WindowStart: testNow().Add(-time.Hour),
		WindowEnd:   testNow().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create settlement batch: %v", err)
	}

	if result.Batch.Status != settlement.BatchStatusCompleted {
		t.Fatalf("expected completed batch, got %q", result.Batch.Status)
	}

	if len(result.LineItems) != 1 {
		t.Fatalf("expected one line item, got %d", len(result.LineItems))
	}

	if len(result.Payments) != 1 {
		t.Fatalf("expected one settled payment, got %d", len(result.Payments))
	}

	storedPayment := getPaymentStatusAndSettlementBatch(t, pool, paymentID)
	if storedPayment.status != payment.StatusSettled {
		t.Fatalf("expected payment SETTLED, got %q", storedPayment.status)
	}

	if storedPayment.settlementBatchID == nil || *storedPayment.settlementBatchID != result.Batch.ID {
		t.Fatalf("expected payment settlement batch %q, got %v", result.Batch.ID, storedPayment.settlementBatchID)
	}

	assertSettlementOutboxEvent(t, pool, paymentID, "payment.settled")
}

func newSettlementTestPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create postgres pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping postgres: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

func createCapturedPaymentFixture(t *testing.T, pool *pgxpool.Pool, merchantID string, payerID string, paymentID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := testNow()
	if _, err := pool.Exec(ctx, `
		INSERT INTO merchants (id, name, status, settlement_currency, created_at, updated_at)
		VALUES ($1, $2, 'ACTIVE', 'USD', $3, $3)
	`, merchantID, merchantID, now); err != nil {
		t.Fatalf("create merchant fixture: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO payers (id, available_balance_minor, held_balance_minor, currency, version, created_at, updated_at)
		VALUES ($1, 0, 0, 'USD', 0, $2, $2)
	`, payerID, now); err != nil {
		t.Fatalf("create payer fixture: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO payments (
			id,
			merchant_id,
			payer_id,
			amount_minor,
			currency,
			status,
			authorization_hold_id,
			authorized_at,
			expires_at,
			captured_at,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, 10000, 'USD', 'CAPTURED', $4, $5, $6, $7, $5, $5)
	`, paymentID, merchantID, payerID, paymentID+"-hold", now, now.Add(time.Hour), now.Add(30*time.Minute)); err != nil {
		t.Fatalf("create payment fixture: %v", err)
	}
}

type storedPaymentSettlement struct {
	status            payment.Status
	settlementBatchID *string
}

func getPaymentStatusAndSettlementBatch(t *testing.T, pool *pgxpool.Pool, paymentID string) storedPaymentSettlement {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stored storedPaymentSettlement
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT status, settlement_batch_id
		FROM payments
		WHERE id = $1
	`, paymentID).Scan(&status, &stored.settlementBatchID); err != nil {
		t.Fatalf("query payment: %v", err)
	}

	stored.status = payment.Status(status)
	return stored
}

func assertSettlementOutboxEvent(t *testing.T, pool *pgxpool.Pool, paymentID string, eventType string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM outbox_events
		WHERE aggregate_type = $1
		AND aggregate_id = $2
		AND event_type = $3
		AND status = $4
	`, "payment", paymentID, eventType, string(outbox.StatusPending)).Scan(&count); err != nil {
		t.Fatalf("query outbox event: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one %s outbox event for payment %q, got %d", eventType, paymentID, count)
	}
}

func cleanupSettlementIntegrationRows(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	likePrefix := prefix + "%"
	statements := []string{
		"DELETE FROM outbox_events WHERE aggregate_id LIKE $1",
		"DELETE FROM settlement_line_items WHERE settlement_batch_id LIKE $1 OR payment_id LIKE $1",
		"UPDATE payments SET settlement_batch_id = NULL WHERE settlement_batch_id LIKE $1",
		"DELETE FROM settlement_batches WHERE id LIKE $1",
		"DELETE FROM payment_holds WHERE payment_id LIKE $1",
		"DELETE FROM payments WHERE id LIKE $1",
		"DELETE FROM payers WHERE id LIKE $1",
		"DELETE FROM merchants WHERE id LIKE $1",
	}

	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, likePrefix); err != nil {
			t.Fatalf("cleanup statement %q: %v", statement, err)
		}
	}
}
