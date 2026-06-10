package payer

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fanryan/paycore/internal/shared/httpjson"
)

type Handler struct {
	service *PayerService
}

type CreatePayerRequest struct {
	ID                    string `json:"id"`
	AvailableBalanceMinor int64  `json:"available_balance_minor"`
	Currency              string `json:"currency"`
}

type PayerResponse struct {
	ID                    string `json:"id"`
	AvailableBalanceMinor int64  `json:"available_balance_minor"`
	HeldBalanceMinor      int64  `json:"held_balance_minor"`
	Currency              string `json:"currency"`
	Version               int64  `json:"version"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
}

func NewHandler(service *PayerService) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.HandlePayers(w, r)
}

func (h *Handler) HandlePayers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createPayer(w, r)
	case http.MethodGet:
		h.listPayers(w, r)
	default:
		httpjson.Write(w, http.StatusMethodNotAllowed, map[string]string{
			"error_code": "METHOD_NOT_ALLOWED",
			"message":    "Method not allowed",
		})
	}
}

func (h *Handler) createPayer(w http.ResponseWriter, r *http.Request) {
	var request CreatePayerRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpjson.Write(w, http.StatusBadRequest, map[string]string{
			"error_code": "INVALID_JSON",
			"message":    "Request body must be valid JSON",
		})
		return
	}

	payer, err := h.service.CreatePayer(r.Context(), CreatePayerInput{
		ID:                    request.ID,
		AvailableBalanceMinor: request.AvailableBalanceMinor,
		Currency:              request.Currency,
	})
	if err != nil {
		status, body := payerErrorResponse(err)
		httpjson.Write(w, status, body)
		return
	}

	httpjson.Write(w, http.StatusCreated, payerResponse(payer))
}

func (h *Handler) listPayers(w http.ResponseWriter, r *http.Request) {
	payers, err := h.service.ListPayers(r.Context())
	if err != nil {
		httpjson.Write(w, http.StatusInternalServerError, map[string]string{
			"error_code": "PAYER_LIST_FAILED",
			"message":    "Failed to list payers",
		})
		return
	}

	response := make([]PayerResponse, 0, len(payers))
	for _, payer := range payers {
		response = append(response, payerResponse(payer))
	}

	httpjson.Write(w, http.StatusOK, response)
}

func payerErrorResponse(err error) (int, map[string]string) {
	switch {
	case errors.Is(err, ErrDuplicatePayer):
		return http.StatusConflict, map[string]string{
			"error_code": "PAYER_ALREADY_EXISTS",
			"message":    "Payer already exists",
		}
	case errors.Is(err, ErrPayerNotFound):
		return http.StatusNotFound, map[string]string{
			"error_code": "PAYER_NOT_FOUND",
			"message":    "Payer not found",
		}
	default:
		return http.StatusBadRequest, map[string]string{
			"error_code": "INVALID_PAYER",
			"message":    err.Error(),
		}
	}
}

func payerResponse(payer Payer) PayerResponse {
	return PayerResponse{
		ID:                    payer.ID,
		AvailableBalanceMinor: payer.AvailableBalanceMinor,
		HeldBalanceMinor:      payer.HeldBalanceMinor,
		Currency:              payer.Currency,
		Version:               payer.Version,
		CreatedAt:             payer.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:             payer.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
