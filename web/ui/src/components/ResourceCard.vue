<script setup lang="ts">
import { computed } from 'vue'
import StatusLed from '@/components/StatusLed.vue'
import { kindOf } from '@/lib/kinds'
import { formatAge } from '@/lib/utils'
import { useSelection } from '@/stores/selection'
import type { ResourceStatus } from '@/lib/api'

// Resource tile with a left status stripe; shared by the cards view and graph.
// Clicking it opens the detail drawer (config, history, actions).

const props = defineProps<{ resource: ResourceStatus }>()

const { state: selection, select } = useSelection()
const drifted = computed(() => props.resource.state === 'OutOfSync')
const meta = computed(() => kindOf(props.resource.kind))
const selected = computed(() =>
  selection.current?.kind === props.resource.kind && selection.current?.name === props.resource.name,
)

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
  <button
    type="button"
    :class="[
      'nodrag group relative w-full overflow-hidden rounded-lg border bg-card pl-4 pr-3 py-3 text-left transition-colors hover:border-ring/60',
      drifted && 'border-amber-500/40',
      selected && 'ring-2 ring-ring',
    ]"
    @click="select(resource.kind, resource.name)"
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
          <StatusLed :state="drifted ? 'drifted' : 'synced'" />
        </div>
        <p class="mt-0.5 truncate font-mono text-xs text-muted-foreground">
          {{ meta.label }}<template v-if="resource.vmid"> · #{{ resource.vmid }}</template><template v-if="resource.node"> · {{ resource.node }}</template>
        </p>
        <p
          v-if="resource.action"
          class="mt-1.5 truncate text-xs text-amber-700 dark:text-amber-400"
          :title="action(resource)"
        >
          {{ action(resource) }}
        </p>
        <p class="mt-1.5 text-xs text-muted-foreground">{{ age(resource) }}</p>
      </div>
    </div>
  </button>
</template>
