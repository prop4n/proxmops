<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { Handle, Position } from '@vue-flow/core'
import { GitBranch } from 'lucide-vue-next'
import { NODE_WIDTH, type SourceNodeData } from '@/lib/graph'

// The git source node; degrades to a setup link when unconfigured.

defineProps<{ data: SourceNodeData }>()
</script>

<template>
  <div :style="{ width: `${NODE_WIDTH.source}px` }" class="rounded-lg border bg-card p-3">
    <div class="flex items-center gap-2.5">
      <span class="flex size-8 shrink-0 items-center justify-center rounded-md bg-brand/10 text-brand">
        <GitBranch class="size-4" />
      </span>
      <div class="min-w-0">
        <p class="text-[11px] font-medium tracking-wider text-muted-foreground uppercase">Source</p>
        <p class="truncate text-sm font-medium">
          {{ data.configured ? 'Git repository' : 'Not configured' }}
        </p>
      </div>
    </div>
    <div v-if="data.configured" class="mt-2.5 space-y-1 border-t pt-2.5 font-mono text-xs text-muted-foreground">
      <p class="truncate" :title="data.repoURL">{{ data.repoURL }}</p>
      <p class="truncate" :title="data.path ? `${data.revision} · ${data.path}` : data.revision">
        {{ data.revision }}<template v-if="data.path"> · {{ data.path }}</template>
      </p>
    </div>
    <RouterLink
      v-else
      to="/settings"
      class="mt-2.5 block border-t pt-2.5 text-xs text-muted-foreground hover:text-foreground"
    >
      Open settings to set it up
    </RouterLink>
  </div>
  <Handle type="source" :position="Position.Right" class="!border-border !bg-muted-foreground/40" />
</template>
