CREATE TYPE task_status AS ENUM (
    'pending',
    'processing',
    'awaiting_ack',
    'acknowledged',
    'completed',
    'escalated',
    'failed'
);

CREATE TABLE clinical_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    result_id UUID NOT NULL
        REFERENCES clinical_results(id),

    task_type TEXT NOT NULL,
    status task_status NOT NULL DEFAULT 'pending',
    severity result_severity NOT NULL,

    assigned_team TEXT NOT NULL,
    assigned_user TEXT,

    escalation_level INTEGER NOT NULL DEFAULT 0,

    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledgement_due_at TIMESTAMPTZ,

    lease_owner TEXT,
    lease_expires_at TIMESTAMPTZ,

    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,

    version BIGINT NOT NULL DEFAULT 1,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT clinical_tasks_escalation_level_non_negative
        CHECK (escalation_level >= 0),

    CONSTRAINT clinical_tasks_attempt_count_non_negative
        CHECK (attempt_count >= 0)
);

CREATE INDEX clinical_tasks_result_id_idx
    ON clinical_tasks (result_id);

CREATE INDEX clinical_tasks_pending_idx
    ON clinical_tasks (
        severity,
        available_at,
        created_at
    )
    WHERE status IN ('pending', 'escalated');

CREATE INDEX clinical_tasks_ack_deadline_idx
    ON clinical_tasks (acknowledgement_due_at)
    WHERE status = 'awaiting_ack';

CREATE TABLE audit_events (
    sequence_number BIGSERIAL PRIMARY KEY,

    event_id UUID NOT NULL
        DEFAULT gen_random_uuid()
        UNIQUE,

    aggregate_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,

    event_type TEXT NOT NULL,

    actor_type TEXT NOT NULL,
    actor_id TEXT,

    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    payload JSONB NOT NULL
);

CREATE INDEX audit_events_aggregate_idx
    ON audit_events (
        aggregate_type,
        aggregate_id,
        sequence_number
    );