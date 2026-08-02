// Code generated from api/openapi.yaml by cmd/openapi-ts; DO NOT EDIT.

export const operations = {
  acknowledgeRiskEvent: { method: "POST", path: "/api/v3/admin/risk-events/{id}/acknowledged" },
  addTicketMessage: { method: "POST", path: "/api/v3/admin/tickets/{id}/messages" },
  adjustAccountWallet: { method: "POST", path: "/api/v3/admin/accounts/{id}/wallet-adjustments" },
  adminClaimEmbyUser: { method: "POST", path: "/api/v3/admin/emby/remote-users/{id}/claim" },
  adoptLegacyEmbyIdentities: { method: "POST", path: "/api/v3/admin/emby/instances/{id}/adopt-legacy" },
  assignAccountRoles: { method: "PUT", path: "/api/v3/admin/accounts/{id}/roles" },
  assignMembership: { method: "POST", path: "/api/v3/admin/accounts/{id}/membership" },
  cancelBatchOperation: { method: "POST", path: "/api/v3/admin/batch-operations/{id}/cancel" },
  changeAccountLifecycle: { method: "PATCH", path: "/api/v3/admin/accounts/{id}/lifecycle" },
  claimImportedEmbyUser: { method: "POST", path: "/api/v3/me/emby-claims" },
  claimTelegramNotification: { method: "GET", path: "/api/v3/internal/notifications/telegram/next" },
  confirmPaymentCallback: { method: "POST", path: "/api/v3/internal/payments/{provider}/callback" },
  createAPIClient: { method: "POST", path: "/api/v3/admin/api-clients" },
  createAccountTag: { method: "POST", path: "/api/v3/admin/account-tags" },
  createAutomationRule: { method: "POST", path: "/api/v3/admin/automation-rules" },
  createBatchOperation: { method: "POST", path: "/api/v3/admin/batch-operations" },
  createBroadcast: { method: "POST", path: "/api/v3/admin/broadcasts" },
  createDeviceRule: { method: "POST", path: "/api/v3/admin/device-rules" },
  createEmbyClaimToken: { method: "POST", path: "/api/v3/admin/emby/remote-users/{id}/claim-token" },
  createEmbyInstance: { method: "POST", path: "/api/v3/admin/emby/instances" },
  createLine: { method: "POST", path: "/api/v3/admin/lines" },
  createMediaRequest: { method: "POST", path: "/api/v3/me/media-requests" },
  createMembershipPlan: { method: "POST", path: "/api/v3/admin/membership-plans" },
  createMembershipProduct: { method: "POST", path: "/api/v3/admin/membership-products" },
  createRechargeOrder: { method: "POST", path: "/api/v3/me/recharge-orders" },
  createRechargeProduct: { method: "POST", path: "/api/v3/admin/recharge-products" },
  createRiskRule: { method: "POST", path: "/api/v3/admin/risk-rules" },
  createTelegramLinkRequest: { method: "POST", path: "/api/v3/me/telegram/link-requests" },
  createTicket: { method: "POST", path: "/api/v3/me/tickets" },
  deleteCredential: { method: "DELETE", path: "/api/v3/admin/credentials/{name}" },
  enqueueEmbyInstanceTask: { method: "POST", path: "/api/v3/admin/emby/instances/{id}/tasks" },
  executeBotAction: { method: "POST", path: "/api/v3/internal/bot/actions" },
  generateEntitlementCodes: { method: "POST", path: "/api/v3/admin/entitlement-codes" },
  generateInvitations: { method: "POST", path: "/api/v3/admin/invitations" },
  getAccessContext: { method: "GET", path: "/api/v3/auth/context" },
  getAccount: { method: "GET", path: "/api/v3/admin/accounts/{id}" },
  getAccountWallet: { method: "GET", path: "/api/v3/admin/accounts/{id}/wallet" },
  getBatchOperation: { method: "GET", path: "/api/v3/admin/batch-operations/{id}" },
  getCurrentAccount: { method: "GET", path: "/api/v3/me" },
  getDynamicSettingHistory: { method: "GET", path: "/api/v3/admin/settings/{key}/history" },
  getLiveness: { method: "GET", path: "/health/live" },
  getMedia: { method: "GET", path: "/api/v3/media/{id}" },
  getMediaRequest: { method: "GET", path: "/api/v3/admin/media-requests/{id}" },
  getMyEmbyProvisioning: { method: "GET", path: "/api/v3/me/emby/provision-requests/{id}" },
  getMyMediaRequest: { method: "GET", path: "/api/v3/me/media-requests/{id}" },
  getMyMembership: { method: "GET", path: "/api/v3/me/membership" },
  getMyRechargeOrder: { method: "GET", path: "/api/v3/me/recharge-orders/{id}" },
  getMyTicket: { method: "GET", path: "/api/v3/me/tickets/{id}" },
  getMyWallet: { method: "GET", path: "/api/v3/me/wallet" },
  getOpenAPISpec: { method: "GET", path: "/openapi.yaml" },
  getOpenSystemInfo: { method: "GET", path: "/open/v1/system/info" },
  getReadiness: { method: "GET", path: "/health/ready" },
  getRiskEvent: { method: "GET", path: "/api/v3/admin/risk-events/{id}" },
  getSession: { method: "GET", path: "/api/v3/auth/session" },
  getSystemInfo: { method: "GET", path: "/api/v3/system/info" },
  getTelegramLinkRequest: { method: "GET", path: "/api/v3/me/telegram/link-requests/{id}" },
  getTicket: { method: "GET", path: "/api/v3/admin/tickets/{id}" },
  grantAccountEntitlement: { method: "POST", path: "/api/v3/admin/entitlements" },
  listAPIClients: { method: "GET", path: "/api/v3/admin/api-clients" },
  listAPIScopes: { method: "GET", path: "/api/v3/admin/api-scopes" },
  listAccountEntitlements: { method: "GET", path: "/api/v3/admin/entitlements" },
  listAccountTags: { method: "GET", path: "/api/v3/admin/account-tags" },
  listAccounts: { method: "GET", path: "/api/v3/admin/accounts" },
  listAllMembershipProducts: { method: "GET", path: "/api/v3/admin/membership-products" },
  listAllRechargeProducts: { method: "GET", path: "/api/v3/admin/recharge-products" },
  listAuditLogs: { method: "GET", path: "/api/v3/admin/audit" },
  listAutomationExecutions: { method: "GET", path: "/api/v3/admin/automation-executions" },
  listAutomationRules: { method: "GET", path: "/api/v3/admin/automation-rules" },
  listAvailableLines: { method: "GET", path: "/api/v3/lines" },
  listBatchOperationItems: { method: "GET", path: "/api/v3/admin/batch-operations/{id}/items" },
  listBatchOperations: { method: "GET", path: "/api/v3/admin/batch-operations" },
  listBroadcasts: { method: "GET", path: "/api/v3/admin/broadcasts" },
  listCredentialMetadata: { method: "GET", path: "/api/v3/admin/credentials" },
  listDeviceProfiles: { method: "GET", path: "/api/v3/admin/devices" },
  listDeviceRules: { method: "GET", path: "/api/v3/admin/device-rules" },
  listDynamicSettings: { method: "GET", path: "/api/v3/admin/settings" },
  listEmbyBindings: { method: "GET", path: "/api/v3/admin/emby/bindings" },
  listEmbyInstances: { method: "GET", path: "/api/v3/admin/emby/instances" },
  listEmbySnapshots: { method: "GET", path: "/api/v3/admin/emby/instances/{id}/snapshots" },
  listEmbyTasks: { method: "GET", path: "/api/v3/admin/emby/tasks" },
  listEntitlementCodes: { method: "GET", path: "/api/v3/admin/entitlement-codes" },
  listFavorites: { method: "GET", path: "/api/v3/admin/favorites" },
  listImportedEmbyUsers: { method: "GET", path: "/api/v3/admin/emby/remote-users" },
  listIntegrationProbes: { method: "GET", path: "/api/v3/admin/integration-probes" },
  listInvitations: { method: "GET", path: "/api/v3/admin/invitations" },
  listLines: { method: "GET", path: "/api/v3/admin/lines" },
  listMediaCatalog: { method: "GET", path: "/api/v3/media" },
  listMediaMatches: { method: "GET", path: "/api/v3/admin/media/{id}/matches" },
  listMediaRequests: { method: "GET", path: "/api/v3/admin/media-requests" },
  listMembershipPlans: { method: "GET", path: "/api/v3/admin/membership-plans" },
  listMoviePilotJobs: { method: "GET", path: "/api/v3/admin/moviepilot-jobs" },
  listMyDevices: { method: "GET", path: "/api/v3/me/devices" },
  listMyEmbyBindings: { method: "GET", path: "/api/v3/me/emby-bindings" },
  listMyEntitlements: { method: "GET", path: "/api/v3/me/entitlements" },
  listMyFavorites: { method: "GET", path: "/api/v3/me/favorites" },
  listMyLedger: { method: "GET", path: "/api/v3/me/wallet/ledger" },
  listMyMediaRequests: { method: "GET", path: "/api/v3/me/media-requests" },
  listMyNotifications: { method: "GET", path: "/api/v3/me/notifications" },
  listMyOnlinePlayback: { method: "GET", path: "/api/v3/me/playback/online" },
  listMyPlaybackHistory: { method: "GET", path: "/api/v3/me/playback/history" },
  listMyRechargeOrders: { method: "GET", path: "/api/v3/me/recharge-orders" },
  listMyReviews: { method: "GET", path: "/api/v3/me/reviews" },
  listMyTicketMessages: { method: "GET", path: "/api/v3/me/tickets/{id}/messages" },
  listMyTickets: { method: "GET", path: "/api/v3/me/tickets" },
  listNotificationPreferences: { method: "GET", path: "/api/v3/me/notification-preferences" },
  listOnlinePlayback: { method: "GET", path: "/api/v3/admin/playback/online" },
  listPermissions: { method: "GET", path: "/api/v3/admin/permissions" },
  listPlaybackHistory: { method: "GET", path: "/api/v3/admin/playback/history" },
  listPublicMembershipPlans: { method: "GET", path: "/api/v3/membership-plans" },
  listPublicMembershipProducts: { method: "GET", path: "/api/v3/membership-products" },
  listPublicRechargeProducts: { method: "GET", path: "/api/v3/recharge-products" },
  listPublicReviews: { method: "GET", path: "/api/v3/reviews" },
  listRechargeOrders: { method: "GET", path: "/api/v3/admin/recharge-orders" },
  listRechargeRefunds: { method: "GET", path: "/api/v3/admin/recharge-refunds" },
  listReviewReports: { method: "GET", path: "/api/v3/admin/review-reports" },
  listReviewsForModeration: { method: "GET", path: "/api/v3/admin/reviews" },
  listRiskEvents: { method: "GET", path: "/api/v3/admin/risk-events" },
  listRiskRules: { method: "GET", path: "/api/v3/admin/risk-rules" },
  listRoles: { method: "GET", path: "/api/v3/admin/roles" },
  listTicketMessages: { method: "GET", path: "/api/v3/admin/tickets/{id}/messages" },
  listTickets: { method: "GET", path: "/api/v3/admin/tickets" },
  loginLocalAccount: { method: "POST", path: "/api/v3/auth/login" },
  logout: { method: "POST", path: "/api/v3/auth/logout" },
  markNotificationRead: { method: "POST", path: "/api/v3/me/notifications/{id}/read" },
  markRiskEventFalsePositive: { method: "POST", path: "/api/v3/admin/risk-events/{id}/false-positive" },
  moderateReview: { method: "PUT", path: "/api/v3/admin/reviews/{id}/moderation" },
  pauseBatchOperation: { method: "POST", path: "/api/v3/admin/batch-operations/{id}/pause" },
  probeIntegration: { method: "POST", path: "/api/v3/admin/integrations/{integration}/probe" },
  probeLine: { method: "POST", path: "/api/v3/admin/lines/{id}/probe" },
  purchaseMembershipWithWallet: { method: "POST", path: "/api/v3/me/membership-purchases" },
  putCredential: { method: "PUT", path: "/api/v3/admin/credentials/{name}" },
  redeemEntitlementCode: { method: "POST", path: "/api/v3/me/entitlements/redeem" },
  redeemInvitation: { method: "POST", path: "/api/v3/me/invitations/redeem" },
  refreshMediaMatches: { method: "POST", path: "/api/v3/admin/media/{id}/match-tasks" },
  refundRechargeOrder: { method: "POST", path: "/api/v3/admin/recharge-orders/{id}/refunds" },
  registerLocalAccount: { method: "POST", path: "/api/v3/auth/register" },
  replyToMyTicket: { method: "POST", path: "/api/v3/me/tickets/{id}/messages" },
  reportReview: { method: "POST", path: "/api/v3/reviews/{id}/reports" },
  requestEmbyProvisioning: { method: "POST", path: "/api/v3/me/emby/provision-requests" },
  resolveReviewReport: { method: "PUT", path: "/api/v3/admin/review-reports/{id}" },
  resolveRiskEvent: { method: "POST", path: "/api/v3/admin/risk-events/{id}/resolved" },
  resumeBatchOperation: { method: "POST", path: "/api/v3/admin/batch-operations/{id}/resume" },
  retryBatchOperation: { method: "POST", path: "/api/v3/admin/batch-operations/{id}/retry" },
  retryEmbyTask: { method: "POST", path: "/api/v3/admin/emby/tasks/{id}/retry" },
  revokeAPIClient: { method: "DELETE", path: "/api/v3/admin/api-clients/{id}" },
  revokeAccountEntitlement: { method: "POST", path: "/api/v3/admin/entitlements/{id}/revoke" },
  revokeInvitation: { method: "DELETE", path: "/api/v3/admin/invitations/{id}" },
  rollbackDynamicSetting: { method: "POST", path: "/api/v3/admin/settings/{key}/rollback" },
  saveCustomRole: { method: "PUT", path: "/api/v3/admin/roles/{code}" },
  searchMoviePilotResources: { method: "GET", path: "/api/v3/admin/media-requests/{id}/moviepilot/resources" },
  searchTMDBMedia: { method: "GET", path: "/api/v3/media/search" },
  setMyFavorite: { method: "POST", path: "/api/v3/me/favorites" },
  setNotificationPreference: { method: "PUT", path: "/api/v3/me/notification-preferences/{event}/{channel}" },
  setReviewLike: { method: "PUT", path: "/api/v3/reviews/{id}/like" },
  streamAdminRealtime: { method: "GET", path: "/api/v3/admin/realtime" },
  streamUserRealtime: { method: "GET", path: "/api/v3/me/realtime" },
  submitMoviePilotResource: { method: "POST", path: "/api/v3/admin/media-requests/{id}/moviepilot" },
  submitReview: { method: "POST", path: "/api/v3/me/reviews" },
  syncMyFavorites: { method: "POST", path: "/api/v3/me/favorites/sync" },
  updateAccountTag: { method: "PUT", path: "/api/v3/admin/account-tags/{id}" },
  updateAutomationRule: { method: "PUT", path: "/api/v3/admin/automation-rules/{id}" },
  updateDeviceRule: { method: "PUT", path: "/api/v3/admin/device-rules/{id}" },
  updateDynamicSetting: { method: "PATCH", path: "/api/v3/admin/settings/{key}" },
  updateEmbyInstance: { method: "PUT", path: "/api/v3/admin/emby/instances/{id}" },
  updateLine: { method: "PUT", path: "/api/v3/admin/lines/{id}" },
  updateMediaRequestStatus: { method: "PUT", path: "/api/v3/admin/media-requests/{id}/status" },
  updateMembershipPlan: { method: "PUT", path: "/api/v3/admin/membership-plans/{id}" },
  updateMembershipProduct: { method: "PUT", path: "/api/v3/admin/membership-products/{id}" },
  updateRechargeProduct: { method: "PUT", path: "/api/v3/admin/recharge-products/{id}" },
  updateRiskRule: { method: "PUT", path: "/api/v3/admin/risk-rules/{id}" },
  updateTicket: { method: "PUT", path: "/api/v3/admin/tickets/{id}" },
} as const;

export type OperationId = keyof typeof operations;
export type RequestOptions = { path?: Record<string, string | number>; query?: Record<string, string | number | boolean | undefined | null>; body?: unknown; headers?: HeadersInit; signal?: AbortSignal };

export class GeneratedApiClient {
  constructor(private readonly baseURL = "/api/v3", private readonly csrfCookie = "sakura_v3_session_csrf") {}

  async call<T>(operationId: OperationId, options: RequestOptions = {}): Promise<T> {
    const operation = operations[operationId];
    let path: string = operation.path;
    const usesApiBase = path.startsWith("/api/v3");
    if (usesApiBase) path = path.slice("/api/v3".length) || "/";
    for (const [key, value] of Object.entries(options.path || {})) {
      path = path.replace("{" + key + "}", encodeURIComponent(String(value)));
    }
    if (/[{][^}]+[}]/.test(path)) throw new Error("Missing path parameter for " + operationId);
    const url = new URL((usesApiBase ? this.baseURL.replace(/\/$/, "") : "") + path, window.location.origin);
    for (const [key, value] of Object.entries(options.query || {})) {
      if (value !== undefined && value !== null && value !== "") url.searchParams.set(key, String(value));
    }
    const headers = new Headers(options.headers);
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (!["GET", "HEAD"].includes(operation.method)) {
      const cookies = document.cookie.split("; ");
      const csrfEntry = cookies.find((entry) => entry.startsWith(this.csrfCookie + "=")) || cookies.find((entry) => entry.split("=", 1)[0].endsWith("_csrf"));
      const csrf = csrfEntry?.split("=").slice(1).join("=");
      if (csrf) headers.set("X-CSRF-Token", decodeURIComponent(csrf));
    }
    const response = await fetch(url, { method: operation.method, headers, credentials: "include", body: options.body === undefined ? undefined : JSON.stringify(options.body), signal: options.signal });
    if (response.status === 204) return undefined as T;
    const contentType = response.headers.get("content-type") || "";
    const payload = contentType.includes("json") ? await response.json() : await response.text();
    if (!response.ok) {
      const message = typeof payload === "object" && payload && "message" in payload ? String(payload.message) : "Request failed (" + response.status + ")";
      throw new ApiError(message, response.status, payload);
    }
    return payload as T;
  }
}

export class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly detail: unknown) { super(message); }
}
