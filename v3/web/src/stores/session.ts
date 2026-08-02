import { defineStore } from 'pinia'
import { api, type AccessContext } from '../lib/api'

export const useSessionStore = defineStore('session', {
  state: () => ({ context: null as AccessContext | null, ready: false, busy: false, error: '' }),
  getters: {
    account: (state) => state.context?.account ?? null,
    username: (state) => state.context?.account.identities?.find((identity) => identity.kind === 'local')?.username || state.context?.account.username || '',
    permissions: (state) => new Set(state.context?.permissions ?? []),
    isAdmin(): boolean { return this.permissions.has('dashboard.read') },
  },
  actions: {
    async restore() {
      try { this.context = await api.call<AccessContext>('getAccessContext') }
      catch { this.context = null }
      finally { this.ready = true }
    },
    async login(username: string, password: string) {
      this.busy = true; this.error = ''
      try {
        await api.call('loginLocalAccount', { body: { username, password } })
        this.context = await api.call<AccessContext>('getAccessContext')
      } catch (error) { this.error = error instanceof Error ? error.message : '登录失败'; throw error }
      finally { this.busy = false }
    },
    async register(username: string, displayName: string, password: string) {
      this.busy = true; this.error = ''
      try {
        await api.call('registerLocalAccount', { body: { username, display_name: displayName, password } })
        await this.login(username, password)
      } catch (error) { this.error = error instanceof Error ? error.message : '注册失败'; throw error }
      finally { this.busy = false }
    },
    async logout() { await api.call('logout'); this.context = null },
    has(permission?: string) { return !permission || this.permissions.has(permission) },
  },
})
