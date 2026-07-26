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
import { useEffect, useMemo, useState } from "react";

type Severity = "critical" | "urgent" | "routine";

type TaskStatus =
  | "pending"
  | "processing"
  | "awaiting_ack"
  | "acknowledged"
  | "escalated"
  | "failed";

type AuditEventType =
  | "task_created"
  | "notification_delivered"
  | "task_acknowledged"
  | "acknowledgement_deadline_missed"
  | "task_escalated";

interface ClinicalTask {
  id: string;
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
  taskId: string;
  type: AuditEventType;
  description: string;
  timestamp: string;
}

const initialTimestamp = Date.now();

const initialTasks: ClinicalTask[] = [
  {
    id: "a1f4c2d8",
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
    type: "notification_delivered",
    description: "Push notification delivered to cardiology.",
    timestamp: new Date(initialTimestamp - 107_000).toISOString(),
  },
  {
    id: "event-4",
    taskId: "a1f4c2d8",
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

const navigationGroups = [
  {
    label: "Overview",
    items: [
      { label: "Dashboard", icon: Gauge, active: true },
      { label: "Tasks", icon: ClipboardList, badge: 3 },
      { label: "Results", icon: HeartPulse },
      { label: "Escalations", icon: ShieldAlert },
    ],
  },
  {
    label: "Operations",
    items: [
      { label: "Submit result", icon: Stethoscope },
      { label: "Notifications", icon: BellRing },
    ],
  },
  {
    label: "Observability",
    items: [
      { label: "Metrics", icon: Activity },
      { label: "Settings", icon: Settings },
    ],
  },
];

const activityIcons = {
  task_escalated: CircleAlert,
  task_acknowledged: CheckCircle2,
  notification_delivered: BellRing,
  task_created: FilePlus2,
  acknowledgement_deadline_missed: Clock3,
};

function formatLabel(value: string): string {
  return value.replaceAll("_", " ");
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

function Sidebar() {
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

              return (
                <button
                  className={`nav-item ${item.active ? "active" : ""}`}
                  key={item.label}
                  type="button"
                >
                  <Icon size={17} />

                  <span>{item.label}</span>

                  {item.badge ? (
                    <span className="nav-badge">{item.badge}</span>
                  ) : null}
                </button>
              );
            })}
          </div>
        ))}
      </nav>

      <div className="sidebar-footer">
        <span>Clinical Escalation Engine</span>
        <small>v0.1.0 · synthetic demonstration</small>
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
          <h2>Active tasks</h2>
          <p>Tasks currently moving through the clinical workflow.</p>
        </div>

        <div className="filters">
          <select
            value={severityFilter}
            onChange={(event) =>
              onSeverityChange(
                event.target.value as "all" | Severity,
              )
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
              onStatusChange(
                event.target.value as "all" | TaskStatus,
              )
            }
          >
            <option value="all">All statuses</option>
            <option value="awaiting_ack">
              Awaiting acknowledgement
            </option>
            <option value="processing">Processing</option>
            <option value="pending">Pending</option>
            <option value="escalated">Escalated</option>
            <option value="acknowledged">Acknowledged</option>
          </select>

          <label className="search-input">
            <Search size={16} />

            <input
              value={query}
              onChange={(event) =>
                onQueryChange(event.target.value)
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
                <td className="mono">{task.id}</td>

                <td>{task.patientReference}</td>

                <td>
                  <strong>{formatLabel(task.test)}</strong>

                  <span className="result-value">
                    {task.value} {task.unit}
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
                {new Date(event.timestamp).toLocaleTimeString(
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

        <div className="drawer-kicker">
          {task.patientReference}
        </div>

        <h2>{formatLabel(task.test)}</h2>

        <div className="drawer-result">
          {task.value} <span>{task.unit}</span>
        </div>

        <div className="drawer-badges">
          <span
            className={`badge severity ${task.severity}`}
          >
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
            <dd>
              {new Date(task.reportedAt).toLocaleString()}
            </dd>
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
          Synthetic demonstration only. This interface is not a
          clinical device and must not be used with real patient
          data.
        </div>

        {error ? (
          <div className="error-banner">{error}</div>
        ) : null}

        <button
          className="primary-button"
          disabled={
            busy || task.status !== "awaiting_ack"
          }
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

function App() {
  const [tasks, setTasks] =
    useState<ClinicalTask[]>(initialTasks);

  const [events, setEvents] =
    useState<AuditEvent[]>(initialEvents);

  const [selectedTask, setSelectedTask] =
    useState<ClinicalTask | null>(null);

  const [severityFilter, setSeverityFilter] =
    useState<"all" | Severity>("all");

  const [statusFilter, setStatusFilter] =
    useState<"all" | TaskStatus>("all");

  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] =
    useState<string | null>(null);

  const [currentTime, setCurrentTime] =
    useState(Date.now());

  useEffect(() => {
    const timer = window.setInterval(() => {
      setCurrentTime(Date.now());
    }, 1000);

    return () => window.clearInterval(timer);
  }, []);

  const filteredTasks = useMemo(() => {
    const normalisedQuery =
      query.trim().toLowerCase();

    return tasks.filter((task) => {
      const severityMatches =
        severityFilter === "all" ||
        task.severity === severityFilter;

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
            .includes(normalisedQuery),
        );

      return (
        severityMatches &&
        statusMatches &&
        searchMatches
      );
    });
  }, [
    tasks,
    severityFilter,
    statusFilter,
    query,
  ]);

  const awaitingAcknowledgementCount =
    tasks.filter(
      (task) => task.status === "awaiting_ack",
    ).length;

  const overdueCount = tasks.filter(
    (task) =>
      task.dueAt !== undefined &&
      new Date(task.dueAt).getTime() < currentTime &&
      task.status === "awaiting_ack",
  ).length;

  const escalationCount = tasks.filter(
    (task) => task.escalationLevel > 0,
  ).length;

  const acknowledgedCount = tasks.filter(
    (task) => task.status === "acknowledged",
  ).length;

  async function handleAcknowledge(
    task: ClinicalTask,
  ) {
    setBusy(true);
    setError(null);

    try {
      await new Promise((resolve) =>
        window.setTimeout(resolve, 650),
      );

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
          id: `event-${Date.now()}`,
          taskId: task.id,
          type: "task_acknowledged",
          description:
            "Acknowledgement received from clinician-42.",
          timestamp: new Date().toISOString(),
        },
        ...currentEvents,
      ]);
    } catch {
      setError("The task could not be acknowledged.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="app-shell">
      <Sidebar />

      <main>
        <header className="topbar">
          <div className="mobile-brand">
            Clinical Escalation
          </div>

          <div className="topbar-actions">
            <span className="health-indicator">
              Healthy
            </span>

            <button
              className="icon-button"
              type="button"
              aria-label="Notifications"
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
          <section className="page-heading">
            <div>
              <h1>Dashboard</h1>

              <p>
                Real-time view of synthetic clinical
                results and task escalation.
              </p>
            </div>

            <button
              className="secondary-button"
              type="button"
              onClick={() =>
                setCurrentTime(Date.now())
              }
            >
              <RefreshCw size={16} />
              Refresh
            </button>
          </section>

          <section className="metrics-grid">
            <MetricCard
              title="Results (15m)"
              value="1,247"
              tone="blue"
              caption="↑ 12% vs previous period"
            />

            <MetricCard
              title="Awaiting acknowledgement"
              value={String(
                awaitingAcknowledgementCount,
              )}
              tone="purple"
              caption={`${tasks.length} active tasks`}
            />

            <MetricCard
              title="Overdue tasks"
              value={String(overdueCount)}
              tone="red"
              caption="Needs immediate review"
            />

            <MetricCard
              title="Escalations (15m)"
              value={String(escalationCount)}
              tone="amber"
              caption="Severity-aware routing"
            />

            <MetricCard
              title="Acknowledged"
              value={String(acknowledgedCount)}
              tone="green"
              caption="Updated through task actions"
            />
          </section>

          <section className="dashboard-grid">
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
                setError(null);
              }}
            />

            <ActivityFeed events={events} />
          </section>

          <section className="bottom-grid">
            <article className="panel compact-panel">
              <div className="panel-heading">
                <div>
                  <h2>Task status distribution</h2>
                  <p>
                    Current active workflow state.
                  </p>
                </div>
              </div>

              <div className="donut-row">
                <div
                  className="donut"
                  aria-label="Task distribution chart"
                >
                  <span>{tasks.length}</span>
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
                      {escalationCount}
                    </strong>
                  </li>
                </ul>
              </div>
            </article>

            <article className="panel compact-panel">
              <div className="panel-heading">
                <div>
                  <h2>System status</h2>

                  <p>
                    Local demonstration services.
                  </p>
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
        </div>
      </main>

      <TaskDrawer
        task={selectedTask}
        busy={busy}
        error={error}
        onClose={() => {
          setSelectedTask(null);
          setError(null);
        }}
        onAcknowledge={handleAcknowledge}
      />
    </div>
  );
}

export default App;