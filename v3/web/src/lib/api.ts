import { GeneratedApiClient } from '../generated/client'

export const api = new GeneratedApiClient('/api/v3')

export type Account = {
  id: string
  username?: string
  display_name: string
  status: string
  created_at?: string
  identities?: { kind: string; subject?: string; username?: string; verified_at?: string }[]
  roles?: string[]
}

export type AccessContext = { account: Account; permissions: string[] }
export type ItemList<T = Record<string, unknown>> = { items: T[] }

export function idempotencyKey(prefix: string) {
  return `${prefix}-${crypto.randomUUID()}`
}

export function readable(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—'
  if (typeof value === 'boolean') return value ? '是' : '否'
  if (typeof value === 'object') return JSON.stringify(value)
  const text = String(value)
  if (/^\d{4}-\d{2}-\d{2}T/.test(text)) return new Date(text).toLocaleString('zh-CN')
  return text
}
