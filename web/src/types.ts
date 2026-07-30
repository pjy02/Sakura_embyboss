export type Area = "portal" | "admin";

export interface Session {
  tg: number;
  auth_method: "telegram" | "emby";
  roles: string[];
  permissions: string[];
}

export interface UserProfile {
  tg: number;
  embyid: string | null;
  name: string | null;
  level: "a" | "b" | "c" | "d";
  created_at: string | null;
  expires_at: string | null;
  registration_days: number;
  coins: number;
  checked_in_at: string | null;
  has_account: boolean;
  roles?: string[];
  permissions?: string[];
  auth_method?: string;
}

export interface PointTransaction {
  id: number;
  tg: number;
  balance_type: "coins" | "registration_days";
  amount: number;
  balance_after: number;
  reason: string;
  actor_kind: string;
  actor_id: string;
  metadata: Record<string, unknown> | null;
  created_at: string;
}

export interface AuditLog {
  id: number;
  request_id: string | null;
  actor_kind: string;
  actor_id: string;
  actor_name: string | null;
  action: string;
  resource_type: string;
  resource_id: string | null;
  outcome: string;
  detail: Record<string, unknown> | null;
  ip_address: string | null;
  created_at: string;
}

export interface Role {
  id: number;
  name: string;
  permissions: string[];
  is_system: boolean;
}

export interface AdminOverview {
  users_total: number;
  accounts_active: number;
  expiring_soon: number;
  levels: Record<string, number>;
  coins_total: number;
  point_changes_today: number;
  audit_events_today: number;
}

export interface TelegramLogin {
  request_id: string;
  request_token: string;
  expires_at: string;
  deep_link: string;
  poll_after_seconds: number;
}

export interface TaskDefinition {
  task_type: string;
  label: string;
  description: string;
  risk: "normal" | "warning" | "danger";
  timeout_seconds: number;
  max_retries: number;
}

export interface OperationTask {
  id: string;
  task_type: string;
  label: string;
  status: "pending" | "retrying" | "running" | "succeeded" | "failed" | "canceled";
  progress: number;
  owner_kind: string;
  owner_id: string;
  input: Record<string, unknown> | null;
  result: Record<string, unknown> | null;
  error_message: string | null;
  retry_count: number;
  max_retries: number;
  next_run_at: string;
  locked_by: string | null;
  cancel_requested: boolean;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
  updated_at: string;
}

export interface JobRun {
  id: number;
  job_name: string;
  trigger_kind: string;
  status: string;
  summary: Record<string, unknown> | null;
  error_message: string | null;
  started_at: string;
  finished_at: string | null;
}

export interface WorkerStatus {
  worker_id: string;
  worker_kind: string;
  hostname: string;
  process_id: number;
  status: string;
  current_task_id: string | null;
  metadata: Record<string, unknown> | null;
  started_at: string;
  last_seen_at: string;
  stale: boolean;
}

export interface SystemStatus {
  status: "healthy" | "degraded";
  workers: WorkerStatus[];
  task_counts: Record<string, number>;
  oldest_pending_at: string | null;
  checked_at: string;
}
