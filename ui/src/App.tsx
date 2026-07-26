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
import { FormEvent, useEffect, useMemo, useState } from "react";

type Severity = "critical" | "urgent" | "routine";

type TaskStatus =
  | "pending"
  | "processing"
  | "awaiting_ack"
  | "acknowledged"
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

const initialTimestamp = Date.now();

const initialResults: ClinicalResult[] = [
  {
    id: "result-a1f4c2d8",
    patientReference: "P-1042",
    test: "serum_potassium",
    value: 6.8,
    unit: "mmol/L",
    severity: "critical",
    sourceSystem: "laboratory-simulator",
    sourceResultId: "LIMS-839211",
    reportedAt: new Date(initialTimestamp - 8 * 60_000).toISOString(),
    receivedAt: new Date(initialTimestamp - 7.8 * 60_000).toISOString(),
    matchedRule: "potassium >= 6.5 mmol/L",
  },
  {
    id: "result-b7e9d102",
    patientReference: "P-1021",
    test: "troponin_t",
    value: 82,
    unit: "ng/L",
    severity: "critical",
    sourceSystem: "laboratory-simulator",
    sourceResultId: "LIMS-839212",
    reportedAt: new Date(initialTimestamp - 7 * 60_000).toISOString(),
    receivedAt: new Date(initialTimestamp - 6.8 * 60_000).toISOString(),
    matchedRule: "troponin_t >= 52 ng/L",
  },
  {
    id: "result-c3f6b9a1",
    patientReference: "P-1055",
    test: "haemoglobin",
    value: 74,
    unit: "g/L",
    severity: "urgent",
    sourceSystem: "laboratory-simulator",
    sourceResultId: "LIMS-839213",
    reportedAt: new Date(initialTimestamp - 5 * 60_000).toISOString(),
    receivedAt: new Date(initialTimestamp - 4.8 * 60_000).toISOString(),
    matchedRule: "haemoglobin < 80 g/L",
  },
  {
    id: "result-d91e7a44",
    patientReference: "P-1008",
    test: "white_blood_cell_count",
    value: 15.2,
    unit: "10⁹/L",
    severity: "routine",
    sourceSystem: "laboratory-simulator",
    sourceResultId: "LIMS-839214",
    reportedAt: new Date(initialTimestamp - 4 * 60_000).toISOString(),
    receivedAt: new Date(initialTimestamp - 3.8 * 60_000).toISOString(),
    matchedRule: "default routine classification",
  },
  {
    id: "result-e5b2d7c9",
    patientReference: "P-1077",
    test: "c_reactive_protein",
    value: 198,
    unit: "mg/L",
    severity: "urgent",
    sourceSystem: "laboratory-simulator",
    sourceResultId: "LIMS-839215",
    reportedAt: new Date(initialTimestamp - 18 * 60_000).toISOString(),
    receivedAt: new Date(initialTimestamp - 17.8 * 60_000).toISOString(),
    matchedRule: "c_reactive_protein >= 150 mg/L",
  },
];

const initialTasks: ClinicalTask[] = [
  {
    id: "a1f4c2d8",
    resultId: "result-a1f4c2d8",
    patientReference: "P-1042",
    test: "serum_potassium",
    value: 6.8,
    unit: "mmol/L",
    severity: "critical",
    status: "awaiting_ack",
    assignedTeam: "acute-medicine",
    dueAt: new Date(initialTimestamp + 151_000).toISOString(),
    escalationLevel: 0,
    version: 3,
    reportedAt: new Date(initialTimestamp - 8 * 60_000).toISOString(),
  },
  {
    id: "b7e9d102",
    resultId: "result-b7e9d102",
    patientReference: "P-1021",
    test: "troponin_t",
    value: 82,
    unit: "ng/L",
    severity: "critical",
    status: "awaiting_ack",
    assignedTeam: "cardiology",
    dueAt: new Date(initialTimestamp + 372_000).toISOString(),
    escalationLevel: 0,
    version: 3,
    reportedAt: new Date(initialTimestamp - 7 * 60_000).toISOString(),
  },
  {
    id: "c3f6b9a1",
    resultId: "result-c3f6b9a1",
    patientReference: "P-1055",
    test: "haemoglobin",
    value: 74,
    unit: "g/L",
    severity: "urgent",
    status: "processing",
    assignedTeam: "lab-review-team",
    escalationLevel: 0,
    version: 2,
    reportedAt: new Date(initialTimestamp - 5 * 60_000).toISOString(),
  },
  {
    id: "d91e7a44",
    resultId: "result-d91e7a44",
    patientReference: "P-1008",
    test: "white_blood_cell_count",
    value: 15.2,
    unit: "10⁹/L",
    severity: "routine",
    status: "pending",
    assignedTeam: "pathology",
    escalationLevel: 0,
    version: 1,
    reportedAt: new Date(initialTimestamp - 4 * 60_000).toISOString(),
  },
  {
    id: "e5b2d7c9",
    resultId: "result-e5b2d7c9",
    patientReference: "P-1077",
    test: "c_reactive_protein",
    value: 198,
    unit: "mg/L",
    severity: "urgent",
    status: "escalated",
    assignedTeam: "medical-registrar",
    dueAt: new Date(initialTimestamp + 525_000).toISOString(),
    escalationLevel: 1,
    version: 5,
    reportedAt: new Date(initialTimestamp - 18 * 60_000).toISOString(),
  },
];

const initialEvents: AuditEvent[] = [
  {
    id: "event-1",
    taskId: "e5b2d7c9",
    resultId: "result-e5b2d7c9",
    type: "task_escalated",
    description:
      "Task escalated to level 1 and assigned to the medical registrar.",
    timestamp: new Date(initialTimestamp - 80_000).toISOString(),
  },
  {
    id: "event-2",
    taskId: "8d71c3e4",
    type: "task_acknowledged",
    description: "Acknowledgement received from clinician-12.",
    timestamp: new Date(initialTimestamp - 101_000).toISOString(),
  },
  {
    id: "event-3",
    taskId: "b7e9d102",
    resultId: "result-b7e9d102",
    type: "notification_delivered",
    description: "Push notification delivered to cardiology.",
    timestamp: new Date(initialTimestamp - 107_000).toISOString(),
  },
  {
    id: "event-4",
    taskId: "a1f4c2d8",
    resultId: "result-a1f4c2d8",
    type: "task_created",
    description:
      "Critical serum potassium result created a clinical review task.",
    timestamp: new Date(initialTimestamp - 130_000).toISOString(),
  },
  {
    id: "event-5",
    taskId: "6c2a1f98",
    type: "acknowledgement_deadline_missed",
    description: "The acknowledgement deadline was missed.",
    timestamp: new Date(initialTimestamp - 197_000).toISOString(),
  },
];

const initialSubmitForm: SubmitResultForm = {
  patientReference: "P-2048",
  test: "serum_potassium",
  value: "6.8",
  unit: "mmol/L",
  sourceSystem: "laboratory-simulator",
  sourceResultId: "LIMS-DEMO-001",
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
  acknowledgement_deadline_missed: Clock3,
  task_escalated: CircleAlert,
};

function formatLabel(value: string): string {
  return value.replaceAll("_", " ");
}

function createShortId(): string {
  return crypto.randomUUID().replaceAll("-", "").slice(0, 8);
}

function classifyResult(test: string, value: number): {
  severity: Severity;
  rule: string;
  assignedTeam: string;
  deadlineMinutes: number;
} {
  if (test === "serum_potassium" && value >= 6.5) {
    return {
      severity: "critical",
      rule: "potassium >= 6.5 mmol/L",
      assignedTeam: "acute-medicine",
      deadlineMinutes: 5,
    };
  }

  if (test === "serum_potassium" && value >= 6.0) {
    return {
      severity: "urgent",
      rule: "potassium >= 6.0 mmol/L",
      assignedTeam: "acute-medicine",
      deadlineMinutes: 15,
    };
  }

  if (test === "troponin_t" && value >= 52) {
    return {
      severity: "critical",
      rule: "troponin_t >= 52 ng/L",
      assignedTeam: "cardiology",
      deadlineMinutes: 5,
    };
  }

  if (test === "haemoglobin" && value < 80) {
    return {
      severity: "urgent",
      rule: "haemoglobin < 80 g/L",
      assignedTeam: "lab-review-team",
      deadlineMinutes: 15,
    };
  }

  if (test === "c_reactive_protein" && value >= 150) {
    return {
      severity: "urgent",
      rule: "c_reactive_protein >= 150 mg/L",
      assignedTeam: "acute-medicine",
      deadlineMinutes: 15,
    };
  }

  return {
    severity: "routine",
    rule: "default routine classification",
    assignedTeam: "pathology",
    deadlineMinutes: 60,
  };
}

function dueLabel(dueAt: string | undefined, currentTime: number): string {
  if (!dueAt) {
    return "—";
  }

  const milliseconds = new Date(dueAt).getTime() - currentTime;

  if (milliseconds <= 0) {
    return "Overdue";
  }

  const minutes = Math.floor(milliseconds / 60_000);
  const seconds = Math.floor((milliseconds % 60_000) / 1000);

  return `${minutes}m ${String(seconds).padStart(2, "0")}s`;
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
        {navigationGroups.map((group) => (
          <div className="nav-group" key={group.label}>
            <div className="nav-label">{group.label}</div>

            {group.items.map((item) => {
              const Icon = item.icon;
              const isActive = activePage === item.id;

              let badge: number | null = null;

              if (item.id === "tasks") {
                badge = taskCount;
              }

              if (item.id === "escalations") {
                badge = escalationCount;
              }

              return (
                <button
                  className={`nav-item ${isActive ? "active" : ""}`}
                  key={item.id}
                  type="button"
                  onClick={() => onPageChange(item.id)}
                >
                  <Icon size={17} />
                  <span>{item.label}</span>

                  {badge !== null && badge > 0 ? (
                    <span className="nav-badge">{badge}</span>
                  ) : null}
                </button>
              );
            })}
          </div>
        ))}
      </nav>

      <div className="sidebar-footer">
        <span>Clinical Escalation Engine</span>
        <small>v0.2.0 · synthetic demonstration</small>
      </div>
    </aside>
  );
}

interface MetricCardProps {
  title: string;
  value: string;
  tone: "blue" | "purple" | "red" | "amber" | "green";
  caption: string;
}

function MetricCard({
  title,
  value,
  tone,
  caption,
}: MetricCardProps) {
  return (
    <article className={`metric-card tone-${tone}`}>
      <div className="metric-title">{title}</div>
      <div className="metric-value">{value}</div>
      <div className="metric-caption">{caption}</div>

      <svg className="sparkline" viewBox="0 0 200 50" aria-hidden="true">
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
  severityFilter: "all" | Severity;
  statusFilter: "all" | TaskStatus;
  query: string;
  onSeverityChange: (value: "all" | Severity) => void;
  onStatusChange: (value: "all" | TaskStatus) => void;
  onQueryChange: (value: string) => void;
  onSelect: (task: ClinicalTask) => void;
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
          <p>Tasks currently moving through the clinical workflow.</p>
        </div>

        <div className="filters">
          <select
            value={severityFilter}
            onChange={(event) =>
              onSeverityChange(event.target.value as "all" | Severity)
            }
          >
            <option value="all">All severities</option>
            <option value="critical">Critical</option>
            <option value="urgent">Urgent</option>
            <option value="routine">Routine</option>
          </select>

          <select
            value={statusFilter}
            onChange={(event) =>
              onStatusChange(event.target.value as "all" | TaskStatus)
            }
          >
            <option value="all">All statuses</option>
            <option value="awaiting_ack">Awaiting acknowledgement</option>
            <option value="processing">Processing</option>
            <option value="pending">Pending</option>
            <option value="escalated">Escalated</option>
            <option value="acknowledged">Acknowledged</option>
          </select>

          <label className="search-input">
            <Search size={16} />

            <input
              value={query}
              onChange={(event) => onQueryChange(event.target.value)}
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
                <td className="mono">{task.id}</td>
                <td>{task.patientReference}</td>

                <td>
                  <strong>{formatLabel(task.test)}</strong>

                  <span className="result-value">
                    {task.value} {task.unit}
                  </span>
                </td>

                <td>
                  <span className={`badge severity ${task.severity}`}>
                    {task.severity}
                  </span>
                </td>

                <td>
                  <span className={`badge status ${task.status}`}>
                    {formatLabel(task.status)}

                    {task.escalationLevel > 0
                      ? ` · L${task.escalationLevel}`
                      : ""}
                  </span>
                </td>

                <td>{task.assignedTeam}</td>

                <td className={task.dueAt ? "due" : ""}>
                  {dueLabel(task.dueAt, currentTime)}
                </td>

                <td>
                  <button
                    className="ghost-button"
                    type="button"
                    onClick={() => onSelect(task)}
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
            No tasks match the selected filters.
          </div>
        ) : null}
      </div>
    </section>
  );
}

function ResultsTable({ results }: { results: ClinicalResult[] }) {
  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <h2>Clinical results</h2>
          <p>Results received and classified by the workflow engine.</p>
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
            {results.map((result) => (
              <tr key={result.id}>
                <td className="mono">{result.id.replace("result-", "")}</td>
                <td>{result.patientReference}</td>
                <td>{formatLabel(result.test)}</td>
                <td>
                  <strong>
                    {result.value} {result.unit}
                  </strong>
                </td>
                <td>
                  <span className={`badge severity ${result.severity}`}>
                    {result.severity}
                  </span>
                </td>
                <td>{result.matchedRule}</td>
                <td>{result.sourceSystem}</td>
                <td>
                  {new Date(result.receivedAt).toLocaleTimeString([], {
                    hour: "2-digit",
                    minute: "2-digit",
                    second: "2-digit",
                  })}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ActivityFeed({ events }: { events: AuditEvent[] }) {
  return (
    <section className="panel activity-panel">
      <div className="panel-heading">
        <div>
          <h2>Recent activity</h2>
          <p>Latest audit events from the workflow engine.</p>
        </div>
      </div>

      <div className="activity-list">
        {events.map((event) => {
          const Icon = activityIcons[event.type];

          return (
            <article
              className={`activity-item event-${event.type}`}
              key={event.id}
            >
              <span className="activity-icon">
                <Icon size={17} />
              </span>

              <div>
                <strong>{formatLabel(event.type)}</strong>
                <p>{event.description}</p>
              </div>

              <time>
                {new Date(event.timestamp).toLocaleTimeString([], {
                  hour: "2-digit",
                  minute: "2-digit",
                  second: "2-digit",
                })}
              </time>
            </article>
          );
        })}
      </div>
    </section>
  );
}

interface TaskDrawerProps {
  task: ClinicalTask | null;
  busy: boolean;
  error: string | null;
  onClose: () => void;
  onAcknowledge: (task: ClinicalTask) => void;
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
        onMouseDown={(event) => event.stopPropagation()}
      >
        <button
          className="drawer-close"
          onClick={onClose}
          type="button"
          aria-label="Close task details"
        >
          <X size={19} />
        </button>

        <div className="drawer-kicker">{task.patientReference}</div>
        <h2>{formatLabel(task.test)}</h2>

        <div className="drawer-result">
          {task.value} <span>{task.unit}</span>
        </div>

        <div className="drawer-badges">
          <span className={`badge severity ${task.severity}`}>
            {task.severity}
          </span>

          <span className={`badge status ${task.status}`}>
            {formatLabel(task.status)}
          </span>
        </div>

        <dl className="details-grid">
          <div>
            <dt>Assigned team</dt>
            <dd>{task.assignedTeam}</dd>
          </div>

          <div>
            <dt>Escalation level</dt>
            <dd>{task.escalationLevel}</dd>
          </div>

          <div>
            <dt>Task version</dt>
            <dd>{task.version}</dd>
          </div>

          <div>
            <dt>Reported</dt>
            <dd>{new Date(task.reportedAt).toLocaleString()}</dd>
          </div>
        </dl>

        <div className="deadline-box">
          <Clock3 size={18} />

          <div>
            <span>Acknowledgement deadline</span>

            <strong>
              {task.dueAt
                ? new Date(task.dueAt).toLocaleTimeString()
                : "Not set"}
            </strong>
          </div>
        </div>

        <div className="drawer-note">
          Synthetic demonstration only. This interface is not a clinical
          device and must not be used with real patient data.
        </div>

        {error ? <div className="error-banner">{error}</div> : null}

        <button
          className="primary-button"
          disabled={busy || task.status !== "awaiting_ack"}
          onClick={() => onAcknowledge(task)}
          type="button"
        >
          <CheckCircle2 size={18} />

          {busy
            ? "Acknowledging…"
            : task.status === "acknowledged"
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
  onFormChange: (field: keyof SubmitResultForm, value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}

function SubmitResultPage({
  form,
  submitting,
  successMessage,
  onFormChange,
  onSubmit,
}: SubmitResultPageProps) {
  const preview = classifyResult(form.test, Number(form.value || 0));

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>Submit clinical result</h1>

          <p>
            Create a synthetic result and demonstrate classification,
            task creation and acknowledgement.
          </p>
        </div>
      </section>

      <section className="form-layout">
        <form className="panel result-form" onSubmit={onSubmit}>
          <div className="panel-heading">
            <div>
              <h2>Result details</h2>
              <p>All data entered here is synthetic demonstration data.</p>
            </div>
          </div>

          <div className="form-body">
            <label>
              <span>Patient reference</span>

              <input
                required
                value={form.patientReference}
                onChange={(event) =>
                  onFormChange("patientReference", event.target.value)
                }
              />
            </label>

            <label>
              <span>Test</span>

              <select
                value={form.test}
                onChange={(event) =>
                  onFormChange("test", event.target.value)
                }
              >
                <option value="serum_potassium">Serum potassium</option>
                <option value="troponin_t">Troponin T</option>
                <option value="haemoglobin">Haemoglobin</option>
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
                    onFormChange("value", event.target.value)
                  }
                />
              </label>

              <label>
                <span>Unit</span>

                <input
                  required
                  value={form.unit}
                  onChange={(event) =>
                    onFormChange("unit", event.target.value)
                  }
                />
              </label>
            </div>

            <label>
              <span>Source system</span>

              <input
                required
                value={form.sourceSystem}
                onChange={(event) =>
                  onFormChange("sourceSystem", event.target.value)
                }
              />
            </label>

            <label>
              <span>Source result ID</span>

              <input
                required
                value={form.sourceResultId}
                onChange={(event) =>
                  onFormChange("sourceResultId", event.target.value)
                }
              />
            </label>

            {successMessage ? (
              <div className="success-banner">
                <CheckCircle2 size={18} />
                {successMessage}
              </div>
            ) : null}

            <button
              className="primary-button form-submit"
              disabled={submitting}
              type="submit"
            >
              <Stethoscope size={18} />
              {submitting ? "Creating workflow…" : "Submit result"}
            </button>
          </div>
        </form>

        <aside className="panel classification-preview">
          <div className="panel-heading">
            <div>
              <h2>Classification preview</h2>
              <p>How the demonstration rule engine will classify this result.</p>
            </div>
          </div>

          <div className="preview-body">
            <span className={`preview-severity ${preview.severity}`}>
              {preview.severity}
            </span>

            <div className="preview-value">
              {form.value || "0"} <small>{form.unit}</small>
            </div>

            <dl className="preview-details">
              <div>
                <dt>Matched rule</dt>
                <dd>{preview.rule}</dd>
              </div>

              <div>
                <dt>Assigned team</dt>
                <dd>{preview.assignedTeam}</dd>
              </div>

              <div>
                <dt>Acknowledgement deadline</dt>
                <dd>{preview.deadlineMinutes} minutes</dd>
              </div>

              <div>
                <dt>Initial task status</dt>
                <dd>awaiting acknowledgement</dd>
              </div>
            </dl>

            <div className="workflow-preview">
              <span>Result received</span>
              <i />
              <span>Classified</span>
              <i />
              <span>Task created</span>
              <i />
              <span>Notification delivered</span>
            </div>
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

      <section className="panel placeholder-panel">{children}</section>
    </>
  );
}

function App() {
  const [activePage, setActivePage] = useState<Page>("dashboard");
  const [tasks, setTasks] = useState<ClinicalTask[]>(initialTasks);
  const [results, setResults] =
    useState<ClinicalResult[]>(initialResults);
  const [events, setEvents] = useState<AuditEvent[]>(initialEvents);

  const [selectedTask, setSelectedTask] =
    useState<ClinicalTask | null>(null);

  const [severityFilter, setSeverityFilter] =
    useState<"all" | Severity>("all");

  const [statusFilter, setStatusFilter] =
    useState<"all" | TaskStatus>("all");

  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [drawerError, setDrawerError] = useState<string | null>(null);

  const [currentTime, setCurrentTime] = useState(Date.now());

  const [submitForm, setSubmitForm] =
    useState<SubmitResultForm>(initialSubmitForm);

  const [submittingResult, setSubmittingResult] = useState(false);
  const [submitSuccess, setSubmitSuccess] = useState<string | null>(
    null,
  );

  useEffect(() => {
    const timer = window.setInterval(() => {
      setCurrentTime(Date.now());
    }, 1000);

    return () => window.clearInterval(timer);
  }, []);

  const filteredTasks = useMemo(() => {
    const normalisedQuery = query.trim().toLowerCase();

    return tasks.filter((task) => {
      const severityMatches =
        severityFilter === "all" || task.severity === severityFilter;

      const statusMatches =
        statusFilter === "all" || task.status === statusFilter;

      const searchMatches =
        normalisedQuery.length === 0 ||
        [
          task.id,
          task.patientReference,
          task.test,
          task.assignedTeam,
          task.status,
        ].some((value) =>
          value.toLowerCase().includes(normalisedQuery),
        );

      return severityMatches && statusMatches && searchMatches;
    });
  }, [tasks, severityFilter, statusFilter, query]);

  const awaitingAcknowledgementCount = tasks.filter(
    (task) => task.status === "awaiting_ack",
  ).length;

  const overdueCount = tasks.filter(
    (task) =>
      task.dueAt !== undefined &&
      new Date(task.dueAt).getTime() < currentTime &&
      task.status === "awaiting_ack",
  ).length;

  const escalationCount = tasks.filter(
    (task) =>
      task.escalationLevel > 0 || task.status === "escalated",
  ).length;

  const acknowledgedCount = tasks.filter(
    (task) => task.status === "acknowledged",
  ).length;

  const criticalCount = tasks.filter(
    (task) => task.severity === "critical",
  ).length;

  async function handleAcknowledge(task: ClinicalTask) {
    setBusy(true);
    setDrawerError(null);

    try {
      await new Promise((resolve) => window.setTimeout(resolve, 650));

      const acknowledgedTask: ClinicalTask = {
        ...task,
        status: "acknowledged",
        version: task.version + 1,
      };

      setTasks((currentTasks) =>
        currentTasks.map((currentTask) =>
          currentTask.id === task.id
            ? acknowledgedTask
            : currentTask,
        ),
      );

      setSelectedTask(acknowledgedTask);

      setEvents((currentEvents) => [
        {
          id: `event-${crypto.randomUUID()}`,
          taskId: task.id,
          resultId: task.resultId,
          type: "task_acknowledged",
          description:
            `Task ${task.id} acknowledged by clinician-42.`,
          timestamp: new Date().toISOString(),
        },
        ...currentEvents,
      ]);
    } catch {
      setDrawerError("The task could not be acknowledged.");
    } finally {
      setBusy(false);
    }
  }

  function handleFormChange(
    field: keyof SubmitResultForm,
    value: string,
  ) {
    setSubmitSuccess(null);

    setSubmitForm((currentForm) => ({
      ...currentForm,
      [field]: value,
    }));

    if (field === "test") {
      const unitByTest: Record<string, string> = {
        serum_potassium: "mmol/L",
        troponin_t: "ng/L",
        haemoglobin: "g/L",
        c_reactive_protein: "mg/L",
        white_blood_cell_count: "10⁹/L",
      };

      setSubmitForm((currentForm) => ({
        ...currentForm,
        test: value,
        unit: unitByTest[value] ?? currentForm.unit,
      }));
    }
  }

  async function handleSubmitResult(
    event: FormEvent<HTMLFormElement>,
  ) {
    event.preventDefault();

    const numericValue = Number(submitForm.value);

    if (!Number.isFinite(numericValue)) {
      return;
    }

    setSubmittingResult(true);
    setSubmitSuccess(null);

    await new Promise((resolve) => window.setTimeout(resolve, 850));

    const resultShortId = createShortId();
    const taskId = createShortId();
    const resultId = `result-${resultShortId}`;
    const timestamp = new Date();

    const classification = classifyResult(
      submitForm.test,
      numericValue,
    );

    const result: ClinicalResult = {
      id: resultId,
      patientReference: submitForm.patientReference,
      test: submitForm.test,
      value: numericValue,
      unit: submitForm.unit,
      severity: classification.severity,
      sourceSystem: submitForm.sourceSystem,
      sourceResultId: submitForm.sourceResultId,
      reportedAt: timestamp.toISOString(),
      receivedAt: timestamp.toISOString(),
      matchedRule: classification.rule,
    };

    const task: ClinicalTask = {
      id: taskId,
      resultId,
      patientReference: submitForm.patientReference,
      test: submitForm.test,
      value: numericValue,
      unit: submitForm.unit,
      severity: classification.severity,
      status: "awaiting_ack",
      assignedTeam: classification.assignedTeam,
      dueAt: new Date(
        timestamp.getTime() +
          classification.deadlineMinutes * 60_000,
      ).toISOString(),
      escalationLevel: 0,
      version: 3,
      reportedAt: timestamp.toISOString(),
    };

    setResults((currentResults) => [result, ...currentResults]);
    setTasks((currentTasks) => [task, ...currentTasks]);

    setEvents((currentEvents) => [
      {
        id: `event-${crypto.randomUUID()}`,
        taskId,
        resultId,
        type: "notification_delivered",
        description:
          `Push notification delivered to ${classification.assignedTeam}.`,
        timestamp: new Date(timestamp.getTime() + 300).toISOString(),
      },
      {
        id: `event-${crypto.randomUUID()}`,
        taskId,
        resultId,
        type: "task_created",
        description:
          `${classification.severity} ${formatLabel(
            submitForm.test,
          )} result created task ${taskId}.`,
        timestamp: new Date(timestamp.getTime() + 200).toISOString(),
      },
      {
        id: `event-${crypto.randomUUID()}`,
        resultId,
        type: "result_classified",
        description:
          `Result classified as ${classification.severity} using rule: ${classification.rule}.`,
        timestamp: new Date(timestamp.getTime() + 100).toISOString(),
      },
      {
        id: `event-${crypto.randomUUID()}`,
        resultId,
        type: "result_received",
        description:
          `Result ${submitForm.sourceResultId} received from ${submitForm.sourceSystem}.`,
        timestamp: timestamp.toISOString(),
      },
      ...currentEvents,
    ]);

    setSubmitSuccess(
      `Workflow created. Result classified as ${classification.severity} and task ${taskId} assigned to ${classification.assignedTeam}.`,
    );

    setSubmittingResult(false);
    setSelectedTask(task);
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
          <strong>{tasks.length}</strong>
        </div>

        <div>
          <span>Critical</span>
          <strong>{criticalCount}</strong>
        </div>

        <div>
          <span>Awaiting acknowledgement</span>
          <strong>{awaitingAcknowledgementCount}</strong>
        </div>

        <div>
          <span>Escalated</span>
          <strong>{escalationCount}</strong>
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
              Real-time view of synthetic clinical results and task
              escalation.
            </p>
          </div>

          <button
            className="secondary-button"
            type="button"
            onClick={() => setCurrentTime(Date.now())}
          >
            <RefreshCw size={16} />
            Refresh
          </button>
        </section>

        <section className="metrics-grid">
          <MetricCard
            title="Results"
            value={String(results.length)}
            tone="blue"
            caption="Synthetic results received"
          />

          <MetricCard
            title="Awaiting acknowledgement"
            value={String(awaitingAcknowledgementCount)}
            tone="purple"
            caption={`${tasks.length} total tasks`}
          />

          <MetricCard
            title="Overdue tasks"
            value={String(overdueCount)}
            tone="red"
            caption="Needs immediate review"
          />

          <MetricCard
            title="Escalations"
            value={String(escalationCount)}
            tone="amber"
            caption="Severity-aware routing"
          />

          <MetricCard
            title="Acknowledged"
            value={String(acknowledgedCount)}
            tone="green"
            caption="Completed clinician actions"
          />
        </section>

        <section className="dashboard-grid">
          <TaskTable
            tasks={filteredTasks.slice(0, 5)}
            currentTime={currentTime}
            severityFilter={severityFilter}
            statusFilter={statusFilter}
            query={query}
            onSeverityChange={setSeverityFilter}
            onStatusChange={setStatusFilter}
            onQueryChange={setQuery}
            onSelect={(task) => {
              setSelectedTask(task);
              setDrawerError(null);
            }}
          />

          <ActivityFeed events={events.slice(0, 6)} />
        </section>

        <section className="bottom-grid">
          <article className="panel compact-panel">
            <div className="panel-heading">
              <div>
                <h2>Task status distribution</h2>
                <p>Current workflow state.</p>
              </div>
            </div>

            <div className="donut-row">
              <div className="donut">
                <span>{tasks.length}</span>
                <small>Total</small>
              </div>

              <ul className="distribution-list">
                <li>
                  <span className="dot purple" />
                  Awaiting acknowledgement
                  <strong>{awaitingAcknowledgementCount}</strong>
                </li>

                <li>
                  <span className="dot blue" />
                  Processing
                  <strong>
                    {
                      tasks.filter(
                        (task) => task.status === "processing",
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
                        (task) => task.status === "pending",
                      ).length
                    }
                  </strong>
                </li>

                <li>
                  <span className="dot red" />
                  Escalated
                  <strong>{escalationCount}</strong>
                </li>
              </ul>
            </div>
          </article>

          <article className="panel compact-panel">
            <div className="panel-heading">
              <div>
                <h2>System status</h2>
                <p>Local demonstration services.</p>
              </div>
            </div>

            <div className="status-list">
              {[
                "API",
                "Workers",
                "Scheduler",
                "PostgreSQL",
                "Prometheus",
                "Jaeger",
              ].map((service) => (
                <div key={service}>
                  <span>{service}</span>

                  <strong>
                    Healthy <i />
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
                <h1>Clinical tasks</h1>

                <p>
                  Review, filter and acknowledge clinical workflow tasks.
                </p>
              </div>

              <button
                className="secondary-button"
                type="button"
                onClick={clearTaskFilters}
              >
                Clear filters
              </button>
            </section>

            {renderTaskSummary()}

            <TaskTable
              tasks={filteredTasks}
              currentTime={currentTime}
              severityFilter={severityFilter}
              statusFilter={statusFilter}
              query={query}
              onSeverityChange={setSeverityFilter}
              onStatusChange={setStatusFilter}
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
                <h1>Clinical results</h1>

                <p>
                  Results received and classified by the demonstration
                  workflow engine.
                </p>
              </div>

              <button
                className="secondary-button"
                type="button"
                onClick={() => setActivePage("submit-result")}
              >
                <Stethoscope size={16} />
                Submit result
              </button>
            </section>

            <ResultsTable results={results} />
          </>
        );

      case "escalations":
        return (
          <>
            <section className="page-heading">
              <div>
                <h1>Escalations</h1>

                <p>
                  Tasks that have moved beyond their original responsible
                  team.
                </p>
              </div>
            </section>

            {renderTaskSummary()}

            <TaskTable
              tasks={tasks.filter(
                (task) =>
                  task.status === "escalated" ||
                  task.escalationLevel > 0,
              )}
              currentTime={currentTime}
              severityFilter="all"
              statusFilter="all"
              query=""
              onSeverityChange={() => undefined}
              onStatusChange={() => undefined}
              onQueryChange={() => undefined}
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
            submitting={submittingResult}
            successMessage={submitSuccess}
            onFormChange={handleFormChange}
            onSubmit={handleSubmitResult}
          />
        );

      case "notifications":
        return (
          <PlaceholderPage
            title="Notifications"
            description="Delivery attempts generated by the synthetic workflow."
          >
            <div className="notification-list">
              {events
                .filter(
                  (event) => event.type === "notification_delivered",
                )
                .map((event) => (
                  <article key={event.id}>
                    <span className="notification-icon">
                      <BellRing size={18} />
                    </span>

                    <div>
                      <strong>Push notification delivered</strong>
                      <p>{event.description}</p>
                    </div>

                    <time>
                      {new Date(event.timestamp).toLocaleTimeString()}
                    </time>
                  </article>
                ))}
            </div>
          </PlaceholderPage>
        );

      case "metrics":
        return (
          <PlaceholderPage
            title="Metrics"
            description="A simplified UI representation of workflow metrics."
          >
            <section className="metrics-grid embedded-metrics">
              <MetricCard
                title="Results ingested"
                value={String(results.length)}
                tone="blue"
                caption="clinical_results_ingested_total"
              />

              <MetricCard
                title="Tasks claimed"
                value={String(tasks.length)}
                tone="purple"
                caption="clinical_tasks_claimed_total"
              />

              <MetricCard
                title="Escalations"
                value={String(escalationCount)}
                tone="amber"
                caption="clinical_escalations_total"
              />

              <MetricCard
                title="Acknowledgements"
                value={String(acknowledgedCount)}
                tone="green"
                caption="Synthetic UI state"
              />
            </section>
          </PlaceholderPage>
        );

      case "settings":
        return (
          <PlaceholderPage
            title="Settings"
            description="Demonstration configuration for the clinician UI."
          >
            <div className="settings-list">
              <label>
                <span>Signed-in clinician</span>
                <input value="clinician-42" readOnly />
              </label>

              <label>
                <span>Default team</span>
                <input value="acute-medicine" readOnly />
              </label>

              <label>
                <span>Data mode</span>
                <input value="Synthetic demonstration data" readOnly />
              </label>

              <label>
                <span>API mode</span>
                <input value="Frontend mock workflow" readOnly />
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
        taskCount={awaitingAcknowledgementCount}
        escalationCount={escalationCount}
        onPageChange={setActivePage}
      />

      <main>
        <header className="topbar">
          <div className="mobile-brand">Clinical Escalation</div>

          <div className="topbar-actions">
            <span className="health-indicator">Healthy</span>

            <button
              className="icon-button"
              type="button"
              aria-label="Notifications"
              onClick={() => setActivePage("notifications")}
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

            <div className="user-chip">clinician-42</div>
          </div>
        </header>

        <div className="content">{renderCurrentPage()}</div>
      </main>

      <TaskDrawer
        task={selectedTask}
        busy={busy}
        error={drawerError}
        onClose={() => {
          setSelectedTask(null);
          setDrawerError(null);
        }}
        onAcknowledge={handleAcknowledge}
      />
    </div>
  );
}

export default App;