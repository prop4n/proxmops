import { computed, reactive } from 'vue'
import { api, type StatusSnapshot } from '@/lib/api'

// Shared sync-status store, following the same plain-composable pattern as
// the auth store. The AppShell opens the SSE stream once and every page reads
// the latest snapshot from here, so the daemon heartbeat lives in one place.
// The manual refresh() stays available for the overview's refresh button.

export type LedState = 'synced' | 'drifted' | 'error' | 'unknown'

interface SyncState {
  snapshot: StatusSnapshot | null
  loading: boolean
  connected: boolean
}

const state = reactive<SyncState>({ snapshot: null, loading: false, connected: false })

let source: EventSource | undefined

// refresh fetches the latest status snapshot over a plain request, keeping the
// previous one on transient errors so the UI never blanks out.
async function refresh(): Promise<void> {
  state.loading = true
  try {
    state.snapshot = await api.resources()
  } catch {
    // Leave the previous snapshot in place.
  } finally {
    state.loading = false
  }
}

// connect opens the Server-Sent Events stream; the server pushes a snapshot
// on connect and after every reconciliation pass. The browser reconnects on
// its own when the stream drops.
function connect(): void {
  if (source) return
  source = new EventSource('/api/v1/events')
  source.addEventListener('snapshot', (e: MessageEvent) => {
    state.connected = true
    state.snapshot = JSON.parse((e as MessageEvent).data) as StatusSnapshot
  })
  source.onerror = () => {
    state.connected = false
  }
}

function disconnect(): void {
  source?.close()
  source = undefined
  state.connected = false
}

const inSync = computed(() => state.snapshot?.inSync ?? false)

// The snapshot carries a structured configured flag, so the UI never has to
// parse the daemon's error message.
const notConfigured = computed(
  () => state.snapshot !== null && !state.snapshot.configured,
)

const counts = computed(() => {
  const resources = state.snapshot?.resources ?? []
  return {
    synced: resources.filter(r => r.state === 'Synced').length,
    outOfSync: resources.filter(r => r.state === 'OutOfSync').length,
  }
})

const ledState = computed<LedState>(() => {
  const s = state.snapshot
  if (!s || notConfigured.value) return 'unknown'
  if (s.error) return 'error'
  return s.inSync ? 'synced' : 'drifted'
})

const statusLabel = computed(
  () =>
    ({
      synced: 'In sync',
      drifted: 'Out of sync',
      error: 'Reconcile error',
      unknown: state.snapshot ? 'Not configured' : 'Waiting for first pass',
    })[ledState.value],
)

export function useSync() {
  return {
    state, refresh, connect, disconnect,
    inSync, notConfigured, counts, ledState, statusLabel,
  }
}
