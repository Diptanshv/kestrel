export class ApiError extends Error {
  // Declared explicitly rather than as constructor parameter properties,
  // which `erasableSyntaxOnly` in the Vite template's tsconfig forbids.
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

function csrfToken(): string {
  const match = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]*)/)
  return match ? decodeURIComponent(match[1]) : ''
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const method = init.method ?? 'GET'
  const headers = new Headers(init.headers)
  if (init.body) headers.set('Content-Type', 'application/json')
  // The server only checks CSRF on state-changing methods.
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) {
    headers.set('X-CSRF-Token', csrfToken())
  }

  const res = await fetch(path, { ...init, headers, credentials: 'include' })

  if (res.status === 204) return undefined as T
  const text = await res.text()
  const body = text ? JSON.parse(text) : null

  if (!res.ok) {
    const err = body?.error
    throw new ApiError(res.status, err?.code ?? 'unknown', err?.message ?? res.statusText)
  }
  return body as T
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  patch: <T>(path: string, body: unknown) =>
    request<T>(path, { method: 'PATCH', body: JSON.stringify(body) }),
  del: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}

export type User = { id: number; email: string; created_at: string }
export type Site = { id: number; domain: string; public_slug: string | null; created_at: string }
export type Summary = { from: string; to: string; pageviews: number; visitors: number }
export type TimeseriesPoint = { bucket: string; pageviews: number; visitors: number }
export type Timeseries = { interval: string; points: TimeseriesPoint[] }
export type BreakdownRow = { value: string; pageviews: number; visitors: number }
export type Breakdown = { dimension: string; rows: BreakdownRow[] }