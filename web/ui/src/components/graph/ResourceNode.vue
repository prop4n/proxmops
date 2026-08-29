<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import ResourceCard from '@/components/ResourceCard.vue'
import { NODE_WIDTH, type ResourceNodeData } from '@/lib/graph'

// Wraps the shared ResourceCard as a graph node.

defineProps<{ data: ResourceNodeData }>()
</script>

<template>
  <Handle type="target" :position="Position.Left" class="!border-border !bg-muted-foreground/40" />
  <div :style="{ width: `${NODE_WIDTH.resource}px` }">
    <ResourceCard :resource="data.resource" />
  </div>
  <Handle type="source" :position="Position.Right" class="!border-border !bg-muted-foreground/40" />
</template>
