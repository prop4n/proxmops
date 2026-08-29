// Minimal typed client for the proxmops JSON API. Requests are same-origin so
// the session cookie is sent automatically.

export interface ApiError {
  status: number
  message: string
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  const text = await res.text()
  const body = text ? JSON.parse(text) : null
  if (!res.ok) {
    const err: ApiError = { status: res.status, message: body?.error ?? res.statusText }
    throw err
  }
  return body as T
}

export interface Account {
  id: number
  username: string
}

export const api = {
  setupStatus: () => request<{ needsSetup: boolean }>('/setup'),
  setup: (token: string, username: string, password: string) =>
    request<{ username: string }>('/setup', {
      method: 'POST',
      body: JSON.stringify({ token, username, password }),
    }),
  login: (username: string, password: string) =>
    request<{ username: string }>('/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  logout: () => request<null>('/logout', { method: 'POST' }),
  me: () => request<Account>('/me'),
}
