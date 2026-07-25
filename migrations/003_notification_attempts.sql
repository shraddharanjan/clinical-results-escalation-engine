CREATE TABLE notification_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    task_id UUID NOT NULL
        REFERENCES clinical_tasks(id),

    escalation_level INTEGER NOT NULL,

    recipient TEXT NOT NULL,
    channel TEXT NOT NULL,

    idempotency_key TEXT NOT NULL UNIQUE,

    status TEXT NOT NULL,

    provider_reference TEXT,

    attempt_count INTEGER NOT NULL DEFAULT 0,

    last_error TEXT,
    next_attempt_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ,

    CONSTRAINT notification_attempt_status_valid
        CHECK (
            status IN (
                'pending',
                'delivered',
                'temporary_failed',
                'permanent_failed'
            )
        ),

    CONSTRAINT notification_attempt_count_non_negative
        CHECK (attempt_count >= 0)
);

CREATE INDEX notification_attempts_task_idx
    ON notification_attempts (
        task_id,
        escalation_level,
        created_at
    );

CREATE INDEX notification_attempts_retry_idx
    ON notification_attempts (next_attempt_at)
    WHERE status = 'temporary_failed';

-- This table simulates a downstream provider that supports
-- idempotency keys. It is separate from our own attempt table
-- because a real provider would be a separate system.
CREATE TABLE fake_provider_deliveries (
    idempotency_key TEXT PRIMARY KEY,

    provider_reference UUID NOT NULL
        DEFAULT gen_random_uuid(),

    recipient TEXT NOT NULL,
    channel TEXT NOT NULL,
    message_body TEXT NOT NULL,

    accepted_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    lost_response_simulated BOOLEAN NOT NULL DEFAULT FALSE
);