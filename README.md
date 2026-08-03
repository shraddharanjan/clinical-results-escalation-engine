# Clinical Results Escalation Engine

A fault-tolerant clinical workflow engine built with Go, PostgreSQL, React and OpenTelemetry.

The system accepts synthetic clinical results, classifies urgency, creates durable review tasks, delivers idempotent notifications, waits for clinician acknowledgement and escalates unacknowledged tasks through configurable responsibility levels.

> **Educational project only:** This application uses synthetic data and demonstration-only rules. It is not a medical device, does not provide clinical advice and must not be used with real patient data.

## Live demo

- **Clinician dashboard:** (https://clinical-results-escalation-engine.vercel.app/)
- **Public API:** `https://clinical-results-api.onrender.com`
- **Health check:** `https://clinical-results-api.onrender.com/health`

The hosted demo uses a Vite/React frontend on Vercel, a containerised Go API on Render and PostgreSQL on Neon. The complete worker, scheduler and observability stack can also be run locally with Docker Compose.

Because the public API uses a free hosting tier, the first request after inactivity may take longer while the service wakes up.

## Demonstration flow

```text
Submit synthetic potassium result: 6.8 mmol/L
        ↓
Go API validates and classifies it as critical
        ↓
PostgreSQL transaction creates result, task and audit events
        ↓
Worker claims task and performs idempotent notification delivery
        ↓
Task waits for clinician acknowledgement
        ↓
Clinician acknowledges or scheduler escalates after the deadline
```

## Clinician dashboard

The React dashboard provides:

- live result and task data from the Go API;
- severity and status filtering;
- patient-reference and task search;
- synthetic result submission;
- task details and acknowledgement deadlines;
- clinician acknowledgement using optimistic version checks;
- escalation and status views;
- automatic polling of PostgreSQL-backed state;
- responsive desktop and mobile layouts.

![Clinician dashboard](docs/images/clinical-dashboard-ui.png)

## Architecture

```mermaid
flowchart TD
    UI[React clinician dashboard] -->|HTTPS| API[Go API]
    API --> V[Validation and classification]
    V --> DB[(PostgreSQL)]

    DB --> R[Clinical results]
    DB --> T[Clinical tasks]
    DB --> N[Notification attempts]
    DB --> A[Append-only audit events]

    T --> W1[Worker A]
    T --> W2[Worker B]
    W1 --> P[Notification provider]
    W2 --> P

    P --> ACK[Awaiting acknowledgement]
    ACK -->|Clinician responds| API
    ACK -->|Deadline expires| S[Escalation scheduler]
    S --> T
```

### Local observability

```mermaid
flowchart LR
    API[API] -->|OTLP| OTel[OpenTelemetry Collector]
    Worker[Worker] -->|OTLP| OTel
    Scheduler[Scheduler] -->|OTLP| OTel
    OTel --> Jaeger[Jaeger]
    OTel --> Prometheus[Prometheus]
    Prometheus --> Grafana[Grafana]
```

OpenTelemetry export can be disabled in environments without a collector:

```env
OTEL_ENABLED=false
```

## Core engineering features

### Transactional workflow creation

Result ingestion creates the clinical result, review task and initial audit events in one PostgreSQL transaction. If any part fails, the whole operation rolls back.

### Multi-worker task claiming

Workers use:

```sql
FOR UPDATE SKIP LOCKED
```

This allows multiple processes to compete for work without claiming the same task concurrently. Selection is severity-aware and FIFO within the same severity.

### Renewable leases and crash recovery

Claimed tasks store a lease owner and expiry time. Workers renew leases while processing. If a worker crashes, renewal stops and another worker can recover the task after expiry.

All worker updates verify ownership and lease validity to prevent a stale worker from updating a recovered task.

### Idempotent notifications

Each logical notification uses a deterministic key:

```text
task:{task_id}:level:{level}:recipient:{recipient}:channel:{channel}
```

The workflow is at-least-once, while provider-side idempotency prevents duplicate external effects after retries or ambiguous responses.

### Acknowledgement and escalation races

Acknowledgement requires the expected task version and valid state. The scheduler also checks and increments the task version. Only one transition can win against a given version.

### Append-only audit history

Durable audit events include:

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
    awaiting_ack --> pending: deadline missed and escalation available
    awaiting_ack --> failed: maximum escalation exhausted
    acknowledged --> completed
```

## Technology stack

- **Go** — API, worker, scheduler and load generator
- **React + TypeScript + Vite** — clinician dashboard
- **PostgreSQL 17 / Neon** — workflow state, queue coordination and audit history
- **pgx** — explicit transactions and PostgreSQL locking
- **Chi** — HTTP routing and middleware
- **Docker / Docker Compose** — service packaging and local infrastructure
- **OpenTelemetry** — tracing and metrics
- **Jaeger** — distributed trace visualisation
- **Prometheus** — metrics collection
- **Grafana** — operational dashboards
- **Render** — hosted Go API
- **Vercel** — hosted frontend

## Repository structure

```text
clinical-results-escalation-engine/
├── cmd/
│   ├── api/
│   ├── worker/
│   ├── scheduler/
│   └── loadgen/
├── internal/
│   ├── api/
│   ├── audit/
│   ├── escalation/
│   ├── notification/
│   ├── platform/
│   ├── result/
│   └── task/
├── migrations/
├── deployments/
├── tests/
│   └── integration/
├── docs/
│   └── images/
├── ui/
├── Dockerfile
├── go.mod
└── README.md
```

## Running locally

### Requirements

- Go
- Node.js 22 or newer
- Docker Desktop
- Git

### Start infrastructure

```powershell
docker compose -f deployments\docker-compose.yml up -d
```

### Start backend processes

API:

```powershell
go run ./cmd/api
```

Worker:

```powershell
go run ./cmd/worker
```

Scheduler:

```powershell
go run ./cmd/scheduler
```

### Start the UI

```powershell
cd ui
npm install
Copy-Item .env.example .env.local
npm run dev
```

Example `ui/.env.local`:

```env
VITE_API_URL=http://localhost:8080
VITE_USE_MOCKS=false
```

Open `http://localhost:5173`.

## API

### Read results and tasks

```http
GET /v1/results
GET /v1/tasks
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
  "patient_reference": "P-DEMO-1042",
  "test_code": "serum_potassium",
  "value": 6.8,
  "unit": "mmol/L",
  "reported_at": "2026-07-24T14:30:00Z"
}
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

A stale version or invalid state returns `409 Conflict`.

## Failure simulation

The fake notification provider supports:

```env
FAKE_NOTIFICATION_MODE=success
FAKE_NOTIFICATION_MODE=temporary
FAKE_NOTIFICATION_MODE=permanent
FAKE_NOTIFICATION_MODE=lost_response
```

The lost-response scenario simulates an accepted delivery whose response is lost. A retry with the same idempotency key reconciles with the original delivery.

## Observability

### Grafana

Open `http://localhost:3000`.

![Clinical workflow dashboard](docs/images/grafana-clinical-workflow-dashboard.png)

### Jaeger

Open `http://localhost:16686`.

![Result ingestion trace](docs/images/jaeger-result-ingestion-trace.png)

![Notification worker trace](docs/images/jaeger-notification-trace.png)

![Idempotent notification retry](docs/images/jaeger-idempotent-retry.png)

## Testing

```powershell
go fmt ./...
go vet ./...
go test ./cmd/... ./internal/... ./tests/...
```

For the integration database:

```powershell
docker compose -f deployments\docker-compose.yml up -d postgres-test

$env:TEST_DATABASE_URL="postgres://clinical_test_user:clinical_test_password@localhost:5433/clinical_results_test?sslmode=disable"

go test ./tests/integration -count=1 -v
```

![Integration tests](docs/images/integration-tests.png)

## Load benchmark

Local synthetic ingestion benchmark:

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

The benchmark covers validation, classification and transactional creation of the result, task and initial audit events on a local Windows and Docker Desktop environment.


## Limitations

This project intentionally does not include:

- real patient data or real clinical thresholds;
- production authentication or authorisation;
- NHS or hospital-system integration;
- regulatory compliance;
- real mobile push infrastructure;
- production-grade secrets rotation;
- multi-region deployment.


## Licence

Educational and portfolio use.
