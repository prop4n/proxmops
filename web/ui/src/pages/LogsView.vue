<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Pause, Play } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { api, type LogEntry } from '@/lib/api'

// The daemon log tail: the retained ring buffer loaded once, then live lines
// streamed over SSE. Auto-scrolls to the bottom unless the user pauses.

const entries = ref<LogEntry[]>([])
const following = ref(true)
const viewport = ref<HTMLElement | null>(null)
let source: EventSource | undefined

const MAX = 2000

function levelClass(level: string): string {
  switch (level) {
    case 'ERROR': return 'text-destructive'
    case 'WARN': return 'text-amber-600 dark:text-amber-400'
    default: return 'text-muted-foreground'
  }
}

function fmtTime(t: string): string {
  const d = new Date(t)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString()
}

function scrollToBottom() {
  if (!following.value) return
  nextTick(() => {
    const el = viewport.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

watch(entries, scrollToBottom, { deep: true })

onMounted(async () => {
  try {
    entries.value = await api.logs()
  } catch {
    // Start empty if the snapshot fails; the stream may still work.
  }
  scrollToBottom()

  source = new EventSource('/api/v1/logs/stream')
  source.addEventListener('log', (e: MessageEvent) => {
    const entry = JSON.parse((e as MessageEvent).data) as LogEntry
    entries.value.push(entry)
    if (entries.value.length > MAX) entries.value.splice(0, entries.value.length - MAX)
  })
})

onBeforeUnmount(() => source?.close())
</script>

<template>
  <div class="space-y-5">
    <header class="flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-semibold tracking-tight">Logs</h1>
        <p class="mt-1 text-sm text-muted-foreground">The daemon's recent activity, streamed live.</p>
      </div>
      <Button variant="outline" size="sm" class="gap-1.5" @click="following = !following">
        <component :is="following ? Pause : Play" class="size-4" />
        {{ following ? 'Following' : 'Paused' }}
      </Button>
    </header>

    <div
      ref="viewport"
      class="h-[70vh] min-h-[400px] overflow-y-auto rounded-lg border bg-muted/20 p-3 font-mono text-xs"
    >
      <p v-if="entries.length === 0" class="text-muted-foreground">No log lines yet.</p>
      <div
        v-for="(e, i) in entries"
        :key="i"
        class="flex gap-2 whitespace-pre-wrap break-all py-0.5"
      >
        <span class="shrink-0 text-muted-foreground/60">{{ fmtTime(e.time) }}</span>
        <span :class="['shrink-0 w-12 uppercase', levelClass(e.level)]">{{ e.level }}</span>
        <span>{{ e.message }}</span>
      </div>
    </div>
  </div>
</template>
