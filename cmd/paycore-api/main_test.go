package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	httpapi "github.com/fanryan/paycore/internal/http"
	"github.com/fanryan/paycore/internal/idempotency"
	"github.com/fanryan/paycore/internal/merchant"
	"github.com/fanryan/paycore/internal/payer"
	"github.com/fanryan/paycore/internal/payment"
	"github.com/fanryan/paycore/internal/shared/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresBackendSupportsHTTPPaymentLifecycle(t *testing.T) {
	databaseURL := os.Getenv("PAYCORE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PAYCORE_DATABASE_URL is not set")
	}

	ctx := context.Background()
	prefix := fmt.Sprintf("smoke-%d", time.Now().UnixNano())

	cleanupPostgresSmokeRows(t, databaseURL, prefix)
	t.Cleanup(func() {
		cleanupPostgresSmokeRows(t, databaseURL, prefix)
	})

	repositories, err := newRepositories(ctx, config.Config{
		RepositoryBackend: "postgres",
		DatabaseURL:       databaseURL,
	})
	if err != nil {
		t.Fatalf("new repositories: %v", err)
	}
	defer repositories.close()

	router := newTestRouter(repositories)
	merchantID := prefix + "-merchant-1"
	payerID := prefix + "-payer-1"

	postJSON(t, router, "/merchants", "", map[string]any{
		"id":                  merchantID,
		"name":                "Smoke Merchant",
		"settlement_currency": "USD",
	}, http.StatusCreated)

	postJSON(t, router, "/payers", "", map[string]any{
		"id":                      payerID,
		"available_balance_minor": int64(10000),
		"currency":                "USD",
	}, http.StatusCreated)

	authorizeBody := postJSON(t, router, "/payments/authorize", prefix+"-authorize", map[string]any{
		"merchant_id": merchantID,
		"payer_id":    payerID,
		"amount":      int64(4000),
		"currency":    "USD",
	}, http.StatusCreated)

	var authorizeResponse payment.AuthorizePaymentResponse
	if err := json.Unmarshal(authorizeBody, &authorizeResponse); err != nil {
		t.Fatalf("decode authorize response: %v", err)
	}

	if authorizeResponse.Status != string(payment.StatusAuthorized) {
		t.Fatalf("expected authorized payment, got %q", authorizeResponse.Status)
	}

	captureBody := postJSON(t, router, "/payments/"+authorizeResponse.PaymentID+"/capture", prefix+"-capture", map[string]any{}, http.StatusOK)

	var captureResponse payment.CapturePaymentResponse
	if err := json.Unmarshal(captureBody, &captureResponse); err != nil {
		t.Fatalf("decode capture response: %v", err)
	}

	if captureResponse.Status != string(payment.StatusCaptured) {
		t.Fatalf("expected captured payment, got %q", captureResponse.Status)
	}

	repositoriesAgain, err := newRepositories(ctx, config.Config{
		RepositoryBackend: "postgres",
		DatabaseURL:       databaseURL,
	})
	if err != nil {
		t.Fatalf("new repositories again: %v", err)
	}
	defer repositoriesAgain.close()

	storedPayment, err := repositoriesAgain.payments.GetPayment(ctx, authorizeResponse.PaymentID)
	if err != nil {
		t.Fatalf("get stored payment: %v", err)
	}

	if storedPayment.Status != payment.StatusCaptured {
		t.Fatalf("expected persisted captured payment, got %q", storedPayment.Status)
	}
}

func newTestRouter(repositories repositories) http.Handler {
	merchantService := merchant.NewMerchantService(repositories.merchants)
	payerService := payer.NewPayerService(repositories.payers)
	paymentService := payment.NewService(repositories.merchants, repositories.payers, repositories.payments)
	idempotencyService := idempotency.NewService(repositories.idempotency, 24*time.Hour)

	return httpapi.NewRouter(httpapi.RouterConfig{
		ServiceName:     serviceName,
		Version:         version,
		StartedAt:       time.Now().UTC(),
		Logger:          slog.New(slog.NewTextHandler(os.Stderr, nil)),
		MerchantHandler: merchant.NewHandler(merchantService),
		PayerHandler:    payer.NewHandler(payerService),
		PaymentHandler:  payment.NewHandlerWithIdempotency(paymentService, idempotencyService),
	})
}

func postJSON(t *testing.T, handler http.Handler, path string, idempotencyKey string, payload map[string]any, expectedStatus int) []byte {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != expectedStatus {
		t.Fatalf("POST %s: expected status %d, got %d with body %s", path, expectedStatus, response.Code, response.Body.String())
	}

	return response.Body.Bytes()
}

func cleanupPostgresSmokeRows(t *testing.T, databaseURL string, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create cleanup pool: %v", err)
	}
	defer pool.Close()

	likePrefix := prefix + "%"
	statements := []string{
		"DELETE FROM payment_holds WHERE id LIKE $1 OR payment_id LIKE $1",
		"DELETE FROM payments WHERE id LIKE $1",
		"DELETE FROM idempotency_records WHERE key LIKE $1",
		"DELETE FROM payers WHERE id LIKE $1",
		"DELETE FROM merchants WHERE id LIKE $1",
	}

	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, likePrefix); err != nil {
			t.Fatalf("cleanup %q: %v", strings.TrimSpace(statement), err)
		}
	}
}
