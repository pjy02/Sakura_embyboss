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
