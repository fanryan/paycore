package payment

import (
	"net/http"
	"testing"

	"github.com/fanryan/paycore/internal/payer"
)

func TestAuthorizationErrorResponseMapsPayerVersionConflict(t *testing.T) {
	status, body := authorizationErrorResponse(payer.ErrPayerVersionConflict)

	if status != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, status)
	}

	if body["error_code"] != "PAYER_VERSION_CONFLICT" {
		t.Fatalf("expected PAYER_VERSION_CONFLICT, got %q", body["error_code"])
	}
}

func TestCaptureErrorResponseMapsPayerVersionConflict(t *testing.T) {
	status, body := captureErrorResponse(payer.ErrPayerVersionConflict)

	if status != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, status)
	}

	if body["error_code"] != "PAYER_VERSION_CONFLICT" {
		t.Fatalf("expected PAYER_VERSION_CONFLICT, got %q", body["error_code"])
	}
}
