package http

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/merchant"
	merchantmemory "github.com/fanryan/paycore/internal/merchant/adapters/memory"
	"github.com/fanryan/paycore/internal/payer"
	payermemory "github.com/fanryan/paycore/internal/payer/adapters/memory"
	"github.com/fanryan/paycore/internal/payment"
	paymentmemory "github.com/fanryan/paycore/internal/payment/adapters/memory"
)

func TestHealthEndpoint(t *testing.T) {
	router := newTestRouter()

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	assertHeaderExists(t, response, "X-Request-ID")
	assertJSONContentType(t, response)

	var body map[string]string
	decodeJSON(t, response, &body)

	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}
}

func TestReadyEndpoint(t *testing.T) {
	router := newTestRouter()

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var body map[string]string
	decodeJSON(t, response, &body)

	if body["status"] != "ready" {
		t.Fatalf("expected status ready, got %q", body["status"])
	}
}

func TestVersionEndpoint(t *testing.T) {
	startedAt := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	router := NewRouter(RouterConfig{
		ServiceName: "paycore-api",
		Version:     "test-version",
		StartedAt:   startedAt,
		Logger:      slog.Default(),
	})

	request := httptest.NewRequest(http.MethodGet, "/version", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var body map[string]any
	decodeJSON(t, response, &body)

	if body["service"] != "paycore-api" {
		t.Fatalf("expected service paycore-api, got %q", body["service"])
	}

	if body["version"] != "test-version" {
		t.Fatalf("expected version test-version, got %q", body["version"])
	}

	if body["started_at"] != "2026-06-06T12:00:00Z" {
		t.Fatalf("expected started_at 2026-06-06T12:00:00Z, got %q", body["started_at"])
	}
}

func TestNotFoundEndpointReturnsJSONError(t *testing.T) {
	router := newTestRouter()

	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}

	assertJSONContentType(t, response)

	var body ErrorResponse
	decodeJSON(t, response, &body)

	if body.ErrorCode != "ROUTE_NOT_FOUND" {
		t.Fatalf("expected error code ROUTE_NOT_FOUND, got %q", body.ErrorCode)
	}

	if body.Message == "" {
		t.Fatal("expected error message")
	}

	if body.RequestID == "" {
		t.Fatal("expected request id")
	}

	if body.Timestamp == "" {
		t.Fatal("expected timestamp")
	}
}

func TestRequestIDHeaderIsPreserved(t *testing.T) {
	router := newTestRouter()

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", "test-request-id")

	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if got := response.Header().Get("X-Request-ID"); got != "test-request-id" {
		t.Fatalf("expected X-Request-ID to be preserved, got %q", got)
	}
}

func TestMerchantRouteIsWired(t *testing.T) {
	merchantRepository := merchantmemory.NewStore()
	merchantService := merchant.NewMerchantService(merchantRepository)
	merchantHandler := merchant.NewHandler(merchantService)

	router := NewRouter(RouterConfig{
		ServiceName:     "paycore-api",
		Version:         "test",
		StartedAt:       time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		Logger:          slog.Default(),
		MerchantHandler: merchantHandler,
	})

	request := httptest.NewRequest(http.MethodPost, "/merchants", bytes.NewBufferString(`{
		"id": "merchant-1",
		"name": "Demo Merchant",
		"settlement_currency": "usd"
	}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, response.Code)
	}

	assertHeaderExists(t, response, "X-Request-ID")
	assertJSONContentType(t, response)

	var body merchant.MerchantResponse
	decodeJSON(t, response, &body)

	if body.ID != "merchant-1" {
		t.Fatalf("expected merchant id merchant-1, got %q", body.ID)
	}

	if body.SettlementCurrency != "USD" {
		t.Fatalf("expected settlement currency USD, got %q", body.SettlementCurrency)
	}
}

func TestPayerRouteIsWired(t *testing.T) {
	payerRepository := payermemory.NewStore()
	payerService := payer.NewPayerService(payerRepository)
	payerHandler := payer.NewHandler(payerService)

	router := NewRouter(RouterConfig{
		ServiceName:  "paycore-api",
		Version:      "test",
		StartedAt:    time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		Logger:       slog.Default(),
		PayerHandler: payerHandler,
	})

	request := httptest.NewRequest(http.MethodPost, "/payers", bytes.NewBufferString(`{
		"id": "payer-1",
		"available_balance_minor": 10000,
		"currency": "usd"
	}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, response.Code)
	}

	assertHeaderExists(t, response, "X-Request-ID")
	assertJSONContentType(t, response)

	var body payer.PayerResponse
	decodeJSON(t, response, &body)

	if body.ID != "payer-1" {
		t.Fatalf("expected payer id payer-1, got %q", body.ID)
	}

	if body.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", body.Currency)
	}
}

func TestPaymentAuthorizeRouteIsWired(t *testing.T) {
	merchantRepository := merchantmemory.NewStore()
	merchantService := merchant.NewMerchantService(merchantRepository)
	merchantHandler := merchant.NewHandler(merchantService)

	payerRepository := payermemory.NewStore()
	payerService := payer.NewPayerService(payerRepository)
	payerHandler := payer.NewHandler(payerService)

	paymentRepository := paymentmemory.NewStore()
	paymentService := payment.NewService(merchantRepository, payerRepository, paymentRepository)
	paymentHandler := payment.NewHandler(paymentService)

	router := NewRouter(RouterConfig{
		ServiceName:     "paycore-api",
		Version:         "test",
		StartedAt:       time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		Logger:          slog.Default(),
		MerchantHandler: merchantHandler,
		PayerHandler:    payerHandler,
		PaymentHandler:  paymentHandler,
	})

	createMerchantRequest := httptest.NewRequest(http.MethodPost, "/merchants", bytes.NewBufferString(`{
		"id": "merchant-1",
		"name": "Demo Merchant",
		"settlement_currency": "USD"
	}`))
	createMerchantResponse := httptest.NewRecorder()
	router.ServeHTTP(createMerchantResponse, createMerchantRequest)
	if createMerchantResponse.Code != http.StatusCreated {
		t.Fatalf("expected merchant create status %d, got %d", http.StatusCreated, createMerchantResponse.Code)
	}

	createPayerRequest := httptest.NewRequest(http.MethodPost, "/payers", bytes.NewBufferString(`{
		"id": "payer-1",
		"available_balance_minor": 10000,
		"currency": "USD"
	}`))
	createPayerResponse := httptest.NewRecorder()
	router.ServeHTTP(createPayerResponse, createPayerRequest)
	if createPayerResponse.Code != http.StatusCreated {
		t.Fatalf("expected payer create status %d, got %d", http.StatusCreated, createPayerResponse.Code)
	}

	authorizeRequest := httptest.NewRequest(http.MethodPost, "/payments/authorize", bytes.NewBufferString(`{
		"merchant_id": "merchant-1",
		"payer_id": "payer-1",
		"amount": 4000,
		"currency": "USD"
	}`))
	authorizeResponse := httptest.NewRecorder()

	router.ServeHTTP(authorizeResponse, authorizeRequest)

	if authorizeResponse.Code != http.StatusCreated {
		t.Fatalf("expected authorize status %d, got %d", http.StatusCreated, authorizeResponse.Code)
	}

	assertHeaderExists(t, authorizeResponse, "X-Request-ID")
	assertJSONContentType(t, authorizeResponse)

	var body payment.AuthorizePaymentResponse
	decodeJSON(t, authorizeResponse, &body)

	if body.Status != "AUTHORIZED" {
		t.Fatalf("expected status AUTHORIZED, got %q", body.Status)
	}
}

func TestPaymentCaptureRouteIsWired(t *testing.T) {
	merchantRepository := merchantmemory.NewStore()
	merchantService := merchant.NewMerchantService(merchantRepository)
	merchantHandler := merchant.NewHandler(merchantService)

	payerRepository := payermemory.NewStore()
	payerService := payer.NewPayerService(payerRepository)
	payerHandler := payer.NewHandler(payerService)

	paymentRepository := paymentmemory.NewStore()
	paymentService := payment.NewService(merchantRepository, payerRepository, paymentRepository)
	paymentHandler := payment.NewHandler(paymentService)

	router := NewRouter(RouterConfig{
		ServiceName:     "paycore-api",
		Version:         "test",
		StartedAt:       time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		Logger:          slog.Default(),
		MerchantHandler: merchantHandler,
		PayerHandler:    payerHandler,
		PaymentHandler:  paymentHandler,
	})

	createMerchantRequest := httptest.NewRequest(http.MethodPost, "/merchants", bytes.NewBufferString(`{
		"id": "merchant-1",
		"name": "Demo Merchant",
		"settlement_currency": "USD"
	}`))
	createMerchantResponse := httptest.NewRecorder()
	router.ServeHTTP(createMerchantResponse, createMerchantRequest)
	if createMerchantResponse.Code != http.StatusCreated {
		t.Fatalf("expected merchant create status %d, got %d", http.StatusCreated, createMerchantResponse.Code)
	}

	createPayerRequest := httptest.NewRequest(http.MethodPost, "/payers", bytes.NewBufferString(`{
		"id": "payer-1",
		"available_balance_minor": 10000,
		"currency": "USD"
	}`))
	createPayerResponse := httptest.NewRecorder()
	router.ServeHTTP(createPayerResponse, createPayerRequest)
	if createPayerResponse.Code != http.StatusCreated {
		t.Fatalf("expected payer create status %d, got %d", http.StatusCreated, createPayerResponse.Code)
	}

	authorizeRequest := httptest.NewRequest(http.MethodPost, "/payments/authorize", bytes.NewBufferString(`{
		"merchant_id": "merchant-1",
		"payer_id": "payer-1",
		"amount": 4000,
		"currency": "USD"
	}`))
	authorizeResponse := httptest.NewRecorder()
	router.ServeHTTP(authorizeResponse, authorizeRequest)
	if authorizeResponse.Code != http.StatusCreated {
		t.Fatalf("expected authorize status %d, got %d", http.StatusCreated, authorizeResponse.Code)
	}

	var authorizeBody payment.AuthorizePaymentResponse
	decodeJSON(t, authorizeResponse, &authorizeBody)

	captureRequest := httptest.NewRequest(http.MethodPost, "/payments/"+authorizeBody.PaymentID+"/capture", nil)
	captureResponse := httptest.NewRecorder()
	router.ServeHTTP(captureResponse, captureRequest)

	if captureResponse.Code != http.StatusOK {
		t.Fatalf("expected capture status %d, got %d", http.StatusOK, captureResponse.Code)
	}

	assertHeaderExists(t, captureResponse, "X-Request-ID")
	assertJSONContentType(t, captureResponse)

	var captureBody payment.CapturePaymentResponse
	decodeJSON(t, captureResponse, &captureBody)

	if captureBody.PaymentID != authorizeBody.PaymentID {
		t.Fatalf("expected payment id %q, got %q", authorizeBody.PaymentID, captureBody.PaymentID)
	}

	if captureBody.Status != "CAPTURED" {
		t.Fatalf("expected status CAPTURED, got %q", captureBody.Status)
	}
}

func newTestRouter() http.Handler {
	return NewRouter(RouterConfig{
		ServiceName: "paycore-api",
		Version:     "test",
		StartedAt:   time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		Logger:      slog.Default(),
	})
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

func assertHeaderExists(t *testing.T, response *httptest.ResponseRecorder, name string) {
	t.Helper()

	if got := response.Header().Get(name); got == "" {
		t.Fatalf("expected %s header", name)
	}
}
