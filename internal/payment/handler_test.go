package payment_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fanryan/paycore/internal/merchant"
	merchantmemory "github.com/fanryan/paycore/internal/merchant/adapters/memory"
	"github.com/fanryan/paycore/internal/payer"
	payermemory "github.com/fanryan/paycore/internal/payer/adapters/memory"
	"github.com/fanryan/paycore/internal/payment"
	paymentmemory "github.com/fanryan/paycore/internal/payment/adapters/memory"
)

func TestHandlerAuthorizesPayment(t *testing.T) {
	fixture := newHandlerFixture(t)

	request := httptest.NewRequest(http.MethodPost, "/payments/authorize", bytes.NewBufferString(`{
		"merchant_id": "merchant-1",
		"payer_id": "payer-1",
		"amount": 4000,
		"currency": "usd"
	}`))
	response := httptest.NewRecorder()

	fixture.handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, response.Code)
	}

	assertJSONContentType(t, response)

	var body payment.AuthorizePaymentResponse
	decodeJSON(t, response, &body)

	if body.Status != "AUTHORIZED" {
		t.Fatalf("expected status AUTHORIZED, got %q", body.Status)
	}

	if body.MerchantID != "merchant-1" {
		t.Fatalf("expected merchant id merchant-1, got %q", body.MerchantID)
	}

	if body.PayerID != "payer-1" {
		t.Fatalf("expected payer id payer-1, got %q", body.PayerID)
	}

	if body.AmountMinor != 4_000 {
		t.Fatalf("expected amount 4000, got %d", body.AmountMinor)
	}

	if body.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", body.Currency)
	}

	if body.PaymentID == "" {
		t.Fatal("expected payment id")
	}

	if body.HoldID == "" {
		t.Fatal("expected hold id")
	}
}

func TestHandlerRejectsInvalidJSON(t *testing.T) {
	fixture := newHandlerFixture(t)

	request := httptest.NewRequest(http.MethodPost, "/payments/authorize", bytes.NewBufferString(`{`))
	response := httptest.NewRecorder()

	fixture.handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}

	var body map[string]string
	decodeJSON(t, response, &body)

	if body["error_code"] != "INVALID_JSON" {
		t.Fatalf("expected INVALID_JSON, got %q", body["error_code"])
	}
}

func TestHandlerRejectsMissingMerchant(t *testing.T) {
	fixture := newHandlerFixture(t)

	request := httptest.NewRequest(http.MethodPost, "/payments/authorize", bytes.NewBufferString(`{
		"merchant_id": "missing",
		"payer_id": "payer-1",
		"amount": 4000,
		"currency": "USD"
	}`))
	response := httptest.NewRecorder()

	fixture.handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}

	var body map[string]string
	decodeJSON(t, response, &body)

	if body["error_code"] != "MERCHANT_NOT_FOUND" {
		t.Fatalf("expected MERCHANT_NOT_FOUND, got %q", body["error_code"])
	}
}

func TestHandlerRejectsInsufficientBalance(t *testing.T) {
	fixture := newHandlerFixture(t)

	request := httptest.NewRequest(http.MethodPost, "/payments/authorize", bytes.NewBufferString(`{
		"merchant_id": "merchant-1",
		"payer_id": "payer-1",
		"amount": 10001,
		"currency": "USD"
	}`))
	response := httptest.NewRecorder()

	fixture.handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, response.Code)
	}

	var body map[string]string
	decodeJSON(t, response, &body)

	if body["error_code"] != "INSUFFICIENT_AVAILABLE_BALANCE" {
		t.Fatalf("expected INSUFFICIENT_AVAILABLE_BALANCE, got %q", body["error_code"])
	}
}

func TestHandlerRejectsUnsupportedMethod(t *testing.T) {
	fixture := newHandlerFixture(t)

	request := httptest.NewRequest(http.MethodGet, "/payments/authorize", nil)
	response := httptest.NewRecorder()

	fixture.handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, response.Code)
	}
}

type handlerFixture struct {
	handler *payment.Handler
}

func newHandlerFixture(t *testing.T) handlerFixture {
	t.Helper()

	merchantRepository := merchantmemory.NewStore()
	payerRepository := payermemory.NewStore()
	paymentRepository := paymentmemory.NewStore()

	merchantRecord, err := merchant.NewMerchant("merchant-1", "Demo Merchant", "USD", testNow())
	if err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	if _, err := merchantRepository.CreateMerchant(context.Background(), merchantRecord); err != nil {
		t.Fatalf("failed to save merchant: %v", err)
	}

	payerRecord, err := payer.NewPayer("payer-1", 10_000, "USD", testNow())
	if err != nil {
		t.Fatalf("failed to create payer: %v", err)
	}

	if _, err := payerRepository.CreatePayer(context.Background(), payerRecord); err != nil {
		t.Fatalf("failed to save payer: %v", err)
	}

	service := payment.NewService(merchantRepository, payerRepository, paymentRepository)

	return handlerFixture{
		handler: payment.NewHandler(service),
	}
}

func decodeJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()

	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
}

func assertJSONContentType(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()

	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", got)
	}
}
