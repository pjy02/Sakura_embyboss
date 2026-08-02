export type Area = "portal" | "admin";

export interface Session {
  account_id: string;
  tg: number;
  auth_method: "local" | "telegram" | "emby";
  purpose: "login" | "registration";
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
  status: "pending" | "credited" | "canceled" | "refunded";
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

export interface RegistrationStatus {
  enabled: boolean;
  invite_required: boolean;
  requires_invite: boolean;
  open_registration_days: number;
  user_limit: number;
  registered: number;
  reserved: number;
  remaining: number;
  queue_waiting: number;
  queue_limit: number;
  has_account: boolean;
  qualification_days: number;
  can_register: boolean;
  active_task: RegistrationTask | null;
  checked_at: string;
}

export interface RegistrationTask {
  id: string;
  task_type: "registration.account";
  status: "pending" | "retrying" | "running" | "succeeded" | "failed" | "canceled";
  progress: number;
  username?: string;
  position?: number;
  result: {
    ok?: boolean;
    code?: string;
    message?: string;
    username?: string;
    emby_id?: string;
    emby_password?: string;
    expires_at?: string;
  } | null;
  error_message: string | null;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
  updated_at: string;
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
  components: {
    task_worker: "healthy" | "degraded";
    event_relay: "healthy" | "degraded";
  };
  workers: WorkerStatus[];
  task_counts: Record<string, number>;
  oldest_pending_at: string | null;
  checked_at: string;
}

export interface RiskEvent {
  id: number;
  event_type: string;
  severity: "info" | "warning" | "danger";
  subject_kind: string | null;
  subject_id: string | null;
  ip_address: string | null;
  detail: Record<string, unknown> | null;
  status: "open" | "acknowledged" | "resolved" | "ignored";
  assigned_to: number | null;
  resolution_note: string | null;
  resolved_by: number | null;
  resolved_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface RiskSummary {
  open_total: number;
  severity_counts: Record<"info" | "warning" | "danger", number>;
  status_counts: Record<RiskEvent["status"], number>;
  recent_24h: number;
  top_types: Array<{ event_type: string; count: number }>;
  checked_at: string;
}

export interface RiskRule {
  id: number;
  name: string;
  event_pattern: string;
  severity: "info" | "warning" | "danger";
  threshold_count: number;
  window_minutes: number;
  cooldown_minutes: number;
  enabled: boolean;
  telegram_alert: boolean;
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface ServiceProbe {
  id: number;
  service_name: string;
  service_kind: string;
  status: "healthy" | "unhealthy";
  latency_ms: number | null;
  status_code: number | null;
  message: string | null;
  detail: Record<string, unknown> | null;
  checked_at: string;
}

export interface DiagnosticSummary {
  status: "healthy" | "degraded";
  services: ServiceProbe[];
  history: ServiceProbe[];
  checked_at: string;
}

export interface AlertDelivery {
  id: string;
  security_event_id: number;
  recipient_tg: number;
  status: "pending" | "sent" | "failed";
  attempt_count: number;
  error_message: string | null;
  event_type: string | null;
  severity: "info" | "warning" | "danger" | null;
  created_at: string;
  sent_at: string | null;
  updated_at: string;
}

export interface AccountLifecycleEvent {
  id: number;
  account_id: string | null;
  batch_id: string;
  tg: number;
  action: string;
  status: "succeeded" | "failed";
  detail: Record<string, unknown> | null;
  actor_kind: string;
  actor_id: string;
  created_at: string;
}

export interface BillingReconciliation {
  status: "healthy" | "attention";
  status_counts: Record<string, number>;
  stale_pending: number;
  credited_without_ledger: number;
  credited_without_ledger_ids: string[];
  duplicate_credit_entries: number;
  checked_at: string;
}

export type DynamicSettingValue = boolean | number | string;

export interface DynamicSetting {
  key: string;
  group: string;
  label: string;
  description: string;
  value: DynamicSettingValue;
  value_type: "boolean" | "integer" | "string";
  source: "config" | "database";
  revision: number;
  minimum: number | null;
  maximum: number | null;
  options: string[];
  restart_required: boolean;
  updated_by: string | null;
  updated_at: string | null;
}

export interface ConfigRevision {
  id: number;
  setting_key: string;
  revision: number;
  old_value: DynamicSettingValue;
  new_value: DynamicSettingValue;
  actor_kind: string;
  actor_id: string;
  created_at: string;
}

export interface DeviceClientRule {
  id: number;
  name: string;
  pattern: string;
  match_type: "exact" | "contains" | "glob" | "regex";
  action: "allow" | "block" | "observe";
  enabled: boolean;
  built_in: boolean;
  priority: number;
  hit_count: number;
  notes: string | null;
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface ManagedCredential {
  id: string;
  name: string;
  provider: string;
  credential_type: "api_token" | "api_key" | "password" | "bearer";
  fingerprint: string;
  metadata: Record<string, unknown>;
  active: boolean;
  last_used_at: string | null;
  expires_at: string | null;
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface EmbyInstance {
  id: string;
  name: string;
  base_url: string;
  credential_id: string;
  enabled: boolean;
  is_default: boolean;
  verify_tls: boolean;
  priority: number;
  status: "unknown" | "healthy" | "unhealthy";
  last_error: string | null;
  last_latency_ms: number | null;
  last_checked_at: string | null;
  binding_count: number;
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface MediaCatalogItem {
  provider: "tmdb";
  media_type: "movie" | "tv";
  provider_id: string;
  external_ref: string;
  title: string;
  original_title: string | null;
  year: number | null;
  overview: string | null;
  poster_url: string | null;
  backdrop_url: string | null;
  vote_average: number;
  genres: Array<{ id: number; name: string }>;
  cached_until: string;
}

export interface AutomationRule {
  id: string;
  name: string;
  description: string | null;
  trigger_type: "event" | "interval";
  trigger_value: string;
  conditions: Record<string, unknown>;
  actions: Array<Record<string, unknown>>;
  enabled: boolean;
  cooldown_seconds: number;
  last_cursor: number;
  last_run_at: string | null;
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface AutomationRun {
  id: string;
  rule_id: string;
  event_id: number | null;
  status: string;
  action_results: Array<Record<string, unknown>>;
  error_message: string | null;
  started_at: string;
  finished_at: string | null;
  created_at: string;
}

export interface ApiClient {
  id: string;
  name: string;
  key_prefix: string;
  scopes: string[];
  active: boolean;
  expires_at: string | null;
  last_used_at: string | null;
  last_ip: string | null;
  created_at: string;
  updated_at: string;
}

export interface BackupArtifact {
  name: string;
  size: number;
  created_at: string;
  sha256: string;
}
