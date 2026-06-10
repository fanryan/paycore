package payer_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fanryan/paycore/internal/payer"
	payermemory "github.com/fanryan/paycore/internal/payer/adapters/memory"
)

func TestHandlerCreatesPayer(t *testing.T) {
	handler := newTestHandler()

	request := httptest.NewRequest(http.MethodPost, "/payers", bytes.NewBufferString(`{
		"id": "payer-1",
		"available_balance_minor": 10000,
		"currency": "usd"
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, response.Code)
	}

	assertJSONContentType(t, response)

	var body payer.PayerResponse
	decodeJSON(t, response, &body)

	if body.ID != "payer-1" {
		t.Fatalf("expected payer id payer-1, got %q", body.ID)
	}

	if body.AvailableBalanceMinor != 10_000 {
		t.Fatalf("expected available balance 10000, got %d", body.AvailableBalanceMinor)
	}

	if body.HeldBalanceMinor != 0 {
		t.Fatalf("expected held balance 0, got %d", body.HeldBalanceMinor)
	}

	if body.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", body.Currency)
	}

	if body.Version != 0 {
		t.Fatalf("expected version 0, got %d", body.Version)
	}
}

func TestHandlerListsPayers(t *testing.T) {
	handler := newTestHandler()

	createRequest := httptest.NewRequest(http.MethodPost, "/payers", bytes.NewBufferString(`{
		"id": "payer-1",
		"available_balance_minor": 10000,
		"currency": "USD"
	}`))
	createResponse := httptest.NewRecorder()

	handler.ServeHTTP(createResponse, createRequest)

	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createResponse.Code)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/payers", nil)
	listResponse := httptest.NewRecorder()

	handler.ServeHTTP(listResponse, listRequest)

	if listResponse.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, listResponse.Code)
	}

	var body []payer.PayerResponse
	decodeJSON(t, listResponse, &body)

	if len(body) != 1 {
		t.Fatalf("expected 1 payer, got %d", len(body))
	}

	if body[0].ID != "payer-1" {
		t.Fatalf("expected payer id payer-1, got %q", body[0].ID)
	}
}

func TestHandlerRejectsInvalidJSON(t *testing.T) {
	handler := newTestHandler()

	request := httptest.NewRequest(http.MethodPost, "/payers", bytes.NewBufferString(`{`))
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

func TestHandlerRejectsInvalidPayer(t *testing.T) {
	handler := newTestHandler()

	request := httptest.NewRequest(http.MethodPost, "/payers", bytes.NewBufferString(`{
		"id": "payer-1",
		"available_balance_minor": -1,
		"currency": "USD"
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}

	var body map[string]string
	decodeJSON(t, response, &body)

	if body["error_code"] != "INVALID_PAYER" {
		t.Fatalf("expected INVALID_PAYER, got %q", body["error_code"])
	}
}

func TestHandlerRejectsDuplicatePayer(t *testing.T) {
	handler := newTestHandler()
	body := `{
		"id": "payer-1",
		"available_balance_minor": 10000,
		"currency": "USD"
	}`

	firstRequest := httptest.NewRequest(http.MethodPost, "/payers", bytes.NewBufferString(body))
	firstResponse := httptest.NewRecorder()

	handler.ServeHTTP(firstResponse, firstRequest)

	if firstResponse.Code != http.StatusCreated {
		t.Fatalf("expected first create status %d, got %d", http.StatusCreated, firstResponse.Code)
	}

	secondRequest := httptest.NewRequest(http.MethodPost, "/payers", bytes.NewBufferString(body))
	secondResponse := httptest.NewRecorder()

	handler.ServeHTTP(secondResponse, secondRequest)

	if secondResponse.Code != http.StatusConflict {
		t.Fatalf("expected duplicate status %d, got %d", http.StatusConflict, secondResponse.Code)
	}

	var responseBody map[string]string
	decodeJSON(t, secondResponse, &responseBody)

	if responseBody["error_code"] != "PAYER_ALREADY_EXISTS" {
		t.Fatalf("expected PAYER_ALREADY_EXISTS, got %q", responseBody["error_code"])
	}
}

func TestHandlerRejectsUnsupportedMethod(t *testing.T) {
	handler := newTestHandler()

	request := httptest.NewRequest(http.MethodDelete, "/payers", nil)
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

func newTestHandler() *payer.Handler {
	repository := payermemory.NewStore()
	service := payer.NewPayerService(repository)

	return payer.NewHandler(service)
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
