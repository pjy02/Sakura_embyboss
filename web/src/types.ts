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
  member_count?: number;
}

export interface PermissionCatalogGroup {
  group: string;
  items: Array<{ permission: string; label: string }>;
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

export interface CoreDashboard {
  live_sessions: number;
  plays_today: number;
  known_devices: number;
  risk_devices: number;
  lines_total: number;
  lines_healthy: number;
  line_statuses: Array<{
    id: number;
    name: string;
    status: "unknown" | "healthy" | "offline";
    latency_ms: number | null;
    maintenance: boolean;
  }>;
  checked_at: string;
  emby_status?: "emby" | "unavailable";
  emby_error?: string | null;
}

export interface PlaybackSession {
  id: number;
  session_id: string;
  emby_user_id: string | null;
  emby_user_name: string | null;
  tg: number | null;
  item_id: string | null;
  item_name: string | null;
  series_name: string | null;
  item_type: string | null;
  client_name: string | null;
  app_version: string | null;
  device_key: string | null;
  device_name: string | null;
  remote_address: string | null;
  position_ticks: number;
  runtime_ticks: number;
  progress_percent: number;
  is_paused: boolean;
  is_transcoding: boolean;
  started_at: string;
  last_seen_at: string;
  ended_at: string | null;
}

export interface LivePlaybackResponse {
  items: PlaybackSession[];
  total: number;
  source: "emby" | "unavailable";
  error: string | null;
  synced_at: string;
}

export interface KnownDevice {
  device_key: string;
  emby_user_id: string | null;
  emby_user_name: string | null;
  tg: number | null;
  device_name: string | null;
  client_name: string | null;
  app_version: string | null;
  last_ip: string | null;
  trusted: boolean;
  banned: boolean;
  risk_level: "normal" | "warning" | "high";
  notes: string | null;
  playback_count: number;
  first_seen_at: string;
  last_seen_at: string;
}

export interface LineEndpoint {
  id: number;
  name: string;
  base_url: string;
  region: string | null;
  carrier: string | null;
  audience: "all" | "whitelist";
  weight: number;
  sort_order: number;
  enabled: boolean;
  maintenance: boolean;
  revision: number;
  last_status: "unknown" | "healthy" | "offline";
  last_latency_ms: number | null;
  last_error: string | null;
  last_checked_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface RechargeProduct {
  id: number;
  name: string;
  description: string | null;
  amount_cents: number;
  coins: number;
  bonus_coins: number;
  enabled: boolean;
  sort_order: number;
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface RechargeOrder {
  id: string;
  order_no: string;
  tg: number;
  product_id: number | null;
  product_name: string;
  amount_cents: number;
  coins: number;
  bonus_coins: number;
  payment_method: string;
  payment_reference: string | null;
  status: "pending" | "credited" | "canceled";
  user_note: string | null;
  admin_note: string | null;
  created_at: string;
  paid_at: string | null;
  credited_at: string | null;
  canceled_at: string | null;
  updated_at: string;
}

export interface BillingEntry {
  id: number;
  order_id: string | null;
  tg: number;
  entry_type: string;
  amount_cents: number | null;
  coins: number | null;
  description: string;
  actor_kind: string;
  actor_id: string;
  metadata: Record<string, unknown> | null;
  created_at: string;
}

export interface TicketMessage {
  id: number;
  ticket_id: string;
  sender_kind: "user" | "admin" | "system";
  sender_tg: number | null;
  body: string;
  internal: boolean;
  created_at: string;
}

export interface SupportTicket {
  id: string;
  ticket_no: string;
  tg: number;
  subject: string;
  category: string;
  priority: "low" | "normal" | "high" | "urgent";
  status: "open" | "pending_user" | "pending_staff" | "resolved" | "closed";
  assignee_tg: number | null;
  last_reply_kind: string;
  last_reply_at: string;
  resolved_at: string | null;
  closed_at: string | null;
  created_at: string;
  updated_at: string;
  messages?: TicketMessage[];
}

export interface MediaRequest {
  id: string;
  request_no: string;
  tg: number;
  title: string;
  year: number | null;
  media_type: string;
  description: string | null;
  status: "submitted" | "reviewing" | "approved" | "searching" | "downloading" | "completed" | "rejected" | "canceled";
  priority: "low" | "normal" | "high" | "urgent";
  source: "web" | "telegram";
  external_ref: string | null;
  download_id: string | null;
  cost_coins: number;
  progress: number;
  admin_note: string | null;
  reviewed_by: number | null;
  reviewed_at: string | null;
  completed_at: string | null;
  canceled_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface MediaReview {
  id: string;
  tg: number;
  media_key: string;
  media_title: string;
  media_year: number | null;
  rating: number;
  content: string;
  spoiler: boolean;
  status: "pending" | "published" | "rejected" | "hidden";
  like_count: number;
  report_count: number;
  liked: boolean;
  admin_note: string | null;
  moderated_by: number | null;
  moderated_at: string | null;
  created_at: string;
  updated_at: string;
  reports?: Array<{
    id: number;
    tg: number;
    reason: string;
    detail: string | null;
    created_at: string;
  }>;
}

export interface UserNotification {
  id: string;
  tg: number;
  category: "system" | "billing" | "ticket" | "request" | "review";
  title: string;
  body: string;
  severity: "info" | "success" | "warning" | "danger";
  action_url: string | null;
  metadata: Record<string, unknown> | null;
  read_at: string | null;
  created_at: string;
}

export interface NotificationPreference {
  category: UserNotification["category"];
  label: string;
  web_enabled: boolean;
  telegram_enabled: boolean;
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
