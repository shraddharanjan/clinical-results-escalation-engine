package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/api/handlers"
)

func NewRouter(
	healthHandler *handlers.HealthHandler,
	resultHandler *handlers.ResultHandler,
) http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)

	router.Get("/health", healthHandler.Handle)

	router.Route("/v1", func(router chi.Router) {
		router.Post("/results", resultHandler.Create)
	})

	return router
}
