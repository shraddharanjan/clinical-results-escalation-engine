const API_URL =
  import.meta.env.VITE_API_URL ??
  "http://localhost:8080";

export type ApiSeverity =
  | "critical"
  | "urgent"
  | "routine";

export type ApiTaskStatus =
  | "pending"
  | "processing"
  | "awaiting_ack"
  | "acknowledged"
  | "completed"
  | "escalated"
  | "failed";

export interface ApiTask {
  id: string;
  result_id: string;
  task_type: string;
  status: ApiTaskStatus;
  severity: ApiSeverity;
  assigned_team: string;
  assigned_user?: string | null;
  escalation_level: number;
  available_at?: string;
  acknowledgement_due_at?: string | null;
  lease_owner?: string | null;
  lease_expires_at?: string | null;
  attempt_count: number;
  version: number;
  created_at: string;
  updated_at: string;

  /*
   * These fields should be returned by GET /v1/tasks
   * through a join with the linked clinical result.
   */
  patient_reference?: string;
  test_code?: string;
  value?: number;
  unit?: string;
  reported_at?: string;
}

export interface ApiResult {
  id: string;
  source_system: string;
  source_result_id: string;
  patient_reference: string;
  test_code: string;
  value: number;
  unit: string;
  reported_at: string;
  received_at: string;
  severity: ApiSeverity;
  matched_rule?: string | null;
}

export interface ApiAuditEvent {
  id: string;
  task_id?: string | null;
  result_id?: string | null;
  event_type: string;
  event_data?: Record<string, unknown>;
  created_at: string;
}

export interface CreateResultRequest {
  source_system: string;
  source_result_id: string;
  patient_reference: string;
  test_code: string;
  value: number;
  unit: string;
  reported_at: string;
}

export interface AcknowledgeTaskRequest {
  clinician_id: string;
  expected_version: number;
}

async function request<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  const response = await fetch(
    `${API_URL}${path}`,
    {
      ...options,
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
        ...options?.headers,
      },
    },
  );

  if (!response.ok) {
    const responseBody =
      await response.text();

    throw new Error(
      responseBody ||
        `Request failed with status ${response.status}`,
    );
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const contentType =
    response.headers.get("content-type");

  if (
    !contentType?.includes(
      "application/json",
    )
  ) {
    return undefined as T;
  }

  return response.json() as Promise<T>;
}

export function getHealth(): Promise<unknown> {
  return request<unknown>("/health");
}

export function getTasks(): Promise<ApiTask[]> {
  return request<ApiTask[]>("/v1/tasks");
}

export function getTask(
  taskId: string,
): Promise<ApiTask> {
  return request<ApiTask>(
    `/v1/tasks/${taskId}`,
  );
}

export function getResults(): Promise<
  ApiResult[]
> {
  return request<ApiResult[]>("/v1/results");
}

export function getTaskEvents(
  taskId: string,
): Promise<ApiAuditEvent[]> {
  return request<ApiAuditEvent[]>(
    `/v1/tasks/${taskId}/events`,
  );
}

export function createResult(
  input: CreateResultRequest,
): Promise<ApiResult> {
  return request<ApiResult>("/v1/results", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function acknowledgeTask(
  taskId: string,
  expectedVersion: number,
  clinicianId = "clinician-42",
): Promise<void> {
  const input: AcknowledgeTaskRequest = {
    clinician_id: clinicianId,
    expected_version: expectedVersion,
  };

  return request<void>(
    `/v1/tasks/${taskId}/acknowledgements`,
    {
      method: "POST",
      body: JSON.stringify(input),
    },
  );
}