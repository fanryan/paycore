package merchant

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fanryan/paycore/internal/shared/httpjson"
)

type Handler struct {
	service *MerchantService
}

type CreateMerchantRequest struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	SettlementCurrency string `json:"settlement_currency"`
}

type MerchantResponse struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Status             string `json:"status"`
	SettlementCurrency string `json:"settlement_currency"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

func NewHandler(service *MerchantService) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.HandleMerchants(w, r)
}

func (h *Handler) HandleMerchants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createMerchant(w, r)
	case http.MethodGet:
		h.listMerchants(w, r)
	default:
		httpjson.Write(w, http.StatusMethodNotAllowed, map[string]string{
			"error_code": "METHOD_NOT_ALLOWED",
			"message":    "Method not allowed",
		})
	}
}

func (h *Handler) createMerchant(w http.ResponseWriter, r *http.Request) {
	var request CreateMerchantRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpjson.Write(w, http.StatusBadRequest, map[string]string{
			"error_code": "INVALID_JSON",
			"message":    "Request body must be valid JSON",
		})
		return
	}

	merchant, err := h.service.CreateMerchant(r.Context(), CreateMerchantInput{
		ID:                 request.ID,
		Name:               request.Name,
		SettlementCurrency: request.SettlementCurrency,
	})
	if err != nil {
		status, body := merchantErrorResponse(err)
		httpjson.Write(w, status, body)
		return
	}

	httpjson.Write(w, http.StatusCreated, merchantResponse(merchant))
}

func (h *Handler) listMerchants(w http.ResponseWriter, r *http.Request) {
	merchants, err := h.service.ListMerchants(r.Context())
	if err != nil {
		httpjson.Write(w, http.StatusInternalServerError, map[string]string{
			"error_code": "MERCHANT_LIST_FAILED",
			"message":    "Failed to list merchants",
		})
		return
	}

	response := make([]MerchantResponse, 0, len(merchants))
	for _, merchant := range merchants {
		response = append(response, merchantResponse(merchant))
	}

	httpjson.Write(w, http.StatusOK, response)
}

func merchantErrorResponse(err error) (int, map[string]string) {
	switch {
	case errors.Is(err, ErrDuplicateMerchant):
		return http.StatusConflict, map[string]string{
			"error_code": "MERCHANT_ALREADY_EXISTS",
			"message":    "Merchant already exists",
		}
	case errors.Is(err, ErrMerchantNotFound):
		return http.StatusNotFound, map[string]string{
			"error_code": "MERCHANT_NOT_FOUND",
			"message":    "Merchant not found",
		}
	default:
		return http.StatusBadRequest, map[string]string{
			"error_code": "INVALID_MERCHANT",
			"message":    err.Error(),
		}
	}
}

func merchantResponse(merchant Merchant) MerchantResponse {
	return MerchantResponse{
		ID:                 merchant.ID,
		Name:               merchant.Name,
		Status:             string(merchant.Status),
		SettlementCurrency: merchant.SettlementCurrency,
		CreatedAt:          merchant.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:          merchant.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
