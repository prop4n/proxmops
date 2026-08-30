<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { VueFlow, useVueFlow } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'
import { Eye, EyeOff } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import SourceNode from '@/components/graph/SourceNode.vue'
import KindNode from '@/components/graph/KindNode.vue'
import ResourceNode from '@/components/graph/ResourceNode.vue'
import { buildGraph } from '@/lib/graph'
import type { ResourceStatus, SettingsSnapshot } from '@/lib/api'

// Interactive topology graph: source -> kind -> resource, laid out by dagre.

const props = defineProps<{
  resources: ResourceStatus[]
  source: SettingsSnapshot | null
}>()

const driftOnly = ref(false)

const { fitView, updateNodeData } = useVueFlow()

function build() {
  return buildGraph({ resources: props.resources, source: props.source, driftOnly: driftOnly.value })
}

const initial = build()
const nodes = ref(initial.nodes)
const edges = ref(initial.edges)

// The structural key covers only what changes the graph's shape (which nodes
// exist and the filter), never a resource's sync state. A plain state flip
// (Synced ↔ OutOfSync) therefore updates node visuals in place, without a
// relayout or a fitView that would jarringly re-zoom the canvas.
function structuralKey(): string {
  const visible = driftOnly.value
    ? props.resources.filter(r => r.state === 'OutOfSync')
    : props.resources
  const keys = visible.map(r => `${r.kind}/${r.name}`).sort().join('|')
  return `${driftOnly.value}::${props.source?.configured ?? false}::${keys}`
}
let lastStructural = structuralKey()

// Capped below 1:1 so a small graph opens slightly zoomed out.
const fitOptions = { padding: 0.25, maxZoom: 0.75 }

function minimapColor(node: { data?: { drifted?: boolean, resource?: ResourceStatus } }): string {
  const drifted = node.data?.drifted || node.data?.resource?.state === 'OutOfSync'
  return drifted ? '#f59e0b' : 'var(--muted-foreground)'
}

watch(
  () => [props.resources, props.source, driftOnly.value],
  () => {
    const next = build()
    const key = structuralKey()
    if (key !== lastStructural) {
      // Shape changed: relayout and refit.
      lastStructural = key
      nodes.value = next.nodes
      edges.value = next.edges
      nextTick(() => fitView(fitOptions))
      return
    }
    // State-only change: patch data in place, keep positions and zoom.
    for (const n of next.nodes) updateNodeData(n.id, n.data)
    edges.value = next.edges
  },
  { deep: true },
)
</script>

<template>
  <div class="relative h-[70vh] min-h-[520px] overflow-hidden rounded-lg border bg-muted/20">
    <VueFlow
      :nodes="nodes"
      :edges="edges"
      :min-zoom="0.25"
      :max-zoom="1.5"
      fit-view-on-init
      :fit-view-options="fitOptions"
    >
      <template #node-source="{ data }">
        <SourceNode :data="data" />
      </template>
      <template #node-kind="{ data }">
        <KindNode :data="data" />
      </template>
      <template #node-resource="{ data }">
        <ResourceNode :data="data" />
      </template>

      <Background :gap="18" :size="1" pattern-color="var(--muted-foreground)" />
      <Controls position="bottom-left" :show-interactive="false" />
      <MiniMap pannable zoomable :node-color="minimapColor" />
    </VueFlow>

    <!-- Filter toggle -->
    <div class="absolute right-3 top-3 z-10">
      <Button
        :variant="driftOnly ? 'secondary' : 'outline'"
        size="sm"
        class="gap-1.5 bg-card"
        @click="driftOnly = !driftOnly"
      >
        <component :is="driftOnly ? EyeOff : Eye" class="size-4" />
        {{ driftOnly ? 'Drift only' : 'All resources' }}
      </Button>
    </div>
  </div>
</template>
