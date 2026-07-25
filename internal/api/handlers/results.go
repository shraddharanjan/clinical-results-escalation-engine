package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	clinicalresult "github.com/shraddharanjan/clinical-results-escalation-engine/internal/result"
)

type ResultHandler struct {
	service *clinicalresult.Service
}

func NewResultHandler(
	service *clinicalresult.Service,
) *ResultHandler {
	return &ResultHandler{
		service: service,
	}
}

type createResultResponse struct {
	Result         clinicalresult.Result         `json:"result"`
	Classification clinicalresult.Classification `json:"classification"`
}

func (h *ResultHandler) Create(
	w http.ResponseWriter,
	r *http.Request,
) {
	var input clinicalresult.CreateResultInput

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "request body must contain valid JSON",
		})
		return
	}

	createdResult, classification, err := h.service.Create(
		r.Context(),
		input,
	)
	if err != nil {
		switch {
		case errors.Is(err, clinicalresult.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": err.Error(),
			})

		case errors.Is(err, clinicalresult.ErrDuplicateResult):
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "this source result has already been received",
			})

		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "an internal error occurred",
			})
		}

		return
	}

	writeJSON(w, http.StatusCreated, createResultResponse{
		Result:         createdResult,
		Classification: classification,
	})
}