package payment

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fanryan/paycore/internal/merchant"
	"github.com/fanryan/paycore/internal/payer"
	"github.com/fanryan/paycore/internal/shared/httpjson"
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

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.HandleAuthorize(w, r)
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
