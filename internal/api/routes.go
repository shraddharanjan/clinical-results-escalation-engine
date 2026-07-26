package api

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/api/handlers"
)

func NewRouter(
	healthHandler *handlers.HealthHandler,
	resultHandler *handlers.ResultHandler,
	readHandler *handlers.ReadHandler,
	acknowledgementHandler *handlers.AcknowledgementHandler,
) http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)

	allowedOrigins := []string{
		"http://localhost:*",
		"http://127.0.0.1:*",
	}

	if frontendURL := os.Getenv("FRONTEND_URL"); frontendURL != "" {
		allowedOrigins = append(
			allowedOrigins,
			frontendURL,
		)
	}

	router.Use(cors.Handler(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
		},
		ExposedHeaders: []string{
			"Link",
		},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	router.Get(
		"/health",
		healthHandler.Handle,
	)

	router.Route("/v1", func(router chi.Router) {
		router.Get(
			"/results",
			readHandler.ListResults,
		)

		router.Post(
			"/results",
			resultHandler.Create,
		)

		router.Get(
			"/tasks",
			readHandler.ListTasks,
		)

		router.Post(
			"/tasks/{taskID}/acknowledgements",
			acknowledgementHandler.Create,
		)
	})

	return otelhttp.NewHandler(
		router,
		"clinical-results-api",
	)
}