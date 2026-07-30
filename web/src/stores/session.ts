import { defineStore } from "pinia";
import { api, ApiError } from "@/lib/api";
import type { Session } from "@/types";

export const useSessionStore = defineStore("session", {
  state: () => ({
    session: null as Session | null,
    checked: false,
  }),
  getters: {
    authenticated: (state) => Boolean(state.session),
    isAdministrator: (state) =>
      Boolean(
        state.session?.roles.some((role) =>
          ["owner", "admin", "operator", "auditor"].includes(role),
        ),
      ),
  },
  actions: {
    async load() {
      try {
        this.session = await api<Session>("/auth/session");
      } catch (error) {
        if (!(error instanceof ApiError) || error.status !== 401) throw error;
        this.session = null;
      } finally {
        this.checked = true;
      }
      return this.session;
    },
    async logout(all = false) {
      await api(all ? "/auth/logout-all" : "/auth/logout", { method: "POST" });
      this.session = null;
    },
  },
});
