package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/settlement"
	settlementpostgres "github.com/fanryan/paycore/internal/settlement/adapters/postgres"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryCreatesGetsAndUpdatesBatch(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := settlementpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupSettlementRows(t, pool, prefix)
	})

	batch := testBatch(t, prefix+"-batch-1")

	created, err := store.CreateBatch(ctx, batch)
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}

	if created.ID != batch.ID {
		t.Fatalf("expected batch id %q, got %q", batch.ID, created.ID)
	}

	loaded, err := store.GetBatch(ctx, batch.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}

	if loaded.Status != settlement.BatchStatusCreated {
		t.Fatalf("expected CREATED, got %q", loaded.Status)
	}

	processing, err := loaded.StartProcessing("worker-1", testNow().Add(time.Minute), testNow())
	if err != nil {
		t.Fatalf("start processing: %v", err)
	}

	updated, err := store.UpdateBatch(ctx, processing)
	if err != nil {
		t.Fatalf("update batch: %v", err)
	}

	if updated.Status != settlement.BatchStatusProcessing {
		t.Fatalf("expected PROCESSING, got %q", updated.Status)
	}

	if updated.ClaimedBy == nil || *updated.ClaimedBy != "worker-1" {
		t.Fatalf("expected claimed_by worker-1, got %v", updated.ClaimedBy)
	}
}

func TestRepositoryRejectsDuplicateBatch(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := settlementpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupSettlementRows(t, pool, prefix)
	})

	batch := testBatch(t, prefix+"-batch-1")
	if _, err := store.CreateBatch(ctx, batch); err != nil {
		t.Fatalf("create batch: %v", err)
	}

	_, err := store.CreateBatch(ctx, batch)
	if !errors.Is(err, settlement.ErrDuplicateBatch) {
		t.Fatalf("expected ErrDuplicateBatch, got %v", err)
	}
}

func TestRepositoryCreatesAndListsLineItems(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := settlementpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupSettlementRows(t, pool, prefix)
	})

	batch := createBatch(t, store, prefix+"-batch-1")
	createPaymentFixture(t, pool, prefix+"-merchant-1", prefix+"-payer-1", prefix+"-payment-1")
	createPaymentFixture(t, pool, prefix+"-merchant-1", prefix+"-payer-2", prefix+"-payment-2")

	first := testLineItem(t, prefix+"-item-1", batch.ID, prefix+"-merchant-1", prefix+"-payment-1")
	second := testLineItem(t, prefix+"-item-2", batch.ID, prefix+"-merchant-1", prefix+"-payment-2")

	if _, err := store.CreateLineItem(ctx, second); err != nil {
		t.Fatalf("create second line item: %v", err)
	}
	if _, err := store.CreateLineItem(ctx, first); err != nil {
		t.Fatalf("create first line item: %v", err)
	}

	items, err := store.ListLineItems(ctx, batch.ID)
	if err != nil {
		t.Fatalf("list line items: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 line items, got %d", len(items))
	}

	if items[0].ID != first.ID {
		t.Fatalf("expected first item %q, got %q", first.ID, items[0].ID)
	}

	if items[1].ID != second.ID {
		t.Fatalf("expected second item %q, got %q", second.ID, items[1].ID)
	}
}

func TestRepositoryRejectsDuplicateLineItemForPayment(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := settlementpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupSettlementRows(t, pool, prefix)
	})

	batch := createBatch(t, store, prefix+"-batch-1")
	createPaymentFixture(t, pool, prefix+"-merchant-1", prefix+"-payer-1", prefix+"-payment-1")

	first := testLineItem(t, prefix+"-item-1", batch.ID, prefix+"-merchant-1", prefix+"-payment-1")
	if _, err := store.CreateLineItem(ctx, first); err != nil {
		t.Fatalf("create first line item: %v", err)
	}

	duplicate := testLineItem(t, prefix+"-item-2", batch.ID, prefix+"-merchant-1", prefix+"-payment-1")
	_, err := store.CreateLineItem(ctx, duplicate)
	if !errors.Is(err, settlement.ErrDuplicateLineItem) {
		t.Fatalf("expected ErrDuplicateLineItem, got %v", err)
	}
}

func TestRepositoryCreateBatchUsesContextTransaction(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := settlementpostgres.NewStore(pool)
	transactor := db.NewPostgresTransactor(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupSettlementRows(t, pool, prefix)
	})

	expectedErr := errors.New("force rollback")
	batch := testBatch(t, prefix+"-batch-1")

	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := store.CreateBatch(ctx, batch); err != nil {
			return err
		}

		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected rollback error %v, got %v", expectedErr, err)
	}

	_, err = store.GetBatch(ctx, batch.ID)
	if !errors.Is(err, settlement.ErrBatchNotFound) {
		t.Fatalf("expected ErrBatchNotFound after rollback, got %v", err)
	}
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("PAYCORE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PAYCORE_DATABASE_URL is not set")
	}

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

func createBatch(t *testing.T, store *settlementpostgres.Store, batchID string) settlement.Batch {
	t.Helper()

	batch := testBatch(t, batchID)
	created, err := store.CreateBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("create batch fixture: %v", err)
	}

	return created
}

func testBatch(t *testing.T, batchID string) settlement.Batch {
	t.Helper()

	batch, err := settlement.NewBatch(settlement.NewBatchInput{
		ID:          batchID,
		WindowStart: testNow().Add(-time.Hour),
		WindowEnd:   testNow(),
		Now:         testNow(),
	})
	if err != nil {
		t.Fatalf("new batch: %v", err)
	}

	return batch
}

func testLineItem(t *testing.T, itemID string, batchID string, merchantID string, paymentID string) settlement.LineItem {
	t.Helper()

	item, err := settlement.NewLineItem(settlement.NewLineItemInput{
		ID:              itemID,
		BatchID:         batchID,
		MerchantID:      merchantID,
		PaymentID:       paymentID,
		AmountMinor:     10000,
		FeeAmountMinor:  250,
		Currency:        "USD",
		PaymentCaptured: testNow().Add(-time.Minute),
		Now:             testNow(),
	})
	if err != nil {
		t.Fatalf("new line item: %v", err)
	}

	return item
}

func createPaymentFixture(t *testing.T, pool *pgxpool.Pool, merchantID string, payerID string, paymentID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := testNow()
	if _, err := pool.Exec(ctx, `
		INSERT INTO merchants (id, name, status, settlement_currency, created_at, updated_at)
		VALUES ($1, $2, 'ACTIVE', 'USD', $3, $3)
		ON CONFLICT (id) DO NOTHING
	`, merchantID, merchantID, now); err != nil {
		t.Fatalf("create merchant fixture: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO payers (id, available_balance_minor, held_balance_minor, currency, version, created_at, updated_at)
		VALUES ($1, 0, 0, 'USD', 0, $2, $2)
		ON CONFLICT (id) DO NOTHING
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

func cleanupSettlementRows(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	likePrefix := prefix + "%"
	statements := []string{
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

func testPrefix() string {
	return fmt.Sprintf("settlement-postgres-%d", time.Now().UnixNano())
}

func testNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}
