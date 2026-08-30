<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Check, Loader2, Trash2, X } from 'lucide-vue-next'
import StatusLed from '@/components/StatusLed.vue'
import { kindOf } from '@/lib/kinds'
import { formatAge } from '@/lib/utils'
import { useSelection } from '@/stores/selection'
import { useSync } from '@/stores/sync'
import {
  api,
  type ApiError,
  type ObservedResource,
  type ResourceDetail,
  type ResourceEvent,
  type ResourceStatus,
} from '@/lib/api'

// Right-hand detail drawer: opens on resource selection, showing desired vs
// observed config, actions, and history. Replaces the per-card buttons.

const { state: selection, clear } = useSelection()
const sync = useSync()

const sel = computed(() => selection.current)
const meta = computed(() => (sel.value ? kindOf(sel.value.kind) : null))

// The live status for the selected resource, from the shared snapshot.
const status = computed<ResourceStatus | null>(() => {
  const s = sel.value
  if (!s) return null
  return sync.state.snapshot?.resources.find(r => r.kind === s.kind && r.name === s.name) ?? null
})
const drifted = computed(() => status.value?.state === 'OutOfSync')
const deletable = computed(() => sel.value?.kind === 'Iso' || sel.value?.kind === 'VirtualMachine')

const detail = ref<ResourceDetail | null>(null)
const events = ref<ResourceEvent[]>([])
const loading = ref(false)

const confirming = ref(false)
const deleting = ref(false)
const actionError = ref('')

// Reload detail + history whenever the selection changes.
watch(sel, async (s) => {
  detail.value = null
  events.value = []
  confirming.value = false
  actionError.value = ''
  if (!s) return
  loading.value = true
  try {
    const [d, ev] = await Promise.all([
      api.resourceDetail(s.kind, s.name),
      api.resourceEvents(s.kind, s.name),
    ])
    detail.value = d
    events.value = ev
  } catch {
    // Leave sections empty on error.
  } finally {
    loading.value = false
  }
}, { immediate: true })

async function doDelete() {
  if (!sel.value) return
  deleting.value = true
  actionError.value = ''
  try {
    await api.deleteResource(sel.value.kind, sel.value.name)
    confirming.value = false
  } catch (e) {
    actionError.value = (e as ApiError)?.message ?? 'delete failed'
  } finally {
    deleting.value = false
  }
}

function observedRows(o: ObservedResource): { label: string, value: string }[] {
  if (!o.present) return []
  const rows: { label: string, value: string }[] = []
  if (o.cores) rows.push({ label: 'Cores', value: String(o.cores) })
  if (o.memoryMB) rows.push({ label: 'Memory', value: `${o.memoryMB} MB` })
  if (o.cpu) rows.push({ label: 'CPU', value: o.cpu })
  rows.push({ label: 'State', value: o.running ? 'running' : 'stopped' })
  if (o.ip) rows.push({ label: 'IP', value: o.ip })
  if (o.nameserver) rows.push({ label: 'Nameserver', value: o.nameserver })
  return rows
}

function eventLabel(e: ResourceEvent): string {
  const when = formatAge(e.at)
  const ago = when ? `${when} ago` : 'just now'
  const detailText = e.reason ? `${e.type}: ${e.reason}` : e.type
  return e.commit ? `${detailText} · ${e.commit.slice(0, 7)} · ${ago}` : `${detailText} · ${ago}`
}
</script>

<template>
  <Transition
    enter-active-class="transition-transform duration-200 ease-out"
    leave-active-class="transition-transform duration-150 ease-in"
    enter-from-class="translate-x-full"
    leave-to-class="translate-x-full"
  >
    <aside
      v-if="sel"
      class="fixed inset-y-0 right-0 z-30 flex w-full max-w-md flex-col border-l bg-card shadow-xl"
    >
      <!-- Header -->
      <div class="flex items-start gap-2.5 border-b p-4">
        <span class="flex size-8 shrink-0 items-center justify-center rounded-md bg-secondary text-secondary-foreground">
          <component :is="meta?.icon" class="size-4" />
        </span>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="truncate text-sm font-medium">{{ sel.name }}</span>
            <StatusLed :state="drifted ? 'drifted' : 'synced'" />
          </div>
          <p class="truncate font-mono text-xs text-muted-foreground">
            {{ meta?.label }}<template v-if="status?.vmid"> · #{{ status.vmid }}</template><template v-if="status?.node"> · {{ status.node }}</template>
          </p>
        </div>
        <button
          type="button"
          aria-label="Close"
          class="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
          @click="clear"
        >
          <X class="size-4" />
        </button>
      </div>

      <div class="flex-1 space-y-5 overflow-y-auto p-4">
        <!-- Drift / action line -->
        <p v-if="status?.action" class="text-xs text-amber-700 dark:text-amber-400">
          {{ status.reason ? `${status.action}: ${status.reason}` : status.action }}
        </p>

        <p v-if="loading" class="text-xs text-muted-foreground">Loading...</p>

        <!-- Desired vs observed -->
        <section v-if="detail" class="space-y-3">
          <div>
            <h3 class="mb-1.5 text-[11px] font-medium tracking-wider text-muted-foreground uppercase">Desired</h3>
            <pre class="overflow-x-auto rounded-md border bg-muted/30 p-3 font-mono text-xs">{{ detail.desiredYAML }}</pre>
          </div>
          <div>
            <h3 class="mb-1.5 text-[11px] font-medium tracking-wider text-muted-foreground uppercase">Observed</h3>
            <p v-if="!detail.observed || !detail.observed.present" class="text-xs text-muted-foreground">
              Not present on the cluster.
            </p>
            <dl v-else class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
              <template v-for="row in observedRows(detail.observed)" :key="row.label">
                <dt class="text-muted-foreground">{{ row.label }}</dt>
                <dd class="font-mono">{{ row.value }}</dd>
              </template>
            </dl>
          </div>
        </section>

        <!-- History -->
        <section>
          <h3 class="mb-1.5 text-[11px] font-medium tracking-wider text-muted-foreground uppercase">History</h3>
          <p v-if="!loading && events.length === 0" class="text-xs text-muted-foreground">No history yet.</p>
          <ul v-else class="space-y-1">
            <li v-for="e in events" :key="e.id" class="flex items-center gap-1.5 text-xs text-muted-foreground">
              <span
                aria-hidden="true"
                :class="[
                  'size-1.5 shrink-0 rounded-full',
                  e.type === 'failed' ? 'bg-destructive'
                  : e.type === 'synced' ? 'bg-emerald-500'
                  : e.type === 'drifted' ? 'bg-amber-500'
                  : 'bg-muted-foreground/50',
                ]"
              />
              <span class="truncate font-mono" :title="eventLabel(e)">{{ eventLabel(e) }}</span>
            </li>
          </ul>
        </section>
      </div>

      <!-- Actions -->
      <div class="border-t p-4">
        <p v-if="actionError" class="mb-2 text-xs text-destructive">{{ actionError }}</p>
        <div v-if="confirming" class="flex items-center gap-2">
          <button
            type="button"
            :disabled="deleting"
            class="inline-flex items-center gap-1.5 rounded-md bg-destructive px-3 py-1.5 text-sm text-destructive-foreground hover:opacity-90 disabled:opacity-50"
            @click="doDelete"
          >
            <Loader2 v-if="deleting" class="size-4 animate-spin" />
            <Check v-else class="size-4" />
            Confirm delete
          </button>
          <button
            type="button"
            :disabled="deleting"
            class="rounded-md border px-3 py-1.5 text-sm hover:bg-accent"
            @click="confirming = false"
          >
            Cancel
          </button>
        </div>
        <button
          v-else
          type="button"
          :disabled="!deletable"
          :title="deletable ? 'Delete from cluster' : 'Deletion not available for this kind yet'"
          class="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm text-destructive hover:bg-destructive/10 disabled:cursor-not-allowed disabled:text-muted-foreground disabled:hover:bg-transparent"
          @click="confirming = true"
        >
          <Trash2 class="size-4" /> Delete
        </button>
      </div>
    </aside>
  </Transition>
</template>
