package payment

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fanryan/paycore/internal/merchant"
	"github.com/fanryan/paycore/internal/payer"
	"github.com/fanryan/paycore/internal/shared/httpjson"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
}

type AuthorizePaymentRequest struct {
	MerchantID  string `json:"merchant_id"`
	PayerID     string `json:"payer_id"`
	AmountMinor int64  `json:"amount"`
	Currency    string `json:"currency"`
}

type AuthorizePaymentResponse struct {
	PaymentID    string `json:"payment_id"`
	Status       string `json:"status"`
	MerchantID   string `json:"merchant_id"`
	PayerID      string `json:"payer_id"`
	AmountMinor  int64  `json:"amount"`
	Currency     string `json:"currency"`
	HoldID       string `json:"hold_id"`
	AuthorizedAt string `json:"authorized_at"`
	ExpiresAt    string `json:"expires_at"`
}

type CapturePaymentResponse struct {
	PaymentID      string `json:"payment_id"`
	Status         string `json:"status"`
	CapturedAmount int64  `json:"captured_amount"`
	Currency       string `json:"currency"`
	CapturedAt     string `json:"captured_at"`
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/payments/authorize":
		h.HandleAuthorize(w, r)
		return
	case chi.URLParam(r, "payment_id") != "":
		h.HandleCapture(w, r)
		return
	}

	httpjson.Write(w, http.StatusNotFound, map[string]string{
		"error_code": "PAYMENT_ROUTE_NOT_FOUND",
		"message":    "Payment route not found",
	})
}

func (h *Handler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpjson.Write(w, http.StatusMethodNotAllowed, map[string]string{
			"error_code": "METHOD_NOT_ALLOWED",
			"message":    "Method not allowed",
		})
		return
	}

	var request AuthorizePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpjson.Write(w, http.StatusBadRequest, map[string]string{
			"error_code": "INVALID_JSON",
			"message":    "Request body must be valid JSON",
		})
		return
	}

	result, err := h.service.AuthorizePayment(r.Context(), AuthorizePaymentInput{
		MerchantID:  request.MerchantID,
		PayerID:     request.PayerID,
		AmountMinor: request.AmountMinor,
		Currency:    request.Currency,
	})
	if err != nil {
		status, body := authorizationErrorResponse(err)
		httpjson.Write(w, status, body)
		return
	}

	httpjson.Write(w, http.StatusCreated, authorizePaymentResponse(result))
}

func (h *Handler) HandleCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpjson.Write(w, http.StatusMethodNotAllowed, map[string]string{
			"error_code": "METHOD_NOT_ALLOWED",
			"message":    "Method not allowed",
		})
		return
	}

	paymentID := chi.URLParam(r, "payment_id")
	if paymentID == "" {
		httpjson.Write(w, http.StatusNotFound, map[string]string{
			"error_code": "PAYMENT_ROUTE_NOT_FOUND",
			"message":    "Payment route not found",
		})
		return
	}

	result, err := h.service.CapturePayment(r.Context(), CapturePaymentInput{
		PaymentID: paymentID,
	})
	if err != nil {
		status, body := captureErrorResponse(err)
		httpjson.Write(w, status, body)
		return
	}

	httpjson.Write(w, http.StatusOK, capturePaymentResponse(result))
}

func authorizationErrorResponse(err error) (int, map[string]string) {
	switch {
	case errors.Is(err, merchant.ErrMerchantNotFound):
		return http.StatusNotFound, map[string]string{
			"error_code": "MERCHANT_NOT_FOUND",
			"message":    "Merchant not found",
		}
	case errors.Is(err, payer.ErrPayerNotFound):
		return http.StatusNotFound, map[string]string{
			"error_code": "PAYER_NOT_FOUND",
			"message":    "Payer not found",
		}
	case errors.Is(err, ErrMerchantCannotCreatePayments):
		return http.StatusConflict, map[string]string{
			"error_code": "MERCHANT_CANNOT_CREATE_PAYMENTS",
			"message":    "Merchant cannot create payments",
		}
	case errors.Is(err, ErrPayerCurrencyMismatch):
		return http.StatusUnprocessableEntity, map[string]string{
			"error_code": "PAYER_CURRENCY_MISMATCH",
			"message":    "Payer currency does not match payment currency",
		}
	case errors.Is(err, ErrInsufficientAvailableBalance):
		return http.StatusUnprocessableEntity, map[string]string{
			"error_code": "INSUFFICIENT_AVAILABLE_BALANCE",
			"message":    "Payer has insufficient available balance",
		}
	default:
		return http.StatusBadRequest, map[string]string{
			"error_code": "INVALID_PAYMENT_AUTHORIZATION",
			"message":    err.Error(),
		}
	}
}

func captureErrorResponse(err error) (int, map[string]string) {
	switch {
	case errors.Is(err, ErrPaymentNotFound):
		return http.StatusNotFound, map[string]string{
			"error_code": "PAYMENT_NOT_FOUND",
			"message":    "Payment not found",
		}
	case errors.Is(err, ErrHoldNotFound):
		return http.StatusNotFound, map[string]string{
			"error_code": "PAYMENT_HOLD_NOT_FOUND",
			"message":    "Payment hold not found",
		}
	case errors.Is(err, payer.ErrPayerNotFound):
		return http.StatusNotFound, map[string]string{
			"error_code": "PAYER_NOT_FOUND",
			"message":    "Payer not found",
		}
	case errors.Is(err, ErrPaymentNotCapturable):
		return http.StatusConflict, map[string]string{
			"error_code": "PAYMENT_NOT_CAPTURABLE",
			"message":    "Payment is not capturable",
		}
	case errors.Is(err, ErrAuthorizationExpired):
		return http.StatusUnprocessableEntity, map[string]string{
			"error_code": "AUTHORIZATION_EXPIRED",
			"message":    "Authorization has expired",
		}
	default:
		return http.StatusBadRequest, map[string]string{
			"error_code": "INVALID_PAYMENT_CAPTURE",
			"message":    err.Error(),
		}
	}
}

func authorizePaymentResponse(result AuthorizePaymentResult) AuthorizePaymentResponse {
	return AuthorizePaymentResponse{
		PaymentID:    result.Payment.ID,
		Status:       string(result.Payment.Status),
		MerchantID:   result.Payment.MerchantID,
		PayerID:      result.Payment.PayerID,
		AmountMinor:  result.Payment.AmountMinor,
		Currency:     result.Payment.Currency,
		HoldID:       result.Hold.ID,
		AuthorizedAt: result.Payment.AuthorizedAt.Format("2006-01-02T15:04:05Z07:00"),
		ExpiresAt:    result.Payment.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func capturePaymentResponse(result CapturePaymentResult) CapturePaymentResponse {
	capturedAt := ""
	if result.Payment.CapturedAt != nil {
		capturedAt = result.Payment.CapturedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	return CapturePaymentResponse{
		PaymentID:      result.Payment.ID,
		Status:         string(result.Payment.Status),
		CapturedAmount: result.Payment.AmountMinor,
		Currency:       result.Payment.Currency,
		CapturedAt:     capturedAt,
	}
}
