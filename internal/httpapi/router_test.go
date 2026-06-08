package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
