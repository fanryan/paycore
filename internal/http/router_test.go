package http

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/idempotency"
	idempotencymemory "github.com/fanryan/paycore/internal/idempotency/adapters/memory"
	"github.com/fanryan/paycore/internal/merchant"
	merchantmemory "github.com/fanryan/paycore/internal/merchant/adapters/memory"
	"github.com/fanryan/paycore/internal/payer"
	payermemory "github.com/fanryan/paycore/internal/payer/adapters/memory"
	"github.com/fanryan/paycore/internal/payment"
	paymentmemory "github.com/fanryan/paycore/internal/payment/adapters/memory"
	"github.com/fanryan/paycore/internal/ratelimit"
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

func TestPaymentAuthorizeRouteUsesIdempotencyWhenConfigured(t *testing.T) {
	merchantRepository := merchantmemory.NewStore()
	merchantService := merchant.NewMerchantService(merchantRepository)
	merchantHandler := merchant.NewHandler(merchantService)

	payerRepository := payermemory.NewStore()
	payerService := payer.NewPayerService(payerRepository)
	payerHandler := payer.NewHandler(payerService)

	paymentRepository := paymentmemory.NewStore()
	paymentService := payment.NewService(merchantRepository, payerRepository, paymentRepository)
	idempotencyRepository := idempotencymemory.NewStore()
	idempotencyService := idempotency.NewService(idempotencyRepository, 24*time.Hour)
	paymentHandler := payment.NewHandlerWithIdempotency(paymentService, idempotencyService)

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

	if authorizeResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected missing idempotency key status %d, got %d", http.StatusBadRequest, authorizeResponse.Code)
	}

	var errorBody map[string]string
	decodeJSON(t, authorizeResponse, &errorBody)
	if errorBody["error_code"] != "IDEMPOTENCY_KEY_REQUIRED" {
		t.Fatalf("expected IDEMPOTENCY_KEY_REQUIRED, got %q", errorBody["error_code"])
	}

	idempotentRequest := httptest.NewRequest(http.MethodPost, "/payments/authorize", bytes.NewBufferString(`{
		"merchant_id": "merchant-1",
		"payer_id": "payer-1",
		"amount": 4000,
		"currency": "USD"
	}`))
	idempotentRequest.Header.Set("Idempotency-Key", "idem-key-1")
	idempotentResponse := httptest.NewRecorder()
	router.ServeHTTP(idempotentResponse, idempotentRequest)

	if idempotentResponse.Code != http.StatusCreated {
		t.Fatalf("expected idempotent authorize status %d, got %d", http.StatusCreated, idempotentResponse.Code)
	}
}

func TestPaymentRoutesUseRateLimiterWhenConfigured(t *testing.T) {
	limiter := &fakeRateLimiter{
		result: ratelimit.Result{
			Allowed:   true,
			Limit:     2,
			Remaining: 1,
		},
	}

	router := NewRouter(RouterConfig{
		ServiceName: "paycore-api",
		Version:     "test",
		StartedAt:   time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		Logger:      slog.Default(),
		PaymentHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}),
		RateLimiter: limiter,
	})

	request := httptest.NewRequest(http.MethodPost, "/payments/authorize", nil)
	request.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.1")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, response.Code)
	}

	if limiter.key != "203.0.113.10" {
		t.Fatalf("expected limiter key 203.0.113.10, got %q", limiter.key)
	}

	if response.Header().Get("RateLimit-Limit") != "2" {
		t.Fatalf("expected RateLimit-Limit 2, got %q", response.Header().Get("RateLimit-Limit"))
	}

	if response.Header().Get("RateLimit-Remaining") != "1" {
		t.Fatalf("expected RateLimit-Remaining 1, got %q", response.Header().Get("RateLimit-Remaining"))
	}
}

func TestPaymentRoutesReturnTooManyRequestsWhenRateLimitExceeded(t *testing.T) {
	router := NewRouter(RouterConfig{
		ServiceName: "paycore-api",
		Version:     "test",
		StartedAt:   time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		Logger:      slog.Default(),
		PaymentHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("payment handler should not be called")
		}),
		RateLimiter: &fakeRateLimiter{
			result: ratelimit.Result{
				Allowed:    false,
				Limit:      2,
				Remaining:  0,
				RetryAfter: 30 * time.Second,
			},
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/payments/authorize", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, response.Code)
	}

	if response.Header().Get("Retry-After") != "30" {
		t.Fatalf("expected Retry-After 30, got %q", response.Header().Get("Retry-After"))
	}

	var body ErrorResponse
	decodeJSON(t, response, &body)
	if body.ErrorCode != "RATE_LIMIT_EXCEEDED" {
		t.Fatalf("expected RATE_LIMIT_EXCEEDED, got %q", body.ErrorCode)
	}
}

func TestPaymentRoutesFailClosedWhenRateLimiterUnavailable(t *testing.T) {
	router := NewRouter(RouterConfig{
		ServiceName: "paycore-api",
		Version:     "test",
		StartedAt:   time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		Logger:      slog.Default(),
		PaymentHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("payment handler should not be called")
		}),
		RateLimiter: &fakeRateLimiter{
			err: ratelimit.ErrLimiterUnavailable,
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/payments/authorize", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.Code)
	}

	var body ErrorResponse
	decodeJSON(t, response, &body)
	if body.ErrorCode != "RATE_LIMITER_UNAVAILABLE" {
		t.Fatalf("expected RATE_LIMITER_UNAVAILABLE, got %q", body.ErrorCode)
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

type fakeRateLimiter struct {
	result ratelimit.Result
	err    error
	key    string
}

func (l *fakeRateLimiter) Allow(ctx context.Context, key string) (ratelimit.Result, error) {
	l.key = key
	return l.result, l.err
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
