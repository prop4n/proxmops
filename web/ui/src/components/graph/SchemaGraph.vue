<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
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

// Interactive topology graph: source → kind → resource, laid out by dagre.

const props = defineProps<{
  resources: ResourceStatus[]
  source: SettingsSnapshot | null
}>()

const driftOnly = ref(false)

const { fitView } = useVueFlow()

function build() {
  return buildGraph({ resources: props.resources, source: props.source, driftOnly: driftOnly.value })
}

const initial = build()
const nodes = ref(initial.nodes)
const edges = ref(initial.edges)

// Rebuild only when the shape changes, so drag positions survive snapshot pushes.
const signature = computed(() => {
  const rs = props.resources
    .map(r => `${r.kind}/${r.name}:${r.state}`)
    .sort()
    .join('|')
  return `${driftOnly.value}::${props.source?.configured ?? false}::${rs}`
})

// Capped below 1:1 so a small graph opens slightly zoomed out.
const fitOptions = { padding: 0.25, maxZoom: 0.75 }

function minimapColor(node: { data?: { drifted?: boolean, resource?: ResourceStatus } }): string {
  const drifted = node.data?.drifted || node.data?.resource?.state === 'OutOfSync'
  return drifted ? '#f59e0b' : 'var(--muted-foreground)'
}

watch(signature, () => {
  const next = build()
  nodes.value = next.nodes
  edges.value = next.edges
  nextTick(() => fitView(fitOptions))
})
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
