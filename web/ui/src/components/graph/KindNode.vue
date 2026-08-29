<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import { NODE_WIDTH, type KindNodeData } from '@/lib/graph'

defineProps<{ data: KindNodeData }>()
</script>

<template>
  <Handle type="target" :position="Position.Left" class="!border-border !bg-muted-foreground/40" />
  <div
    :style="{ width: `${NODE_WIDTH.kind}px` }"
    :class="[
      'flex items-center gap-2.5 rounded-lg border bg-card px-3 py-2.5',
      data.drifted && 'border-amber-500/40',
    ]"
  >
    <span class="flex size-8 shrink-0 items-center justify-center rounded-md bg-secondary text-secondary-foreground">
      <component :is="data.icon" class="size-4" />
    </span>
    <div class="min-w-0 flex-1">
      <p class="truncate text-sm font-medium">{{ data.label }}</p>
      <p class="font-mono text-xs text-muted-foreground">{{ data.synced }}/{{ data.total }} synced</p>
    </div>
  </div>
  <Handle type="source" :position="Position.Right" class="!border-border !bg-muted-foreground/40" />
</template>
