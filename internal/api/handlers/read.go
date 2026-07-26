package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	clinicalresult "github.com/shraddharanjan/clinical-results-escalation-engine/internal/result"
	clinicaltask "github.com/shraddharanjan/clinical-results-escalation-engine/internal/task"
)

type ReadHandler struct {
	taskRepository   *clinicaltask.PostgresRepository
	resultRepository *clinicalresult.PostgresRepository
}

func NewReadHandler(
	taskRepository *clinicaltask.PostgresRepository,
	resultRepository *clinicalresult.PostgresRepository,
) *ReadHandler {
	return &ReadHandler{
		taskRepository:   taskRepository,
		resultRepository: resultRepository,
	}
}

func (h *ReadHandler) ListTasks(
	writer http.ResponseWriter,
	request *http.Request,
) {
	tasks, err := h.taskRepository.ListForAPI(
		request.Context(),
	)
	if err != nil {
		log.Printf("list tasks: %v", err)

		writeReadError(
			writer,
			http.StatusInternalServerError,
			"failed to retrieve tasks",
		)
		return
	}

	writeReadJSON(
		writer,
		http.StatusOK,
		tasks,
	)
}

func (h *ReadHandler) ListResults(
	writer http.ResponseWriter,
	request *http.Request,
) {
	results, err :=
		h.resultRepository.ListForAPI(
			request.Context(),
		)
	if err != nil {
		log.Printf("list results: %v", err)

		writeReadError(
			writer,
			http.StatusInternalServerError,
			"failed to retrieve results",
		)
		return
	}

	writeReadJSON(
		writer,
		http.StatusOK,
		results,
	)
}

func writeReadJSON(
	writer http.ResponseWriter,
	status int,
	value any,
) {
	writer.Header().Set(
		"Content-Type",
		"application/json",
	)
	writer.WriteHeader(status)

	if err := json.NewEncoder(writer).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeReadError(
	writer http.ResponseWriter,
	status int,
	message string,
) {
	writeReadJSON(
		writer,
		status,
		map[string]string{
			"error": message,
		},
	)
}
