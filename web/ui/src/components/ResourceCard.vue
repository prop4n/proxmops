<script setup lang="ts">
import { computed, ref } from 'vue'
import { Check, History, Loader2, Trash2, X } from 'lucide-vue-next'
import StatusLed from '@/components/StatusLed.vue'
import { kindOf } from '@/lib/kinds'
import { formatAge } from '@/lib/utils'
import { api, type ApiError, type ResourceEvent, type ResourceStatus } from '@/lib/api'

// Resource tile with a left status stripe; shared by the cards view and graph.
// A trash action deletes the resource from the cluster (ISO only for now); it
// reappears on the next reconcile if still declared in the repo.

const props = defineProps<{ resource: ResourceStatus }>()

const drifted = computed(() => props.resource.state === 'OutOfSync')
const meta = computed(() => kindOf(props.resource.kind))
const deletable = computed(() => props.resource.kind === 'Iso' || props.resource.kind === 'VirtualMachine')

const confirming = ref(false)
const deleting = ref(false)
const error = ref('')

async function doDelete() {
  deleting.value = true
  error.value = ''
  try {
    await api.deleteResource(props.resource.kind, props.resource.name)
    confirming.value = false
  } catch (e) {
    error.value = (e as ApiError)?.message ?? 'delete failed'
  } finally {
    deleting.value = false
  }
}

// Resource history: loaded on first open of the timeline.
const showHistory = ref(false)
const events = ref<ResourceEvent[]>([])
const loadingHistory = ref(false)

async function toggleHistory() {
  showHistory.value = !showHistory.value
  if (showHistory.value && events.value.length === 0) {
    loadingHistory.value = true
    try {
      events.value = await api.resourceEvents(props.resource.kind, props.resource.name)
    } catch {
      // Leave the timeline empty on error.
    } finally {
      loadingHistory.value = false
    }
  }
}

function eventLabel(e: ResourceEvent): string {
  const when = formatAge(e.at)
  const ago = when ? `${when} ago` : 'just now'
  const detail = e.reason ? `${e.type}: ${e.reason}` : e.type
  return e.commit ? `${detail} · ${e.commit.slice(0, 7)} · ${ago}` : `${detail} · ${ago}`
}

function age(r: ResourceStatus): string {
  const a = formatAge(r.lastTransition)
  if (!a) return r.state === 'Synced' ? 'in sync' : 'out of sync'
  return r.state === 'Synced' ? `in sync for ${a}` : `out of sync for ${a}`
}

function action(r: ResourceStatus): string {
  return r.reason ? `${r.action}: ${r.reason}` : (r.action ?? '')
}
</script>

<template>
  <div
    :class="[
      'group relative overflow-hidden rounded-lg border bg-card pl-4 pr-3 py-3 transition-colors hover:border-ring/60',
      drifted && 'border-amber-500/40',
    ]"
  >
    <!-- Status stripe -->
    <span
      aria-hidden="true"
      :class="[
        'absolute inset-y-0 left-0 w-1',
        drifted ? 'bg-amber-500' : 'bg-emerald-500',
      ]"
    />
    <div class="flex items-start gap-2.5">
      <span class="flex size-8 shrink-0 items-center justify-center rounded-md bg-secondary text-secondary-foreground">
        <component :is="meta.icon" class="size-4" />
      </span>
      <div class="min-w-0 flex-1">
        <div class="flex items-center justify-between gap-2">
          <span class="truncate text-sm font-medium">{{ resource.name }}</span>
          <div class="flex items-center gap-1.5">
            <StatusLed :state="drifted ? 'drifted' : 'synced'" />

            <!-- History toggle -->
            <button
              type="button"
              title="History"
              :class="[
                'nodrag rounded p-0.5 transition hover:text-foreground focus-visible:opacity-100',
                showHistory ? 'text-foreground' : 'text-muted-foreground opacity-0 group-hover:opacity-100',
              ]"
              @click="toggleHistory"
            >
              <History class="size-3.5" />
            </button>

            <!-- Delete: two-step inline confirm -->
            <div v-if="confirming" class="nodrag flex items-center gap-0.5">
              <button
                type="button"
                :disabled="deleting"
                title="Confirm delete"
                class="nodrag rounded p-0.5 text-destructive hover:bg-destructive/10 disabled:opacity-50"
                @click="doDelete"
              >
                <Loader2 v-if="deleting" class="size-3.5 animate-spin" />
                <Check v-else class="size-3.5" />
              </button>
              <button
                type="button"
                :disabled="deleting"
                title="Cancel"
                class="nodrag rounded p-0.5 text-muted-foreground hover:bg-accent"
                @click="confirming = false"
              >
                <X class="size-3.5" />
              </button>
            </div>
            <button
              v-else
              type="button"
              :disabled="!deletable"
              :title="deletable ? 'Delete from cluster' : `Deletion not available for ${meta.label} yet`"
              class="nodrag rounded p-0.5 text-muted-foreground opacity-0 transition group-hover:opacity-100 focus-visible:opacity-100 hover:text-destructive disabled:cursor-not-allowed disabled:opacity-30 group-hover:disabled:opacity-30"
              @click="confirming = true"
            >
              <Trash2 class="size-3.5" />
            </button>
          </div>
        </div>
        <p class="mt-0.5 truncate font-mono text-xs text-muted-foreground">
          {{ meta.label }}<template v-if="resource.vmid"> · #{{ resource.vmid }}</template><template v-if="resource.node"> · {{ resource.node }}</template>
        </p>
        <p
          v-if="error"
          class="mt-1.5 truncate text-xs text-destructive"
          :title="error"
        >
          {{ error }}
        </p>
        <p
          v-else-if="resource.action"
          class="mt-1.5 truncate text-xs text-amber-700 dark:text-amber-400"
          :title="action(resource)"
        >
          {{ action(resource) }}
        </p>
        <p class="mt-1.5 text-xs text-muted-foreground">{{ age(resource) }}</p>

        <!-- History timeline -->
        <div v-if="showHistory" class="nodrag mt-2 border-t pt-2">
          <p v-if="loadingHistory" class="text-xs text-muted-foreground">Loading history...</p>
          <p v-else-if="events.length === 0" class="text-xs text-muted-foreground">No history yet.</p>
          <ul v-else class="space-y-1">
            <li
              v-for="e in events"
              :key="e.id"
              class="flex items-center gap-1.5 text-xs text-muted-foreground"
            >
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
        </div>
      </div>
    </div>
  </div>
</template>
