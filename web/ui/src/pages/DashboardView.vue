<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { RefreshCw, CheckCircle2, AlertTriangle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from '@/components/ui/card'
import ThemeToggle from '@/components/ThemeToggle.vue'
import { useAuth } from '@/stores/auth'
import { api, type StatusSnapshot } from '@/lib/api'

const router = useRouter()
const { state, logout } = useAuth()

const snapshot = ref<StatusSnapshot | null>(null)
const loading = ref(false)
let timer: number | undefined

async function refresh() {
  loading.value = true
  try {
    snapshot.value = await api.resources()
  } catch {
    // Leave the previous snapshot in place on transient errors.
  } finally {
    loading.value = false
  }
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleTimeString()
}

onMounted(() => {
  refresh()
  timer = window.setInterval(refresh, 5000)
})
onUnmounted(() => window.clearInterval(timer))

async function onLogout() {
  await logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="min-h-screen bg-background text-foreground">
    <header class="border-b">
      <div class="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
        <div class="flex items-center gap-2">
          <span class="text-lg font-semibold tracking-tight">proxmops</span>
          <span class="rounded bg-secondary px-2 py-0.5 text-xs text-secondary-foreground">
            GitOps for Proxmox
          </span>
        </div>
        <div class="flex items-center gap-3">
          <ThemeToggle />
          <span class="text-sm text-muted-foreground">{{ state.user?.username }}</span>
          <Button variant="outline" size="sm" @click="onLogout">Sign out</Button>
        </div>
      </div>
    </header>

    <main class="mx-auto max-w-6xl px-6 py-8">
      <Card>
        <CardHeader>
          <div class="flex items-start justify-between">
            <div>
              <CardTitle class="flex items-center gap-2">
                Applications
                <template v-if="snapshot">
                  <span v-if="snapshot.inSync" class="inline-flex items-center gap-1 text-sm font-normal text-emerald-600 dark:text-emerald-400">
                    <CheckCircle2 class="size-4" /> In sync
                  </span>
                  <span v-else class="inline-flex items-center gap-1 text-sm font-normal text-amber-600 dark:text-amber-400">
                    <AlertTriangle class="size-4" /> Out of sync
                  </span>
                </template>
              </CardTitle>
              <CardDescription>
                <template v-if="snapshot">
                  Last reconciled at {{ formatTime(snapshot.updatedAt) }}
                </template>
                <template v-else>Sync status of your managed resources.</template>
              </CardDescription>
            </div>
            <Button variant="ghost" size="icon" aria-label="Refresh" :disabled="loading" @click="refresh">
              <RefreshCw class="size-4" :class="{ 'animate-spin': loading }" />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <p v-if="snapshot?.error" class="mb-4 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {{ snapshot.error }}
          </p>

          <div
            v-if="!snapshot || snapshot.resources.length === 0"
            class="rounded-lg border border-dashed p-10 text-center text-sm text-muted-foreground"
          >
            No managed resources yet. Add manifests to your repository and they
            will appear here.
          </div>

          <ul v-else class="divide-y rounded-lg border">
            <li
              v-for="r in snapshot.resources"
              :key="r.kind + '/' + r.name"
              class="flex items-center justify-between px-4 py-3"
            >
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <span class="font-medium">{{ r.name }}</span>
                  <span class="text-xs text-muted-foreground">{{ r.kind }}</span>
                  <span v-if="r.node" class="text-xs text-muted-foreground">· {{ r.node }}</span>
                </div>
                <p v-if="r.action" class="truncate text-xs text-muted-foreground">
                  {{ r.action }}<template v-if="r.reason"> — {{ r.reason }}</template>
                </p>
              </div>
              <Badge :variant="r.state === 'Synced' ? 'secondary' : 'destructive'">
                {{ r.state }}
              </Badge>
            </li>
          </ul>
        </CardContent>
      </Card>
    </main>
  </div>
</template>
