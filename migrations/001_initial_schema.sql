CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE result_severity AS ENUM (
    'routine',
    'urgent',
    'critical'
);

CREATE TABLE clinical_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    source_system TEXT NOT NULL,
    source_result_id TEXT NOT NULL,

    patient_reference TEXT NOT NULL,
    test_code TEXT NOT NULL,

    numeric_value NUMERIC NOT NULL,
    unit TEXT NOT NULL,

    reported_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    severity result_severity NOT NULL,
    matched_rule TEXT,

    raw_payload JSONB NOT NULL,

    UNIQUE (source_system, source_result_id)
);