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

export type SyncState = 'Synced' | 'OutOfSync'

export interface ResourceStatus {
  kind: string
  name: string
  node?: string
  vmid?: number
  state: SyncState
  action?: string
  reason?: string
  /** When the resource entered its current state. */
  lastTransition?: string
}

export interface StatusSnapshot {
  updatedAt: string
  inSync: boolean
  /** False when the daemon has no target yet; the UI must show the setup path. */
  configured: boolean
  /** Git commit the desired state was read from; empty for a local source. */
  commit?: string
  error?: string
  resources: ResourceStatus[]
}

export interface ClusterSettings {
  endpoint: string
  tokenId: string
  tokenSecret: string
  tokenSecretSet: boolean
  insecureSkipVerify: boolean
}

export interface SourceSettings {
  repoURL: string
  path: string
  revision: string
  username: string
  token: string
  tokenSet: boolean
}

export interface ReconcileSettings {
  intervalSeconds: number
  autoSync: boolean
  prune: boolean
  dryRun: boolean
  concurrency: number
}

export interface SettingsSnapshot {
  configured: boolean
  cluster: ClusterSettings
  source: SourceSettings
  reconcile: ReconcileSettings
  updatedAt?: string
}

export interface SettingsTestProbe {
  ok: boolean
  error?: string
}

export interface SettingsTestResult {
  cluster: SettingsTestProbe
  source: SettingsTestProbe
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
  resources: () => request<StatusSnapshot>('/resources'),
  deleteResource: (kind: string, name: string) =>
    request<null>(`/resources/${encodeURIComponent(kind)}/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  getSettings: () => request<SettingsSnapshot>('/settings'),
  saveSettings: (settings: SettingsSnapshot) =>
    request<SettingsSnapshot>('/settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    }),
  testSettings: () => request<SettingsTestResult>('/settings/test', { method: 'POST', body: '{}' }),
}
