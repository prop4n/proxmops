import { reactive } from 'vue'
import { api, type Account } from '@/lib/api'

// A tiny reactive auth store shared across the app; Pinia would be overkill for
// this surface.
interface AuthState {
  user: Account | null
  needsSetup: boolean
  loaded: boolean
}

const state = reactive<AuthState>({
  user: null,
  needsSetup: false,
  loaded: false,
})

// refresh loads the current setup/authentication state from the server.
async function refresh(): Promise<void> {
  const { needsSetup } = await api.setupStatus()
  state.needsSetup = needsSetup
  if (needsSetup) {
    state.user = null
  } else {
    state.user = await api.me().catch(() => null)
  }
  state.loaded = true
}

async function setup(token: string, username: string, password: string): Promise<void> {
  await api.setup(token, username, password)
  await refresh()
}

async function login(username: string, password: string): Promise<void> {
  await api.login(username, password)
  await refresh()
}

async function logout(): Promise<void> {
  await api.logout()
  state.user = null
}

export function useAuth() {
  return { state, refresh, setup, login, logout }
}
