import { runtime } from "@/lib/runtime";

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public detail?: unknown,
  ) {
    super(message);
  }
}

function getCookie(name: string) {
  return document.cookie
    .split("; ")
    .find((row) => row.startsWith(`${name}=`))
    ?.split("=")
    .slice(1)
    .join("=");
}

function errorText(payload: unknown, status: number) {
  if (typeof payload === "object" && payload && "detail" in payload) {
    const detail = (payload as { detail: unknown }).detail;
    if (typeof detail === "string") return detail;
  }
  return status === 401 ? "登录状态已失效，请重新登录" : "请求失败，请稍后重试";
}

export async function api<T>(
  path: string,
  init: RequestInit & { idempotencyKey?: string } = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (init.method && !["GET", "HEAD"].includes(init.method.toUpperCase())) {
    const csrf = getCookie(runtime.csrfCookieName);
    if (csrf) headers.set("X-CSRF-Token", decodeURIComponent(csrf));
  }
  if (init.idempotencyKey) headers.set("Idempotency-Key", init.idempotencyKey);

  const response = await fetch(`${runtime.apiBase}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });
  if (response.status === 204) return undefined as T;

  const contentType = response.headers.get("content-type") || "";
  const payload = contentType.includes("application/json")
    ? await response.json()
    : await response.text();
  if (!response.ok) {
    throw new ApiError(errorText(payload, response.status), response.status, payload);
  }
  return payload as T;
}

export function idempotencyKey(prefix: string) {
  return `${prefix}-${Date.now()}-${crypto.randomUUID()}`;
}
