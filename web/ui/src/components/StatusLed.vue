<script setup lang="ts">
import { computed } from 'vue'
import type { LedState } from '@/stores/sync'

// The sync LED: one dot, one meaning. Steady green when reconciled, amber
// pulse when the cluster drifted, red on failure, muted when unknown. The
// pulse is reserved for states that need attention.

const props = withDefaults(
  defineProps<{ state?: LedState, pulse?: boolean }>(),
  { state: 'unknown', pulse: false },
)

const dotClass = computed(
  () =>
    ({
      synced: 'bg-emerald-500',
      drifted: 'bg-amber-500',
      error: 'bg-destructive',
      unknown: 'bg-muted-foreground/40',
    })[props.state],
)
</script>

<template>
  <span class="relative inline-flex size-2.5 shrink-0" aria-hidden="true">
    <span
      v-if="pulse && (state === 'drifted' || state === 'error')"
      :class="['absolute inline-flex size-full animate-ping rounded-full opacity-50', dotClass]"
    />
    <span :class="['relative inline-flex size-2.5 rounded-full', dotClass]" />
  </span>
</template>
