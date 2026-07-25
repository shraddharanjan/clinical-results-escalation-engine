package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/platform/telemetry"
	clinicaltask "github.com/shraddharanjan/clinical-results-escalation-engine/internal/task"
)

type TaskAcknowledger interface {
	Acknowledge(
		ctx context.Context,
		taskID uuid.UUID,
		clinicianID string,
		expectedVersion int64,
	) (clinicaltask.Task, error)
}

type AcknowledgementHandler struct {
	acknowledger TaskAcknowledger
	metrics      *telemetry.Metrics
}

func NewAcknowledgementHandler(
	acknowledger TaskAcknowledger,
	metrics *telemetry.Metrics,
) *AcknowledgementHandler {
	return &AcknowledgementHandler{
		acknowledger: acknowledger,
		metrics:      metrics,
	}
}

type acknowledgementRequest struct {
	ClinicianID     string `json:"clinician_id"`
	ExpectedVersion int64  `json:"expected_version"`
}

type acknowledgementResponse struct {
	Task clinicaltask.Task `json:"task"`
}

func (h *AcknowledgementHandler) Create(
	w http.ResponseWriter,
	r *http.Request,
) {
	taskID, err := uuid.Parse(
		chi.URLParam(r, "taskID"),
	)
	if err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"error": "task ID must be a valid UUID",
			},
		)
		return
	}

	var request acknowledgementRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"error": "request body must contain valid JSON",
			},
		)
		return
	}

	if strings.TrimSpace(request.ClinicianID) == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"error": "clinician_id is required",
			},
		)
		return
	}

	if request.ExpectedVersion <= 0 {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"error": "expected_version must be greater than zero",
			},
		)
		return
	}

	task, err := h.acknowledger.Acknowledge(
		r.Context(),
		taskID,
		request.ClinicianID,
		request.ExpectedVersion,
	)
	if err != nil {
		switch {
		case errors.Is(
			err,
			clinicaltask.ErrTaskNotFound,
		):
			writeJSON(
				w,
				http.StatusNotFound,
				map[string]string{
					"error": "clinical task was not found",
				},
			)

		case errors.Is(
			err,
			clinicaltask.ErrTaskStateConflict,
		):
			writeJSON(
				w,
				http.StatusConflict,
				map[string]string{
					"error": "task is no longer awaiting acknowledgement or its version has changed",
				},
			)

		default:
			writeJSON(
				w,
				http.StatusInternalServerError,
				map[string]string{
					"error": "an internal error occurred",
				},
			)
		}

		return
	}

	h.metrics.RecordAcknowledgement(
		r.Context(),
		task.Severity,
		0,
	)

	writeJSON(
		w,
		http.StatusOK,
		acknowledgementResponse{
			Task: task,
		},
	)
}
