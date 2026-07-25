package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const instrumentationName = "github.com/shraddharanjan/clinical-results-escalation-engine"

type Metrics struct {
	resultsIngested          metric.Int64Counter
	tasksClaimed             metric.Int64Counter
	taskRecoveries           metric.Int64Counter
	leaseRenewals            metric.Int64Counter
	notifications            metric.Int64Counter
	escalations              metric.Int64Counter
	acknowledgements         metric.Int64Counter
	workerProcessingDuration metric.Float64Histogram
	acknowledgementLatency   metric.Float64Histogram
}

func NewMetrics() (*Metrics, error) {
	meter := otel.Meter(instrumentationName)

	resultsIngested, err := meter.Int64Counter(
		"clinical_results_ingested_total",
		metric.WithDescription(
			"Number of synthetic clinical results ingested",
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create results-ingested counter: %w",
			err,
		)
	}

	tasksClaimed, err := meter.Int64Counter(
		"clinical_tasks_claimed_total",
		metric.WithDescription(
			"Number of clinical tasks claimed",
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create tasks-claimed counter: %w",
			err,
		)
	}

	taskRecoveries, err := meter.Int64Counter(
		"clinical_task_recoveries_total",
		metric.WithDescription(
			"Number of processing tasks recovered after lease expiry",
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create task-recoveries counter: %w",
			err,
		)
	}

	leaseRenewals, err := meter.Int64Counter(
		"clinical_task_lease_renewals_total",
		metric.WithDescription(
			"Number of successful task lease renewals",
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create lease-renewals counter: %w",
			err,
		)
	}

	notifications, err := meter.Int64Counter(
		"clinical_notifications_total",
		metric.WithDescription(
			"Number of notification outcomes",
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create notifications counter: %w",
			err,
		)
	}

	escalations, err := meter.Int64Counter(
		"clinical_escalations_total",
		metric.WithDescription(
			"Number of clinical task escalations",
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create escalations counter: %w",
			err,
		)
	}

	acknowledgements, err := meter.Int64Counter(
		"clinical_acknowledgements_total",
		metric.WithDescription(
			"Number of acknowledged clinical tasks",
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create acknowledgements counter: %w",
			err,
		)
	}

	workerProcessingDuration, err := meter.Float64Histogram(
		"clinical_worker_processing_duration_seconds",
		metric.WithDescription(
			"Duration of task processing by workers",
		),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create processing-duration histogram: %w",
			err,
		)
	}

	acknowledgementLatency, err := meter.Float64Histogram(
		"clinical_acknowledgement_latency_seconds",
		metric.WithDescription(
			"Duration between notification delivery and acknowledgement",
		),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create acknowledgement-latency histogram: %w",
			err,
		)
	}

	return &Metrics{
		resultsIngested:          resultsIngested,
		tasksClaimed:             tasksClaimed,
		taskRecoveries:           taskRecoveries,
		leaseRenewals:            leaseRenewals,
		notifications:            notifications,
		escalations:              escalations,
		acknowledgements:         acknowledgements,
		workerProcessingDuration: workerProcessingDuration,
		acknowledgementLatency:   acknowledgementLatency,
	}, nil
}

func (m *Metrics) RecordResultIngested(
	ctx context.Context,
	severity string,
) {
	if m == nil {
		return
	}

	m.resultsIngested.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.String("severity", severity),
		),
	)
}

func (m *Metrics) RecordTaskClaim(
	ctx context.Context,
	severity string,
	recovered bool,
) {
	if m == nil {
		return
	}

	m.tasksClaimed.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.String("severity", severity),
		),
	)

	if recovered {
		m.taskRecoveries.Add(
			ctx,
			1,
			metric.WithAttributes(
				attribute.String("severity", severity),
			),
		)
	}
}

func (m *Metrics) RecordLeaseRenewal(
	ctx context.Context,
) {
	if m == nil {
		return
	}

	m.leaseRenewals.Add(ctx, 1)
}

func (m *Metrics) RecordNotification(
	ctx context.Context,
	status string,
	channel string,
) {
	if m == nil {
		return
	}

	m.notifications.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.String("status", status),
			attribute.String("channel", channel),
		),
	)
}

func (m *Metrics) RecordEscalation(
	ctx context.Context,
	level int,
) {
	if m == nil {
		return
	}

	m.escalations.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.Int("level", level),
		),
	)
}

func (m *Metrics) RecordAcknowledgement(
	ctx context.Context,
	severity string,
	latency time.Duration,
) {
	if m == nil {
		return
	}

	attributes := metric.WithAttributes(
		attribute.String("severity", severity),
	)

	m.acknowledgements.Add(
		ctx,
		1,
		attributes,
	)

	if latency > 0 {
		m.acknowledgementLatency.Record(
			ctx,
			latency.Seconds(),
			attributes,
		)
	}
}

func (m *Metrics) RecordProcessingDuration(
	ctx context.Context,
	severity string,
	duration time.Duration,
) {
	if m == nil {
		return
	}

	m.workerProcessingDuration.Record(
		ctx,
		duration.Seconds(),
		metric.WithAttributes(
			attribute.String("severity", severity),
		),
	)
}
