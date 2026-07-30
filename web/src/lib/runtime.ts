import type { Area } from "@/types";

const fallbackArea = __SAKURA_BUILD_AREA__;
const fallbackPath = window.location.pathname.replace(/\/+$/, "") || "/";

export const runtime = {
  apiBase: window.__SAKURA_CONFIG__?.apiBase || "/api/v1",
  area: (window.__SAKURA_CONFIG__?.area || fallbackArea) as Area,
  basePath: window.__SAKURA_CONFIG__?.basePath || fallbackPath,
  portalPath: window.__SAKURA_CONFIG__?.portalPath || "/app",
  adminPath: window.__SAKURA_CONFIG__?.adminPath || "",
  botUsername: window.__SAKURA_CONFIG__?.botUsername || "",
  csrfCookieName: window.__SAKURA_CONFIG__?.csrfCookieName || "sakura_csrf",
};
