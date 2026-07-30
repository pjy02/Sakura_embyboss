/// <reference types="vite/client" />

declare const __SAKURA_BUILD_AREA__: "portal" | "admin";

interface SakuraRuntimeConfig {
  apiBase: string;
  area: "portal" | "admin";
  basePath?: string;
  portalPath?: string;
  adminPath?: string;
  botUsername?: string;
  csrfCookieName?: string;
}

interface Window {
  __SAKURA_CONFIG__?: SakuraRuntimeConfig;
}
