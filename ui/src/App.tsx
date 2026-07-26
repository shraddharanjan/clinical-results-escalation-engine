import {
  Activity,
  Bell,
  BellRing,
  CheckCircle2,
  CircleAlert,
  CircleHelp,
  ClipboardList,
  Clock3,
  FilePlus2,
  Gauge,
  HeartPulse,
  RefreshCw,
  Search,
  Settings,
  ShieldAlert,
  Stethoscope,
  X,
} from "lucide-react";
import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";

import {
  acknowledgeTask,
  createResult,
  getResults,
  getTasks,
  type ApiResult,
  type ApiTask,
} from "./api";

type Severity =
  | "critical"
  | "urgent"
  | "routine";

type TaskStatus =
  | "pending"
  | "processing"
  | "awaiting_ack"
  | "acknowledged"
  | "completed"
  | "escalated"
  | "failed";

type Page =
  | "dashboard"
  | "tasks"
  | "results"
  | "escalations"
  | "submit-result"
  | "notifications"
  | "metrics"
  | "settings";

type AuditEventType =
  | "result_received"
  | "result_classified"
  | "task_created"
  | "notification_delivered"
  | "task_acknowledged"
  | "acknowledgement_deadline_missed"
  | "task_escalated";

interface ClinicalResult {
  id: string;
  patientReference: string;
  test: string;
  value: number;
  unit: string;
  severity: Severity;
  sourceSystem: string;
  sourceResultId: string;
  reportedAt: string;
  receivedAt: string;
  matchedRule: string;
}

interface ClinicalTask {
  id: string;
  resultId: string;
  patientReference: string;
  test: string;
  value: number;
  unit: string;
  severity: Severity;
  status: TaskStatus;
  assignedTeam: string;
  dueAt?: string;
  escalationLevel: number;
  version: number;
  reportedAt: string;
}

interface AuditEvent {
  id: string;
  taskId?: string;
  resultId?: string;
  type: AuditEventType;
  description: string;
  timestamp: string;
}

interface SubmitResultForm {
  patientReference: string;
  test: string;
  value: string;
  unit: string;
  sourceSystem: string;
  sourceResultId: string;
}

const initialSubmitForm: SubmitResultForm = {
  patientReference: "P-DEMO-2048",
  test: "serum_potassium",
  value: "6.8",
  unit: "mmol/L",
  sourceSystem: "laboratory-simulator",
  sourceResultId: `LIMS-DEMO-${Date.now()}`,
};

const navigationGroups = [
  {
    label: "Overview",
    items: [
      {
        id: "dashboard" as Page,
        label: "Dashboard",
        icon: Gauge,
      },
      {
        id: "tasks" as Page,
        label: "Tasks",
        icon: ClipboardList,
      },
      {
        id: "results" as Page,
        label: "Results",
        icon: HeartPulse,
      },
      {
        id: "escalations" as Page,
        label: "Escalations",
        icon: ShieldAlert,
      },
    ],
  },
  {
    label: "Operations",
    items: [
      {
        id: "submit-result" as Page,
        label: "Submit result",
        icon: Stethoscope,
      },
      {
        id: "notifications" as Page,
        label: "Notifications",
        icon: BellRing,
      },
    ],
  },
  {
    label: "Observability",
    items: [
      {
        id: "metrics" as Page,
        label: "Metrics",
        icon: Activity,
      },
      {
        id: "settings" as Page,
        label: "Settings",
        icon: Settings,
      },
    ],
  },
];

const activityIcons = {
  result_received: HeartPulse,
  result_classified: Activity,
  task_created: FilePlus2,
  notification_delivered: BellRing,
  task_acknowledged: CheckCircle2,
  acknowledgement_deadline_missed:
    Clock3,
  task_escalated: CircleAlert,
};

function formatLabel(
  value: string,
): string {
  return value.replaceAll("_", " ");
}

function mapApiResult(
  result: ApiResult,
): ClinicalResult {
  return {
    id: result.id,
    patientReference:
      result.patient_reference,
    test: result.test_code,
    value: result.value,
    unit: result.unit,
    severity: result.severity,
    sourceSystem: result.source_system,
    sourceResultId:
      result.source_result_id,
    reportedAt: result.reported_at,
    receivedAt: result.received_at,
    matchedRule:
      result.matched_rule ??
      "No matched rule returned",
  };
}

function mapApiTask(
  task: ApiTask,
  resultById: Map<
    string,
    ClinicalResult
  >,
): ClinicalTask {
  const linkedResult =
    resultById.get(task.result_id);

  return {
    id: task.id,
    resultId: task.result_id,

    patientReference:
      task.patient_reference ??
      linkedResult?.patientReference ??
      "Unknown",

    test:
      task.test_code ??
      linkedResult?.test ??
      "clinical_result",

    value:
      task.value ??
      linkedResult?.value ??
      0,

    unit:
      task.unit ??
      linkedResult?.unit ??
      "",

    severity: task.severity,
    status: task.status,
    assignedTeam: task.assigned_team,

    dueAt:
      task.acknowledgement_due_at ??
      undefined,

    escalationLevel:
      task.escalation_level,

    version: task.version,

    reportedAt:
      task.reported_at ??
      linkedResult?.reportedAt ??
      task.created_at,
  };
}

function dueLabel(
  dueAt: string | undefined,
  currentTime: number,
): string {
  if (!dueAt) {
    return "—";
  }

  const milliseconds =
    new Date(dueAt).getTime() -
    currentTime;

  if (milliseconds <= 0) {
    return "Overdue";
  }

  const minutes = Math.floor(
    milliseconds / 60_000,
  );

  const seconds = Math.floor(
    (milliseconds % 60_000) / 1000,
  );

  return `${minutes}m ${String(
    seconds,
  ).padStart(2, "0")}s`;
}

interface SidebarProps {
  activePage: Page;
  taskCount: number;
  escalationCount: number;
  onPageChange: (page: Page) => void;
}

function Sidebar({
  activePage,
  taskCount,
  escalationCount,
  onPageChange,
}: SidebarProps) {
  return (
    <aside className="sidebar">
      <div className="brand">
        <span className="brand-mark">
          <HeartPulse size={24} />
        </span>

        <span>Clinical Escalation</span>
      </div>

      <nav>
        {navigationGroups.map(
          (group) => (
            <div
              className="nav-group"
              key={group.label}
            >
              <div className="nav-label">
                {group.label}
              </div>

              {group.items.map((item) => {
                const Icon = item.icon;
                const isActive =
                  activePage === item.id;

                let badge: number | null =
                  null;

                if (item.id === "tasks") {
                  badge = taskCount;
                }

                if (
                  item.id ===
                  "escalations"
                ) {
                  badge =
                    escalationCount;
                }

                return (
                  <button
                    className={`nav-item ${
                      isActive
                        ? "active"
                        : ""
                    }`}
                    key={item.id}
                    type="button"
                    onClick={() =>
                      onPageChange(item.id)
                    }
                  >
                    <Icon size={17} />
                    <span>
                      {item.label}
                    </span>

                    {badge !== null &&
                    badge > 0 ? (
                      <span className="nav-badge">
                        {badge}
                      </span>
                    ) : null}
                  </button>
                );
              })}
            </div>
          ),
        )}
      </nav>

      <div className="sidebar-footer">
        <span>
          Clinical Escalation Engine
        </span>
        <small>
          Live Go API demonstration
        </small>
      </div>
    </aside>
  );
}

interface MetricCardProps {
  title: string;
  value: string;
  tone:
    | "blue"
    | "purple"
    | "red"
    | "amber"
    | "green";
  caption: string;
}

function MetricCard({
  title,
  value,
  tone,
  caption,
}: MetricCardProps) {
  return (
    <article
      className={`metric-card tone-${tone}`}
    >
      <div className="metric-title">
        {title}
      </div>

      <div className="metric-value">
        {value}
      </div>

      <div className="metric-caption">
        {caption}
      </div>

      <svg
        className="sparkline"
        viewBox="0 0 200 50"
        aria-hidden="true"
      >
        <polyline
          fill="none"
          points="0,38 14,25 27,34 43,19 58,31 72,23 86,29 103,15 117,24 132,19 146,22 161,12 176,17 200,6"
          stroke="currentColor"
          strokeWidth="2"
        />
      </svg>
    </article>
  );
}

interface TaskTableProps {
  tasks: ClinicalTask[];
  currentTime: number;
  severityFilter:
    | "all"
    | Severity;
  statusFilter:
    | "all"
    | TaskStatus;
  query: string;

  onSeverityChange: (
    value: "all" | Severity,
  ) => void;

  onStatusChange: (
    value: "all" | TaskStatus,
  ) => void;

  onQueryChange: (
    value: string,
  ) => void;

  onSelect: (
    task: ClinicalTask,
  ) => void;
}

function TaskTable({
  tasks,
  currentTime,
  severityFilter,
  statusFilter,
  query,
  onSeverityChange,
  onStatusChange,
  onQueryChange,
  onSelect,
}: TaskTableProps) {
  return (
    <section className="panel tasks-panel">
      <div className="panel-heading task-toolbar">
        <div>
          <h2>Clinical tasks</h2>
          <p>
            Tasks loaded from the Go API and
            PostgreSQL.
          </p>
        </div>

        <div className="filters">
          <select
            value={severityFilter}
            onChange={(event) =>
              onSeverityChange(
                event.target.value as
                  | "all"
                  | Severity,
              )
            }
          >
            <option value="all">
              All severities
            </option>

            <option value="critical">
              Critical
            </option>

            <option value="urgent">
              Urgent
            </option>

            <option value="routine">
              Routine
            </option>
          </select>

          <select
            value={statusFilter}
            onChange={(event) =>
              onStatusChange(
                event.target.value as
                  | "all"
                  | TaskStatus,
              )
            }
          >
            <option value="all">
              All statuses
            </option>

            <option value="awaiting_ack">
              Awaiting acknowledgement
            </option>

            <option value="processing">
              Processing
            </option>

            <option value="pending">
              Pending
            </option>

            <option value="escalated">
              Escalated
            </option>

            <option value="acknowledged">
              Acknowledged
            </option>

            <option value="failed">
              Failed
            </option>
          </select>

          <label className="search-input">
            <Search size={16} />

            <input
              value={query}
              onChange={(event) =>
                onQueryChange(
                  event.target.value,
                )
              }
              placeholder="Search tasks"
            />
          </label>
        </div>
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Task</th>
              <th>Patient</th>
              <th>Result</th>
              <th>Severity</th>
              <th>Status</th>
              <th>Assigned team</th>
              <th>Due</th>
              <th />
            </tr>
          </thead>

          <tbody>
            {tasks.map((task) => (
              <tr key={task.id}>
                <td className="mono">
                  {task.id.slice(0, 8)}
                </td>

                <td>
                  {
                    task.patientReference
                  }
                </td>

                <td>
                  <strong>
                    {formatLabel(
                      task.test,
                    )}
                  </strong>

                  <span className="result-value">
                    {task.value}{" "}
                    {task.unit}
                  </span>
                </td>

                <td>
                  <span
                    className={`badge severity ${task.severity}`}
                  >
                    {task.severity}
                  </span>
                </td>

                <td>
                  <span
                    className={`badge status ${task.status}`}
                  >
                    {formatLabel(
                      task.status,
                    )}

                    {task.escalationLevel >
                    0
                      ? ` · L${task.escalationLevel}`
                      : ""}
                  </span>
                </td>

                <td>
                  {task.assignedTeam}
                </td>

                <td
                  className={
                    task.dueAt
                      ? "due"
                      : ""
                  }
                >
                  {dueLabel(
                    task.dueAt,
                    currentTime,
                  )}
                </td>

                <td>
                  <button
                    className="ghost-button"
                    type="button"
                    onClick={() =>
                      onSelect(task)
                    }
                  >
                    View
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>

        {tasks.length === 0 ? (
          <div className="empty-state">
            No tasks match the selected
            filters.
          </div>
        ) : null}
      </div>
    </section>
  );
}

function ResultsTable({
  results,
}: {
  results: ClinicalResult[];
}) {
  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <h2>Clinical results</h2>

          <p>
            Results returned by the Go API.
          </p>
        </div>
      </div>

      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Result</th>
              <th>Patient</th>
              <th>Test</th>
              <th>Value</th>
              <th>Severity</th>
              <th>Matched rule</th>
              <th>Source</th>
              <th>Received</th>
            </tr>
          </thead>

          <tbody>
            {results.map(
              (result) => (
                <tr key={result.id}>
                  <td className="mono">
                    {result.id.slice(
                      0,
                      8,
                    )}
                  </td>

                  <td>
                    {
                      result.patientReference
                    }
                  </td>

                  <td>
                    {formatLabel(
                      result.test,
                    )}
                  </td>

                  <td>
                    <strong>
                      {result.value}{" "}
                      {result.unit}
                    </strong>
                  </td>

                  <td>
                    <span
                      className={`badge severity ${result.severity}`}
                    >
                      {
                        result.severity
                      }
                    </span>
                  </td>

                  <td>
                    {
                      result.matchedRule
                    }
                  </td>

                  <td>
                    {
                      result.sourceSystem
                    }
                  </td>

                  <td>
                    {new Date(
                      result.receivedAt,
                    ).toLocaleTimeString(
                      [],
                      {
                        hour: "2-digit",
                        minute:
                          "2-digit",
                        second:
                          "2-digit",
                      },
                    )}
                  </td>
                </tr>
              ),
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ActivityFeed({
  events,
}: {
  events: AuditEvent[];
}) {
  return (
    <section className="panel activity-panel">
      <div className="panel-heading">
        <div>
          <h2>Recent activity</h2>

          <p>
            Local UI activity for the
            current session.
          </p>
        </div>
      </div>

      <div className="activity-list">
        {events.map((event) => {
          const Icon =
            activityIcons[event.type];

          return (
            <article
              className={`activity-item event-${event.type}`}
              key={event.id}
            >
              <span className="activity-icon">
                <Icon size={17} />
              </span>

              <div>
                <strong>
                  {formatLabel(
                    event.type,
                  )}
                </strong>

                <p>
                  {event.description}
                </p>
              </div>

              <time>
                {new Date(
                  event.timestamp,
                ).toLocaleTimeString(
                  [],
                  {
                    hour: "2-digit",
                    minute: "2-digit",
                    second: "2-digit",
                  },
                )}
              </time>
            </article>
          );
        })}

        {events.length === 0 ? (
          <div className="empty-state">
            Submit or acknowledge a task
            to generate activity.
          </div>
        ) : null}
      </div>
    </section>
  );
}

interface TaskDrawerProps {
  task: ClinicalTask | null;
  busy: boolean;
  error: string | null;
  onClose: () => void;
  onAcknowledge: (
    task: ClinicalTask,
  ) => void;
}

function TaskDrawer({
  task,
  busy,
  error,
  onClose,
  onAcknowledge,
}: TaskDrawerProps) {
  if (!task) {
    return null;
  }

  return (
    <div
      className="drawer-backdrop"
      onMouseDown={onClose}
      role="presentation"
    >
      <aside
        className="drawer"
        onMouseDown={(event) =>
          event.stopPropagation()
        }
      >
        <button
          className="drawer-close"
          onClick={onClose}
          type="button"
          aria-label="Close task details"
        >
          <X size={19} />
        </button>

        <div className="drawer-kicker">
          {task.patientReference}
        </div>

        <h2>
          {formatLabel(task.test)}
        </h2>

        <div className="drawer-result">
          {task.value}{" "}
          <span>{task.unit}</span>
        </div>

        <div className="drawer-badges">
          <span
            className={`badge severity ${task.severity}`}
          >
            {task.severity}
          </span>

          <span
            className={`badge status ${task.status}`}
          >
            {formatLabel(
              task.status,
            )}
          </span>
        </div>

        <dl className="details-grid">
          <div>
            <dt>Assigned team</dt>
            <dd>
              {task.assignedTeam}
            </dd>
          </div>

          <div>
            <dt>Escalation level</dt>
            <dd>
              {task.escalationLevel}
            </dd>
          </div>

          <div>
            <dt>Task version</dt>
            <dd>{task.version}</dd>
          </div>

          <div>
            <dt>Reported</dt>
            <dd>
              {new Date(
                task.reportedAt,
              ).toLocaleString()}
            </dd>
          </div>
        </dl>

        <div className="deadline-box">
          <Clock3 size={18} />

          <div>
            <span>
              Acknowledgement deadline
            </span>

            <strong>
              {task.dueAt
                ? new Date(
                    task.dueAt,
                  ).toLocaleTimeString()
                : "Not set"}
            </strong>
          </div>
        </div>

        <div className="drawer-note">
          Synthetic demonstration only.
          This interface is not a
          clinical device and must not be
          used with real patient data.
        </div>

        {error ? (
          <div className="error-banner">
            {error}
          </div>
        ) : null}

        <button
          className="primary-button"
          disabled={
            busy ||
            task.status !==
              "awaiting_ack"
          }
          onClick={() =>
            onAcknowledge(task)
          }
          type="button"
        >
          <CheckCircle2 size={18} />

          {busy
            ? "Acknowledging…"
            : task.status ===
                "acknowledged"
              ? "Task acknowledged"
              : "Acknowledge task"}
        </button>
      </aside>
    </div>
  );
}

interface SubmitResultPageProps {
  form: SubmitResultForm;
  submitting: boolean;
  successMessage: string | null;
  errorMessage: string | null;

  onFormChange: (
    field: keyof SubmitResultForm,
    value: string,
  ) => void;

  onSubmit: (
    event: FormEvent<HTMLFormElement>,
  ) => void;
}

function SubmitResultPage({
  form,
  submitting,
  successMessage,
  errorMessage,
  onFormChange,
  onSubmit,
}: SubmitResultPageProps) {
  return (
    <>
      <section className="page-heading">
        <div>
          <h1>
            Submit clinical result
          </h1>

          <p>
            Send a synthetic result to
            the live Go API.
          </p>
        </div>
      </section>

      <section className="form-layout">
        <form
          className="panel result-form"
          onSubmit={onSubmit}
        >
          <div className="panel-heading">
            <div>
              <h2>Result details</h2>

              <p>
                Use synthetic patient
                references only.
              </p>
            </div>
          </div>

          <div className="form-body">
            <label>
              <span>
                Patient reference
              </span>

              <input
                required
                value={
                  form.patientReference
                }
                onChange={(event) =>
                  onFormChange(
                    "patientReference",
                    event.target.value,
                  )
                }
              />
            </label>

            <label>
              <span>Test</span>

              <select
                value={form.test}
                onChange={(event) =>
                  onFormChange(
                    "test",
                    event.target.value,
                  )
                }
              >
                <option value="serum_potassium">
                  Serum potassium
                </option>

                <option value="troponin_t">
                  Troponin T
                </option>

                <option value="haemoglobin">
                  Haemoglobin
                </option>

                <option value="c_reactive_protein">
                  C-reactive protein
                </option>

                <option value="white_blood_cell_count">
                  White blood cell count
                </option>
              </select>
            </label>

            <div className="form-row">
              <label>
                <span>Value</span>

                <input
                  required
                  type="number"
                  step="0.1"
                  value={form.value}
                  onChange={(event) =>
                    onFormChange(
                      "value",
                      event.target.value,
                    )
                  }
                />
              </label>

              <label>
                <span>Unit</span>

                <input
                  required
                  value={form.unit}
                  onChange={(event) =>
                    onFormChange(
                      "unit",
                      event.target.value,
                    )
                  }
                />
              </label>
            </div>

            <label>
              <span>Source system</span>

              <input
                required
                value={
                  form.sourceSystem
                }
                onChange={(event) =>
                  onFormChange(
                    "sourceSystem",
                    event.target.value,
                  )
                }
              />
            </label>

            <label>
              <span>
                Source result ID
              </span>

              <input
                required
                value={
                  form.sourceResultId
                }
                onChange={(event) =>
                  onFormChange(
                    "sourceResultId",
                    event.target.value,
                  )
                }
              />
            </label>

            {successMessage ? (
              <div className="success-banner">
                <CheckCircle2
                  size={18}
                />
                {successMessage}
              </div>
            ) : null}

            {errorMessage ? (
              <div className="error-banner">
                {errorMessage}
              </div>
            ) : null}

            <button
              className="primary-button form-submit"
              disabled={submitting}
              type="submit"
            >
              <Stethoscope
                size={18}
              />

              {submitting
                ? "Submitting…"
                : "Submit result"}
            </button>
          </div>
        </form>

        <aside className="panel classification-preview">
          <div className="panel-heading">
            <div>
              <h2>Demo scenario</h2>

              <p>
                This pre-filled result
                demonstrates the critical
                potassium workflow.
              </p>
            </div>
          </div>

          <div className="preview-body">
            <span className="preview-severity critical">
              Critical
            </span>

            <div className="preview-value">
              {form.value || "0"}{" "}
              <small>{form.unit}</small>
            </div>

            <dl className="preview-details">
              <div>
                <dt>
                  Expected matched rule
                </dt>

                <dd>
                  potassium ≥ 6.5 mmol/L
                </dd>
              </div>

              <div>
                <dt>
                  Expected assigned team
                </dt>

                <dd>acute-medicine</dd>
              </div>

              <div>
                <dt>
                  Expected workflow
                </dt>

                <dd>
                  result → task → worker →
                  acknowledgement
                </dd>
              </div>
            </dl>
          </div>
        </aside>
      </section>
    </>
  );
}

function PlaceholderPage({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <>
      <section className="page-heading">
        <div>
          <h1>{title}</h1>
          <p>{description}</p>
        </div>
      </section>

      <section className="panel placeholder-panel">
        {children}
      </section>
    </>
  );
}

function App() {
  const [activePage, setActivePage] =
    useState<Page>("dashboard");

  const [tasks, setTasks] = useState<
    ClinicalTask[]
  >([]);

  const [results, setResults] =
    useState<ClinicalResult[]>([]);

  const [events, setEvents] =
    useState<AuditEvent[]>([]);

  const [
    selectedTask,
    setSelectedTask,
  ] = useState<ClinicalTask | null>(
    null,
  );

  const [
    severityFilter,
    setSeverityFilter,
  ] = useState<"all" | Severity>(
    "all",
  );

  const [
    statusFilter,
    setStatusFilter,
  ] = useState<
    "all" | TaskStatus
  >("all");

  const [query, setQuery] =
    useState("");

  const [busy, setBusy] =
    useState(false);

  const [
    drawerError,
    setDrawerError,
  ] = useState<string | null>(null);

  const [
    loadingData,
    setLoadingData,
  ] = useState(false);

  const [
    dataError,
    setDataError,
  ] = useState<string | null>(null);

  const [
    currentTime,
    setCurrentTime,
  ] = useState(Date.now());

  const [
    submitForm,
    setSubmitForm,
  ] = useState<SubmitResultForm>(
    initialSubmitForm,
  );

  const [
    submittingResult,
    setSubmittingResult,
  ] = useState(false);

  const [
    submitSuccess,
    setSubmitSuccess,
  ] = useState<string | null>(null);

  const [
    submitError,
    setSubmitError,
  ] = useState<string | null>(null);

  const loadBackendData =
    useCallback(async () => {
      setLoadingData(true);
      setDataError(null);

      try {
        const [
          apiTasks,
          apiResults,
        ] = await Promise.all([
          getTasks(),
          getResults(),
        ]);

        const mappedResults =
          apiResults.map(mapApiResult);

        const resultById = new Map(
          mappedResults.map((result) => [
            result.id,
            result,
          ]),
        );

        const mappedTasks =
          apiTasks.map((task) =>
            mapApiTask(
              task,
              resultById,
            ),
          );

        setResults(mappedResults);
        setTasks(mappedTasks);

        setSelectedTask(
          (currentTask) => {
            if (!currentTask) {
              return null;
            }

            return (
              mappedTasks.find(
                (task) =>
                  task.id ===
                  currentTask.id,
              ) ?? null
            );
          },
        );
      } catch (error) {
        const message =
          error instanceof Error
            ? error.message
            : "Failed to load backend data.";

        setDataError(message);

        console.error(
          "Failed to load backend data:",
          error,
        );
      } finally {
        setLoadingData(false);
      }
    }, []);

  useEffect(() => {
    const timer =
      window.setInterval(() => {
        setCurrentTime(Date.now());
      }, 1000);

    return () =>
      window.clearInterval(timer);
  }, []);

  useEffect(() => {
    void loadBackendData();

    const pollingInterval =
      window.setInterval(() => {
        void loadBackendData();
      }, 5000);

    return () =>
      window.clearInterval(
        pollingInterval,
      );
  }, [loadBackendData]);

  const filteredTasks = useMemo(
    () => {
      const normalisedQuery =
        query.trim().toLowerCase();

      return tasks.filter((task) => {
        const severityMatches =
          severityFilter === "all" ||
          task.severity ===
            severityFilter;

        const statusMatches =
          statusFilter === "all" ||
          task.status === statusFilter;

        const searchMatches =
          normalisedQuery.length === 0 ||
          [
            task.id,
            task.patientReference,
            task.test,
            task.assignedTeam,
            task.status,
          ].some((value) =>
            value
              .toLowerCase()
              .includes(
                normalisedQuery,
              ),
          );

        return (
          severityMatches &&
          statusMatches &&
          searchMatches
        );
      });
    },
    [
      tasks,
      severityFilter,
      statusFilter,
      query,
    ],
  );

  const awaitingAcknowledgementCount =
    tasks.filter(
      (task) =>
        task.status ===
        "awaiting_ack",
    ).length;

  const overdueCount = tasks.filter(
    (task) =>
      task.dueAt !== undefined &&
      new Date(
        task.dueAt,
      ).getTime() < currentTime &&
      task.status ===
        "awaiting_ack",
  ).length;

  const escalationCount =
    tasks.filter(
      (task) =>
        task.escalationLevel > 0 ||
        task.status === "escalated",
    ).length;

  const acknowledgedCount =
    tasks.filter(
      (task) =>
        task.status ===
        "acknowledged",
    ).length;

  const criticalCount =
    tasks.filter(
      (task) =>
        task.severity === "critical",
    ).length;

  async function handleAcknowledge(
    task: ClinicalTask,
  ) {
    setBusy(true);
    setDrawerError(null);

    try {
      await acknowledgeTask(
        task.id,
        task.version,
      );

      setEvents(
        (currentEvents) => [
          {
            id: crypto.randomUUID(),
            taskId: task.id,
            resultId: task.resultId,
            type: "task_acknowledged",
            description:
              `Task ${task.id.slice(
                0,
                8,
              )} acknowledged by clinician-42.`,
            timestamp:
              new Date().toISOString(),
          },
          ...currentEvents,
        ],
      );

      await loadBackendData();
    } catch (error) {
      setDrawerError(
        error instanceof Error
          ? error.message
          : "The task could not be acknowledged.",
      );
    } finally {
      setBusy(false);
    }
  }

  function handleFormChange(
    field: keyof SubmitResultForm,
    value: string,
  ) {
    setSubmitSuccess(null);
    setSubmitError(null);

    const unitByTest: Record<
      string,
      string
    > = {
      serum_potassium: "mmol/L",
      troponin_t: "ng/L",
      haemoglobin: "g/L",
      c_reactive_protein: "mg/L",
      white_blood_cell_count:
        "10⁹/L",
    };

    setSubmitForm(
      (currentForm) => ({
        ...currentForm,
        [field]: value,
        ...(field === "test"
          ? {
              unit:
                unitByTest[value] ??
                currentForm.unit,
            }
          : {}),
      }),
    );
  }

  async function handleSubmitResult(
    event: FormEvent<HTMLFormElement>,
  ) {
    event.preventDefault();

    const numericValue =
      Number(submitForm.value);

    if (
      !Number.isFinite(numericValue)
    ) {
      setSubmitError(
        "The result value must be a valid number.",
      );

      return;
    }

    setSubmittingResult(true);
    setSubmitSuccess(null);
    setSubmitError(null);

    try {
      await createResult({
        source_system:
          submitForm.sourceSystem,

        source_result_id:
          submitForm.sourceResultId,

        patient_reference:
          submitForm.patientReference,

        test_code:
          submitForm.test,

        value: numericValue,
        unit: submitForm.unit,

        reported_at:
          new Date().toISOString(),
      });

      setEvents(
        (currentEvents) => [
          {
            id: crypto.randomUUID(),
            type: "result_received",
            description:
              `Result ${submitForm.sourceResultId} submitted to the Go API.`,
            timestamp:
              new Date().toISOString(),
          },
          ...currentEvents,
        ],
      );

      setSubmitSuccess(
        "Result submitted successfully. The worker will now process the generated task.",
      );

      setSubmitForm(
        (currentForm) => ({
          ...currentForm,
          sourceResultId:
            `LIMS-DEMO-${Date.now()}`,
        }),
      );

      await loadBackendData();
    } catch (error) {
      setSubmitError(
        error instanceof Error
          ? error.message
          : "The result could not be submitted.",
      );
    } finally {
      setSubmittingResult(false);
    }
  }

  function clearTaskFilters() {
    setSeverityFilter("all");
    setStatusFilter("all");
    setQuery("");
  }

  function renderTaskSummary() {
    return (
      <section className="task-page-summary">
        <div>
          <span>Total tasks</span>
          <strong>
            {tasks.length}
          </strong>
        </div>

        <div>
          <span>Critical</span>
          <strong>
            {criticalCount}
          </strong>
        </div>

        <div>
          <span>
            Awaiting acknowledgement
          </span>

          <strong>
            {
              awaitingAcknowledgementCount
            }
          </strong>
        </div>

        <div>
          <span>Escalated</span>
          <strong>
            {escalationCount}
          </strong>
        </div>
      </section>
    );
  }

  function renderDashboard() {
    return (
      <>
        <section className="page-heading">
          <div>
            <h1>Dashboard</h1>

            <p>
              Live view of clinical
              results and task
              escalation.
            </p>
          </div>

          <button
            className="secondary-button"
            type="button"
            disabled={loadingData}
            onClick={() =>
              void loadBackendData()
            }
          >
            <RefreshCw size={16} />

            {loadingData
              ? "Refreshing…"
              : "Refresh"}
          </button>
        </section>

        {dataError ? (
          <div className="error-banner">
            API error: {dataError}
          </div>
        ) : null}

        <section className="metrics-grid">
          <MetricCard
            title="Results"
            value={String(
              results.length,
            )}
            tone="blue"
            caption="Loaded from PostgreSQL"
          />

          <MetricCard
            title="Awaiting acknowledgement"
            value={String(
              awaitingAcknowledgementCount,
            )}
            tone="purple"
            caption={`${tasks.length} total tasks`}
          />

          <MetricCard
            title="Overdue tasks"
            value={String(
              overdueCount,
            )}
            tone="red"
            caption="Needs immediate review"
          />

          <MetricCard
            title="Escalations"
            value={String(
              escalationCount,
            )}
            tone="amber"
            caption="Severity-aware routing"
          />

          <MetricCard
            title="Acknowledged"
            value={String(
              acknowledgedCount,
            )}
            tone="green"
            caption="Persisted clinician actions"
          />
        </section>

        <section className="dashboard-grid">
          <TaskTable
            tasks={filteredTasks.slice(
              0,
              5,
            )}
            currentTime={
              currentTime
            }
            severityFilter={
              severityFilter
            }
            statusFilter={
              statusFilter
            }
            query={query}
            onSeverityChange={
              setSeverityFilter
            }
            onStatusChange={
              setStatusFilter
            }
            onQueryChange={setQuery}
            onSelect={(task) => {
              setSelectedTask(task);
              setDrawerError(null);
            }}
          />

          <ActivityFeed
            events={events.slice(
              0,
              6,
            )}
          />
        </section>

        <section className="bottom-grid">
          <article className="panel compact-panel">
            <div className="panel-heading">
              <div>
                <h2>
                  Task status
                  distribution
                </h2>

                <p>
                  Current PostgreSQL
                  workflow state.
                </p>
              </div>
            </div>

            <div className="donut-row">
              <div className="donut">
                <span>
                  {tasks.length}
                </span>

                <small>Total</small>
              </div>

              <ul className="distribution-list">
                <li>
                  <span className="dot purple" />
                  Awaiting acknowledgement

                  <strong>
                    {
                      awaitingAcknowledgementCount
                    }
                  </strong>
                </li>

                <li>
                  <span className="dot blue" />
                  Processing

                  <strong>
                    {
                      tasks.filter(
                        (task) =>
                          task.status ===
                          "processing",
                      ).length
                    }
                  </strong>
                </li>

                <li>
                  <span className="dot amber" />
                  Pending

                  <strong>
                    {
                      tasks.filter(
                        (task) =>
                          task.status ===
                          "pending",
                      ).length
                    }
                  </strong>
                </li>

                <li>
                  <span className="dot red" />
                  Escalated

                  <strong>
                    {
                      escalationCount
                    }
                  </strong>
                </li>
              </ul>
            </div>
          </article>

          <article className="panel compact-panel">
            <div className="panel-heading">
              <div>
                <h2>
                  Connected services
                </h2>

                <p>
                  Live demonstration
                  architecture.
                </p>
              </div>
            </div>

            <div className="status-list">
              {[
                "React UI",
                "Go API",
                "Worker",
                "Scheduler",
                "PostgreSQL",
              ].map((service) => (
                <div key={service}>
                  <span>
                    {service}
                  </span>

                  <strong>
                    Connected <i />
                  </strong>
                </div>
              ))}
            </div>
          </article>
        </section>
      </>
    );
  }

  function renderCurrentPage() {
    switch (activePage) {
      case "dashboard":
        return renderDashboard();

      case "tasks":
        return (
          <>
            <section className="page-heading">
              <div>
                <h1>
                  Clinical tasks
                </h1>

                <p>
                  Review and acknowledge
                  database-backed tasks.
                </p>
              </div>

              <button
                className="secondary-button"
                type="button"
                onClick={
                  clearTaskFilters
                }
              >
                Clear filters
              </button>
            </section>

            {dataError ? (
              <div className="error-banner">
                API error: {dataError}
              </div>
            ) : null}

            {renderTaskSummary()}

            <TaskTable
              tasks={filteredTasks}
              currentTime={
                currentTime
              }
              severityFilter={
                severityFilter
              }
              statusFilter={
                statusFilter
              }
              query={query}
              onSeverityChange={
                setSeverityFilter
              }
              onStatusChange={
                setStatusFilter
              }
              onQueryChange={setQuery}
              onSelect={(task) => {
                setSelectedTask(task);
                setDrawerError(null);
              }}
            />
          </>
        );

      case "results":
        return (
          <>
            <section className="page-heading">
              <div>
                <h1>
                  Clinical results
                </h1>

                <p>
                  Results stored by the
                  Go backend.
                </p>
              </div>

              <button
                className="secondary-button"
                type="button"
                onClick={() =>
                  setActivePage(
                    "submit-result",
                  )
                }
              >
                <Stethoscope
                  size={16}
                />
                Submit result
              </button>
            </section>

            {dataError ? (
              <div className="error-banner">
                API error: {dataError}
              </div>
            ) : null}

            <ResultsTable
              results={results}
            />
          </>
        );

      case "escalations":
        return (
          <>
            <section className="page-heading">
              <div>
                <h1>Escalations</h1>

                <p>
                  Tasks that have moved
                  beyond their original
                  responsible team.
                </p>
              </div>
            </section>

            {renderTaskSummary()}

            <TaskTable
              tasks={tasks.filter(
                (task) =>
                  task.status ===
                    "escalated" ||
                  task.escalationLevel >
                    0,
              )}
              currentTime={
                currentTime
              }
              severityFilter="all"
              statusFilter="all"
              query=""
              onSeverityChange={() =>
                undefined
              }
              onStatusChange={() =>
                undefined
              }
              onQueryChange={() =>
                undefined
              }
              onSelect={(task) => {
                setSelectedTask(task);
                setDrawerError(null);
              }}
            />
          </>
        );

      case "submit-result":
        return (
          <SubmitResultPage
            form={submitForm}
            submitting={
              submittingResult
            }
            successMessage={
              submitSuccess
            }
            errorMessage={
              submitError
            }
            onFormChange={
              handleFormChange
            }
            onSubmit={
              handleSubmitResult
            }
          />
        );

      case "notifications":
        return (
          <PlaceholderPage
            title="Notifications"
            description="Notification activity generated during this browser session."
          >
            <div className="notification-list">
              {events
                .filter(
                  (event) =>
                    event.type ===
                      "notification_delivered" ||
                    event.type ===
                      "result_received",
                )
                .map((event) => (
                  <article
                    key={event.id}
                  >
                    <span className="notification-icon">
                      <BellRing
                        size={18}
                      />
                    </span>

                    <div>
                      <strong>
                        {formatLabel(
                          event.type,
                        )}
                      </strong>

                      <p>
                        {
                          event.description
                        }
                      </p>
                    </div>

                    <time>
                      {new Date(
                        event.timestamp,
                      ).toLocaleTimeString()}
                    </time>
                  </article>
                ))}

              {events.length === 0 ? (
                <div className="empty-state">
                  No local activity yet.
                </div>
              ) : null}
            </div>
          </PlaceholderPage>
        );

      case "metrics":
        return (
          <PlaceholderPage
            title="Metrics"
            description="Live summary derived from the task and result APIs."
          >
            <section className="metrics-grid embedded-metrics">
              <MetricCard
                title="Results ingested"
                value={String(
                  results.length,
                )}
                tone="blue"
                caption="GET /v1/results"
              />

              <MetricCard
                title="Tasks"
                value={String(
                  tasks.length,
                )}
                tone="purple"
                caption="GET /v1/tasks"
              />

              <MetricCard
                title="Escalations"
                value={String(
                  escalationCount,
                )}
                tone="amber"
                caption="Current task state"
              />

              <MetricCard
                title="Acknowledgements"
                value={String(
                  acknowledgedCount,
                )}
                tone="green"
                caption="Persisted workflow state"
              />
            </section>
          </PlaceholderPage>
        );

      case "settings":
        return (
          <PlaceholderPage
            title="Settings"
            description="Frontend configuration for the live demonstration."
          >
            <div className="settings-list">
              <label>
                <span>
                  Signed-in clinician
                </span>

                <input
                  value="clinician-42"
                  readOnly
                />
              </label>

              <label>
                <span>Data mode</span>

                <input
                  value="Live Go API"
                  readOnly
                />
              </label>

              <label>
                <span>
                  Refresh interval
                </span>

                <input
                  value="5 seconds"
                  readOnly
                />
              </label>

              <label>
                <span>
                  API environment variable
                </span>

                <input
                  value={
                    import.meta.env
                      .VITE_API_URL ??
                    "http://localhost:8080"
                  }
                  readOnly
                />
              </label>
            </div>
          </PlaceholderPage>
        );
    }
  }

  return (
    <div className="app-shell">
      <Sidebar
        activePage={activePage}
        taskCount={
          awaitingAcknowledgementCount
        }
        escalationCount={
          escalationCount
        }
        onPageChange={
          setActivePage
        }
      />

      <main>
        <header className="topbar">
          <div className="mobile-brand">
            Clinical Escalation
          </div>

          <div className="topbar-actions">
            <span className="health-indicator">
              Live
            </span>

            <button
              className="icon-button"
              type="button"
              aria-label="Notifications"
              onClick={() =>
                setActivePage(
                  "notifications",
                )
              }
            >
              <Bell size={18} />
            </button>

            <button
              className="icon-button"
              type="button"
              aria-label="Help"
            >
              <CircleHelp size={18} />
            </button>

            <div className="user-chip">
              clinician-42
            </div>
          </div>
        </header>

        <div className="content">
          {renderCurrentPage()}
        </div>
      </main>

      <TaskDrawer
        task={selectedTask}
        busy={busy}
        error={drawerError}
        onClose={() => {
          setSelectedTask(null);
          setDrawerError(null);
        }}
        onAcknowledge={
          handleAcknowledge
        }
      />
    </div>
  );
}

export default App;