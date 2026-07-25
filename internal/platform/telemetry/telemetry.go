package telemetry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

type Providers struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *metric.MeterProvider
}

func Initialise(
	ctx context.Context,
	config Config,
) (*Providers, error) {
	appResource, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironmentName(
				config.Environment,
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create OpenTelemetry resource: %w",
			err,
		)
	}

	traceOptions := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(config.OTLPEndpoint),
	}

	metricOptions := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(config.OTLPEndpoint),
	}

	if config.Insecure {
		traceOptions = append(
			traceOptions,
			otlptracegrpc.WithInsecure(),
		)

		metricOptions = append(
			metricOptions,
			otlpmetricgrpc.WithInsecure(),
		)
	}

	traceExporter, err := otlptracegrpc.New(
		ctx,
		traceOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create OTLP trace exporter: %w",
			err,
		)
	}

	metricExporter, err := otlpmetricgrpc.New(
		ctx,
		metricOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create OTLP metric exporter: %w",
			err,
		)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(appResource),
		sdktrace.WithSampler(
			sdktrace.ParentBased(
				sdktrace.TraceIDRatioBased(1),
			),
		),
	)

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(appResource),
		metric.WithReader(
			metric.NewPeriodicReader(
				metricExporter,
				metric.WithInterval(5*time.Second),
			),
		),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return &Providers{
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
	}, nil
}

func (p *Providers) Shutdown(
	ctx context.Context,
) error {
	if p == nil {
		return nil
	}

	var shutdownErrors []error

	if p.MeterProvider != nil {
		if err := p.MeterProvider.Shutdown(ctx); err != nil {
			shutdownErrors = append(
				shutdownErrors,
				fmt.Errorf(
					"shut down meter provider: %w",
					err,
				),
			)
		}
	}

	if p.TracerProvider != nil {
		if err := p.TracerProvider.Shutdown(ctx); err != nil {
			shutdownErrors = append(
				shutdownErrors,
				fmt.Errorf(
					"shut down tracer provider: %w",
					err,
				),
			)
		}
	}

	return errors.Join(shutdownErrors...)
}
