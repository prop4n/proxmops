<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { LayoutGrid, RefreshCw, Settings2, Workflow } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import ResourceCard from '@/components/ResourceCard.vue'
import StatusLed from '@/components/StatusLed.vue'
import SchemaGraph from '@/components/graph/SchemaGraph.vue'
import { useSync } from '@/stores/sync'
import { formatAge } from '@/lib/utils'
import { kindOf, kindOrder, type KindMeta } from '@/lib/kinds'
import { api, type ResourceStatus, type SettingsSnapshot } from '@/lib/api'

// Overview with two views: the schema graph and the grouped cards.

type ViewMode = 'flow' | 'cards'
const view = ref<ViewMode>(storedView())

function storedView(): ViewMode {
  return localStorage.getItem('proxmops-view') === 'cards' ? 'cards' : 'flow'
}

function setView(mode: ViewMode) {
  view.value = mode
  localStorage.setItem('proxmops-view', mode)
}

const sync = useSync()
const { ledState, statusLabel, notConfigured, counts, inSync } = sync

const snapshot = computed(() => sync.state.snapshot)
const resources = computed(() => snapshot.value?.resources ?? [])

interface Group extends KindMeta {
  kind: string
  resources: ResourceStatus[]
}

const groups = computed<Group[]>(() => {
  const byKind = new Map<string, ResourceStatus[]>()
  for (const r of resources.value) {
    const list = byKind.get(r.kind) ?? []
    list.push(r)
    byKind.set(r.kind, list)
  }
  return [...byKind.keys()]
    .sort((a, b) => kindOrder(a) - kindOrder(b) || a.localeCompare(b))
    .map((kind) => {
      const meta = kindOf(kind)
      return { kind, resources: byKind.get(kind)!, ...meta }
    })
})

const source = ref<SettingsSnapshot | null>(null)

onMounted(async () => {
  try {
    source.value = await api.getSettings()
  } catch {
    // Degrades to an unconfigured source node.
  }
})
</script>

<template>
  <div class="space-y-5">
    <header class="flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-semibold tracking-tight">Overview</h1>
        <p class="mt-1 text-sm text-muted-foreground">Managed resources and their sync status.</p>
      </div>
      <div class="flex items-center gap-1">
        <div class="flex rounded-md border p-0.5" role="group" aria-label="View mode">
          <Button
            v-for="mode in [
              { id: 'flow', icon: Workflow, label: 'Schema view' },
              { id: 'cards', icon: LayoutGrid, label: 'Card view' },
            ] as const"
            :key="mode.id"
            :variant="view === mode.id ? 'secondary' : 'ghost'"
            size="icon"
            class="size-7"
            :aria-label="mode.label"
            :title="mode.label"
            @click="setView(mode.id)"
          >
            <component :is="mode.icon" class="size-4" />
          </Button>
        </div>
        <Button variant="outline" size="icon" aria-label="Refresh" :disabled="sync.state.loading" @click="sync.refresh()">
          <RefreshCw class="size-4" :class="{ 'animate-spin': sync.state.loading }" />
        </Button>
      </div>
    </header>

    <!-- Status toolbar -->
    <div class="flex flex-wrap items-center gap-2">
      <div class="flex items-center gap-2 rounded-md border bg-card px-3 py-1.5 text-sm">
        <span class="text-[11px] font-medium tracking-wider text-muted-foreground uppercase">Sync</span>
        <StatusLed :state="ledState" :pulse="!inSync" />
        {{ statusLabel }}
      </div>
      <div v-if="snapshot" class="flex items-center gap-2 rounded-md border bg-card px-3 py-1.5 text-sm">
        <span class="text-[11px] font-medium tracking-wider text-muted-foreground uppercase">Resources</span>
        <span class="font-mono text-xs">{{ counts.synced }} synced · {{ counts.outOfSync }} out of sync</span>
      </div>
      <div
        v-if="snapshot?.updatedAt"
        class="flex items-center gap-2 rounded-md border bg-card px-3 py-1.5 text-sm"
        :title="new Date(snapshot.updatedAt).toLocaleString()"
      >
        <span class="text-[11px] font-medium tracking-wider text-muted-foreground uppercase">Reconciled</span>
        <span class="font-mono text-xs">{{ formatAge(snapshot.updatedAt) }} ago</span>
      </div>
    </div>

    <p v-if="snapshot?.error && !notConfigured" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
      {{ snapshot.error }}
    </p>

    <div v-if="notConfigured" class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-dashed px-4 py-3">
      <p class="text-sm text-muted-foreground">The daemon has no target yet — set the cluster and repository.</p>
      <Button variant="outline" size="sm" as-child>
        <RouterLink to="/settings">
          <Settings2 class="size-4" /> Open settings
        </RouterLink>
      </Button>
    </div>

    <div
      v-else-if="!snapshot || snapshot.resources.length === 0"
      class="rounded-lg border border-dashed p-10 text-center text-sm text-muted-foreground"
    >
      No managed resources yet. Add manifests to your repository and they
      will appear here.
    </div>

    <!-- Schema view: interactive topology graph -->
    <SchemaGraph v-else-if="view === 'flow'" :resources="resources" :source="source" />

    <!-- Card view: tiles grouped by kind -->
    <div v-else class="space-y-6">
      <section v-for="g in groups" :key="g.kind" class="space-y-2.5">
        <div class="flex items-center gap-2 px-0.5">
          <component :is="g.icon" class="size-4 text-muted-foreground" />
          <h3 class="text-sm font-medium">{{ g.label }}</h3>
          <span class="font-mono text-xs text-muted-foreground">
            {{ g.resources.filter(r => r.state === 'Synced').length }}/{{ g.resources.length }} synced
          </span>
        </div>
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <ResourceCard v-for="r in g.resources" :key="r.kind + '/' + r.name" :resource="r" />
        </div>
      </section>
    </div>
  </div>
</template>
