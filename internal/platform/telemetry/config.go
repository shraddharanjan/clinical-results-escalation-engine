package telemetry

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLPEndpoint   string
	Insecure       bool
	Enabled        bool
}

func ConfigFromEnvironment(
	serviceName string,
) (Config, error) {
	if serviceName == "" {
		return Config{}, fmt.Errorf(
			"telemetry service name is required",
		)
	}

	enabled := true

	if value := os.Getenv("OTEL_ENABLED"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf(
				"parse OTEL_ENABLED: %w",
				err,
			)
		}

		enabled = parsed
	}

	endpoint := os.Getenv(
		"OTEL_EXPORTER_OTLP_ENDPOINT",
	)

	if endpoint == "" && enabled {
		endpoint = "localhost:4317"
	}

	environment := os.Getenv(
		"APP_ENVIRONMENT",
	)

	if environment == "" {
		environment = "development"
	}

	version := os.Getenv("APP_VERSION")

	if version == "" {
		version = "development"
	}

	insecure := true

	if value := os.Getenv(
		"OTEL_EXPORTER_OTLP_INSECURE",
	); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf(
				"parse OTEL_EXPORTER_OTLP_INSECURE: %w",
				err,
			)
		}

		insecure = parsed
	}

	return Config{
		ServiceName:    serviceName,
		ServiceVersion: version,
		Environment:    environment,
		OTLPEndpoint:   endpoint,
		Insecure:       insecure,
		Enabled:        enabled,
	}, nil
}