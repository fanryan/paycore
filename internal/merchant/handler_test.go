package merchant_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fanryan/paycore/internal/merchant"
	merchantmemory "github.com/fanryan/paycore/internal/merchant/adapters/memory"
)

func TestHandlerCreatesMerchant(t *testing.T) {
	handler := newTestHandler()

	request := httptest.NewRequest(http.MethodPost, "/merchants", bytes.NewBufferString(`{
		"id": "merchant-1",
		"name": "Demo Merchant",
		"settlement_currency": "usd"
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, response.Code)
	}

	assertJSONContentType(t, response)

	var body merchant.MerchantResponse
	decodeJSON(t, response, &body)

	if body.ID != "merchant-1" {
		t.Fatalf("expected merchant id merchant-1, got %q", body.ID)
	}

	if body.Name != "Demo Merchant" {
		t.Fatalf("expected merchant name Demo Merchant, got %q", body.Name)
	}

	if body.Status != "ACTIVE" {
		t.Fatalf("expected status ACTIVE, got %q", body.Status)
	}

	if body.SettlementCurrency != "USD" {
		t.Fatalf("expected settlement currency USD, got %q", body.SettlementCurrency)
	}
}

func TestHandlerListsMerchants(t *testing.T) {
	handler := newTestHandler()

	createRequest := httptest.NewRequest(http.MethodPost, "/merchants", bytes.NewBufferString(`{
		"id": "merchant-1",
		"name": "Demo Merchant",
		"settlement_currency": "USD"
	}`))
	createResponse := httptest.NewRecorder()

	handler.ServeHTTP(createResponse, createRequest)

	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createResponse.Code)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/merchants", nil)
	listResponse := httptest.NewRecorder()

	handler.ServeHTTP(listResponse, listRequest)

	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, listResponse.Code)
	}

	var body []merchant.MerchantResponse
	decodeJSON(t, listResponse, &body)

	if len(body) != 1 {
		t.Fatalf("expected 1 merchant, got %d", len(body))
	}

	if body[0].ID != "merchant-1" {
		t.Fatalf("expected merchant id merchant-1, got %q", body[0].ID)
	}
}

func TestHandlerRejectsInvalidJSON(t *testing.T) {
	handler := newTestHandler()

	request := httptest.NewRequest(http.MethodPost, "/merchants", bytes.NewBufferString(`{`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}

	var body map[string]string
	decodeJSON(t, response, &body)

	if body["error_code"] != "INVALID_JSON" {
		t.Fatalf("expected INVALID_JSON, got %q", body["error_code"])
	}
}

func TestHandlerRejectsInvalidMerchant(t *testing.T) {
	handler := newTestHandler()

	request := httptest.NewRequest(http.MethodPost, "/merchants", bytes.NewBufferString(`{
		"id": "",
		"name": "Demo Merchant",
		"settlement_currency": "USD"
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}

	var body map[string]string
	decodeJSON(t, response, &body)

	if body["error_code"] != "INVALID_MERCHANT" {
		t.Fatalf("expected INVALID_MERCHANT, got %q", body["error_code"])
	}
}

func TestHandlerRejectsDuplicateMerchant(t *testing.T) {
	handler := newTestHandler()
	body := `{
		"id": "merchant-1",
		"name": "Demo Merchant",
		"settlement_currency": "USD"
	}`

	firstRequest := httptest.NewRequest(http.MethodPost, "/merchants", bytes.NewBufferString(body))
	firstResponse := httptest.NewRecorder()

	handler.ServeHTTP(firstResponse, firstRequest)

	if firstResponse.Code != http.StatusCreated {
		t.Fatalf("expected first create status %d, got %d", http.StatusCreated, firstResponse.Code)
	}

	secondRequest := httptest.NewRequest(http.MethodPost, "/merchants", bytes.NewBufferString(body))
	secondResponse := httptest.NewRecorder()

	handler.ServeHTTP(secondResponse, secondRequest)

	if secondResponse.Code != http.StatusConflict {
		t.Fatalf("expected duplicate status %d, got %d", http.StatusConflict, secondResponse.Code)
	}

	var responseBody map[string]string
	decodeJSON(t, secondResponse, &responseBody)

	if responseBody["error_code"] != "MERCHANT_ALREADY_EXISTS" {
		t.Fatalf("expected MERCHANT_ALREADY_EXISTS, got %q", responseBody["error_code"])
	}
}

func TestHandlerRejectsUnsupportedMethod(t *testing.T) {
	handler := newTestHandler()

	request := httptest.NewRequest(http.MethodDelete, "/merchants", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, response.Code)
	}

	var body map[string]string
	decodeJSON(t, response, &body)

	if body["error_code"] != "METHOD_NOT_ALLOWED" {
		t.Fatalf("expected METHOD_NOT_ALLOWED, got %q", body["error_code"])
	}
}

func newTestHandler() *merchant.Handler {
	repository := merchantmemory.NewStore()
	service := merchant.NewMerchantService(repository)

	return merchant.NewHandler(service)
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
