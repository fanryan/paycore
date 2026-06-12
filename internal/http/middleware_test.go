package http

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoveryMiddlewareReturnsStructuredError(t *testing.T) {
	handler := requestIDMiddleware(
		recoveryMiddleware(slog.Default())(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				panic("boom")
			}),
		),
	)

	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	request.Header.Set("X-Request-ID", "test-request-id")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, response.Code)
	}

	assertJSONContentType(t, response)

	var body ErrorResponse
	decodeJSON(t, response, &body)

	if body.ErrorCode != "INTERNAL_SERVER_ERROR" {
		t.Fatalf("expected INTERNAL_SERVER_ERROR, got %q", body.ErrorCode)
	}

	if body.RequestID != "test-request-id" {
		t.Fatalf("expected request id test-request-id, got %q", body.RequestID)
	}

	if got := response.Header().Get("X-Request-ID"); got != "test-request-id" {
		t.Fatalf("expected X-Request-ID to be preserved, got %q", got)
	}
}

func TestBodyLimitMiddlewareLimitsRequestBody(t *testing.T) {
	handler := bodyLimitMiddleware(4)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			if err != nil {
				if errors.Is(err, http.ErrBodyReadAfterClose) {
					t.Fatalf("unexpected body read after close error: %v", err)
				}

				writeError(w, r, http.StatusRequestEntityTooLarge, "REQUEST_BODY_TOO_LARGE", "Request body is too large")
				return
			}

			w.WriteHeader(http.StatusNoContent)
		}),
	)

	request := httptest.NewRequest(http.MethodPost, "/limited", strings.NewReader("12345"))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d", http.StatusRequestEntityTooLarge, response.Code)
	}

	var body ErrorResponse
	decodeJSON(t, response, &body)

	if body.ErrorCode != "REQUEST_BODY_TOO_LARGE" {
		t.Fatalf("expected REQUEST_BODY_TOO_LARGE, got %q", body.ErrorCode)
	}
}

func TestBodyLimitMiddlewareAllowsRequestWithinLimit(t *testing.T) {
	handler := bodyLimitMiddleware(5)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("expected body read to succeed, got error: %v", err)
			}

			if string(body) != "12345" {
				t.Fatalf("expected body 12345, got %q", body)
			}

			w.WriteHeader(http.StatusNoContent)
		}),
	)

	request := httptest.NewRequest(http.MethodPost, "/limited", strings.NewReader("12345"))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, response.Code)
	}
}
