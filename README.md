# Clinical Results Escalation Engine

A fault-tolerant clinical workflow engine built with Go, PostgreSQL and OpenTelemetry.

The system ingests synthetic clinical results, classifies their urgency, creates durable review tasks, sends idempotent notifications, waits for clinician acknowledgement and escalates unacknowledged tasks through configurable responsibility levels.

> **Educational project only:** This application uses synthetic data and demonstration-only rules. It is not a medical device, does not provide clinical advice and must not be used with real patient data.

---

## Overview

Hospital workflows often require more than storing a result. A result must be routed to the correct team, acknowledged within a deadline, escalated when no response is received and recorded in an auditable history.

This project models that workflow:

```text
Result received
    ↓
Validate and classify urgency
    ↓
Create durable clinical task
    ↓
Notify responsible team
    ↓
Await acknowledgement
    ↓
Acknowledge or escalate
    ↓
Record every transition
```

Example synthetic result:

```json
{
  "source_system": "laboratory-simulator",
  "source_result_id": "LIMS-839211",
  "patient_reference": "P-1042",
  "test_code": "serum_potassium",
  "value": 6.8,
  "unit": "mmol/L",
  "reported_at": "2026-07-24T14:30:00Z"
}
```

The result may create a critical review task assigned to `acute-medicine`, followed by escalation to the medical registrar, consultant on call and site operations team if acknowledgement deadlines are missed.

---

## Architecture

```mermaid
flowchart TD
    L[Laboratory Simulator] -->|POST /v1/results| API[Results Ingestion API]

    API --> V[Validation and Classification]
    V --> DB[(PostgreSQL)]

    DB --> R[Clinical Results]
    DB --> T[Clinical Tasks]
    DB --> N[Notification Attempts]
    DB --> A[Append-only Audit Events]

    T --> W1[Notification Worker A]
    T --> W2[Notification Worker B]
    T --> W3[Notification Worker N]

    W1 --> P[Fake Notification Provider]
    W2 --> P
    W3 --> P

    P --> ACK[Awaiting Acknowledgement]

    ACK -->|Clinician responds| API
    ACK -->|Deadline expires| S[Escalation Scheduler]

    S --> T
```

### Observability

```mermaid
flowchart LR
    API[API] -->|OTLP| OTel[OpenTelemetry Collector]
    Worker[Worker] -->|OTLP| OTel
    Scheduler[Scheduler] -->|OTLP| OTel

    OTel --> Jaeger[Jaeger]
    OTel --> Prometheus[Prometheus]
    Prometheus --> Grafana[Grafana]
```

---

## Core engineering features

### Transactional workflow creation

Result ingestion creates the following in one PostgreSQL transaction:

- clinical result;
- review task;
- `result_received` audit event;
- `result_classified` audit event;
- `task_created` audit event.

If task creation or audit insertion fails, the result insert is rolled back.

This prevents a clinical result from being stored without creating the work required to review it.

### Multi-worker task claiming

Workers claim tasks using:

```sql
FOR UPDATE SKIP LOCKED
```

This allows multiple worker processes to compete for tasks without claiming the same row concurrently.

Tasks are selected in this order:

1. critical;
2. urgent;
3. routine;
4. oldest available task within the same severity.

### Renewable leases

A claimed task stores:

```text
lease_owner
lease_expires_at
```

Workers periodically renew their leases while processing.

If a worker stops or crashes:

```text
renewals stop
→ lease expires
→ another worker recovers the task
```

All worker updates verify that:

```text
status = processing
lease_owner = current worker
lease has not expired
```

This prevents a stale worker from updating a task after another worker has recovered it.

### Idempotent notification delivery

Each logical notification uses a deterministic idempotency key:

```text
task:{task_id}:level:{level}:recipient:{recipient}:channel:{channel}
```

Notification delivery is at-least-once, but duplicate external effects are controlled through provider-side idempotency.

The fake provider can simulate:

- successful delivery;
- temporary failure;
- permanent failure;
- accepted delivery followed by a lost response.

In the lost-response scenario, the worker retries using the same idempotency key and the provider returns the original delivery rather than creating another one.

### Acknowledgement and escalation

After notification delivery, a task enters:

```text
awaiting_ack
```

The acknowledgement endpoint requires:

```text
status = awaiting_ack
version = expected_version
```

The escalation scheduler also operates on the task version and locks overdue rows before updating them.

This ensures that an acknowledgement and escalation racing against the same task version cannot both succeed.

### Maximum escalation handling

The demonstration escalation chain is:

```text
Level 0: responsible team
Level 1: medical registrar
Level 2: consultant on call
Level 3: site operations team
```

If the final level also misses its deadline, the task is marked `failed` and requires manual intervention rather than escalating indefinitely.

### Append-only audit history

Domain transitions are recorded as structured audit events, including:

```text
result_received
result_classified
task_created
task_claimed
task_recovered_after_lease_expiry
notification_requested
notification_delivered
notification_temporary_failed
notification_permanent_failed
task_awaiting_acknowledgement
task_acknowledged
acknowledgement_deadline_missed
task_escalated
task_escalation_exhausted
task_failed
```

The audit log is separate from application logs and represents durable workflow history.

---

## Task state machine

```mermaid
stateDiagram-v2
    [*] --> pending

    pending --> processing: worker claim
    escalated --> processing: worker claim
    processing --> pending: retryable failure
    processing --> awaiting_ack: notification delivered
    processing --> failed: permanent failure

    awaiting_ack --> acknowledged: clinician acknowledgement
    awaiting_ack --> pending: acknowledgement deadline missed
    awaiting_ack --> failed: maximum escalation exhausted

    acknowledged --> completed
```

---

## Technology stack

- **Go** — API, workers, scheduler and load generator
- **PostgreSQL 17** — workflow state, queue coordination and audit history
- **pgx** — explicit PostgreSQL transactions and locking
- **Chi** — HTTP routing
- **Docker Compose** — local infrastructure
- **OpenTelemetry** — tracing and metrics
- **OpenTelemetry Collector** — telemetry processing and export
- **Jaeger** — distributed trace visualisation
- **Prometheus** — metric storage and querying
- **Grafana** — dashboards

---

## Repository structure

```text
clinical-results-escalation-engine/
├── cmd/
│   ├── api/
│   ├── worker/
│   ├── scheduler/
│   └── loadgen/
│
├── internal/
│   ├── api/
│   ├── audit/
│   ├── escalation/
│   ├── notification/
│   ├── platform/
│   │   ├── database/
│   │   └── telemetry/
│   ├── result/
│   └── task/
│
├── migrations/
├── deployments/
├── tests/
│   └── integration/
├── docs/
│   └── images/
├── go.mod
└── README.md
```

---

## Running locally

### Requirements

Install:

- Go;
- Docker Desktop;
- Git.

### 1. Clone the repository

```bash
git clone git@github.com:shraddharanjan/clinical-results-escalation-engine.git
cd clinical-results-escalation-engine
```

### 2. Create the local environment file

PowerShell:

```powershell
Copy-Item .env.example .env
```

The default local configuration uses:

```env
HTTP_PORT=8080
DATABASE_URL=postgres://clinical_user:clinical_password@localhost:5432/clinical_results?sslmode=disable

WORKER_POLL_INTERVAL=500ms
WORKER_LEASE_DURATION=30s
WORKER_RENEWAL_INTERVAL=10s
WORKER_RETRY_DELAY=30s

FAKE_NOTIFICATION_MODE=success
FAKE_NOTIFICATION_LATENCY=100ms

SCHEDULER_POLL_INTERVAL=1s

APP_ENVIRONMENT=development
APP_VERSION=0.1.0

OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
OTEL_EXPORTER_OTLP_INSECURE=true
```

### 3. Start infrastructure

```powershell
docker compose -f deployments\docker-compose.yml up -d
```

Check the containers:

```powershell
docker compose -f deployments\docker-compose.yml ps
```

### 4. Start the API

```powershell
go run ./cmd/api
```

### 5. Start a worker

In a second terminal:

```powershell
go run ./cmd/worker
```

### 6. Start the escalation scheduler

In a third terminal:

```powershell
go run ./cmd/scheduler
```

---

## API usage

### Health check

```http
GET /health
```

### Submit a result

```http
POST /v1/results
Content-Type: application/json
```

```json
{
  "source_system": "laboratory-simulator",
  "source_result_id": "LIMS-DEMO-001",
  "patient_reference": "P-1042",
  "test_code": "serum_potassium",
  "value": 6.8,
  "unit": "mmol/L",
  "reported_at": "2026-07-24T14:30:00Z"
}
```

PowerShell:

```powershell
$body = @{
    source_system      = "laboratory-simulator"
    source_result_id   = "LIMS-DEMO-001"
    patient_reference = "P-1042"
    test_code          = "serum_potassium"
    value              = 6.8
    unit               = "mmol/L"
    reported_at        = (Get-Date).ToUniversalTime().AddMinutes(-1).ToString("o")
} | ConvertTo-Json

Invoke-RestMethod `
  -Method Post `
  -Uri "http://localhost:8080/v1/results" `
  -ContentType "application/json" `
  -Body $body
```

### Acknowledge a task

```http
POST /v1/tasks/{task_id}/acknowledgements
Content-Type: application/json
```

```json
{
  "clinician_id": "clinician-42",
  "expected_version": 3
}
```

A stale version or invalid state returns:

```http
409 Conflict
```

---

## Failure simulation

The fake notification provider supports:

```env
FAKE_NOTIFICATION_MODE=success
FAKE_NOTIFICATION_MODE=temporary
FAKE_NOTIFICATION_MODE=permanent
FAKE_NOTIFICATION_MODE=lost_response
```

Example:

```powershell
$env:FAKE_NOTIFICATION_MODE="lost_response"
$env:WORKER_RETRY_DELAY="5s"

go run ./cmd/worker
```

---

## Observability

### Grafana

Open:

```text
http://localhost:3000
```

Default local credentials:

```text
Username: admin
Password: admin
```

Prometheus data-source URL from inside Docker:

```text
http://prometheus:9090
```

![Clinical workflow dashboard](docs/images/grafana-clinical-workflow-dashboard.png)

*Workflow and notification metrics exported through OpenTelemetry and visualised in Grafana.*

### Jaeger

Open:

```text
http://localhost:16686
```

#### Result-ingestion trace

![Result ingestion trace](docs/images/jaeger-result-ingestion-trace.png)

*HTTP and application spans for transactional creation of a clinical result and review task.*

#### Notification worker trace

![Notification worker trace](docs/images/jaeger-notification-trace.png)

*Worker task processing with a nested notification-delivery span.*

#### Idempotent retry

![Idempotent notification retry](docs/images/jaeger-idempotent-retry.png)

*A repeated send reconciled with an existing provider delivery using the same idempotency key.*

---

## Testing

### Unit tests

```powershell
go test ./...
```

### Static checks

```powershell
go fmt ./...
go vet ./...
```

### Integration database

```powershell
docker compose -f deployments\docker-compose.yml up -d postgres-test
```

Configure:

```powershell
$env:TEST_DATABASE_URL="postgres://clinical_test_user:clinical_test_password@localhost:5433/clinical_results_test?sslmode=disable"
```

Run:

```powershell
go test ./tests/integration -count=1 -v
```

![Integration tests](docs/images/integration-tests.png)

The concurrency test starts multiple workers against the same queue and verifies that every task is claimed once without duplicate `task_claimed` events.

---

## Load benchmark

The load generator sends concurrent synthetic result-ingestion requests:

```powershell
$env:LOAD_REQUEST_COUNT="1000"
$env:LOAD_CONCURRENCY="50"

go run ./cmd/loadgen
```

Measured local result:

| Metric | Result |
|---|---:|
| Requests | 1,000 |
| Concurrency | 50 |
| Successful | 1,000 |
| Failed | 0 |
| Total duration | 4.10 s |
| Throughput | 243.61 requests/second |
| p50 latency | 171 ms |
| p95 latency | 330 ms |
| p99 latency | 758 ms |
| Maximum latency | 780 ms |
![Load-test results](docs/images/load-test-results.png)

> This benchmark measures the result-ingestion endpoint on a local Windows and Docker Desktop environment. Each request performs validation, classification and transactional creation of the result, task and initial audit events. It is not a benchmark of complete end-to-end workflow completion.

---

## Important design decisions

### Why PostgreSQL instead of Redis?

Clinical tasks are durable workflow state, not ephemeral messages.

PostgreSQL allows result creation, task creation and audit insertion to commit atomically. It also provides row locking, delayed availability and lease storage without introducing another authoritative system.

Redis could later be used for short-lived caching, throttling or presence information, but not as the workflow source of truth.

### Why PostgreSQL instead of Kafka?

The core workflow requires immediate transactional consistency between:

```text
result
task
audit events
```

Kafka would introduce a database-and-broker dual-write problem.

A future Kafka integration would use a transactional outbox and would be limited to downstream events such as analytics, search indexing or external integration.

### Why a modular monolith?

The API, worker and scheduler run as separate processes, but share domain packages and one PostgreSQL database.

This provides independent execution and horizontal worker scaling without introducing premature service-to-service communication and deployment complexity.

### Why at-least-once rather than exactly-once?

Workers may retry after crashes, timeouts or ambiguous provider responses.

Exactly-once side effects cannot be guaranteed across independent systems without cooperation from the downstream provider.

The engine therefore uses:

```text
at-least-once task attempts
+
deterministic idempotency keys
+
provider-side deduplication
```

### Why version checks?

Acknowledgement and escalation may race.

Both operations require the expected task status and version, and both increment the version. Only one transition can succeed against a given task version.

---

## Limitations

This project intentionally does not include:

- real patient data;
- real clinical thresholds;
- production authentication;
- real NHS or hospital-system integration;
- regulatory compliance;
- mobile push infrastructure;
- production-grade secrets management;
- multi-region deployment.

The clinical rules, teams and deadlines are fictional development examples.

---

## Possible extensions

- clinician task-inbox UI;
- authentication and role-based access;
- configurable versioned classification rules;
- configurable escalation policies;
- Kubernetes deployment and pod-failure recovery demonstration;
- Kafka integration using a transactional outbox;
- FHIR-inspired synthetic adapters;
- manual-intervention queue for failed tasks;
- trace-context propagation through PostgreSQL;
- queue-depth and overdue-task alerts.

---

## Interview discussion points

This project was designed to support discussion of:

- `FOR UPDATE SKIP LOCKED`;
- renewable leases;
- stale-worker protection;
- idempotent external effects;
- acknowledgement and escalation races;
- optimistic concurrency;
- transactional audit trails;
- modular monolith versus microservices;
- PostgreSQL queue versus Redis or Kafka;
- metrics cardinality;
- failure injection and recovery testing.

---

## Licence

This project is intended for educational and portfolio use.
