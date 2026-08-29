<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Button } from '@/components/ui/button'
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from '@/components/ui/card'
import ThemeToggle from '@/components/ThemeToggle.vue'
import { useAuth } from '@/stores/auth'

const router = useRouter()
const { state, logout } = useAuth()

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
          <CardTitle>Applications</CardTitle>
          <CardDescription>
            Sync status of your managed resources will appear here.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div class="rounded-lg border border-dashed p-10 text-center text-sm text-muted-foreground">
            No sync status yet. The reconciliation dashboard lands in the next
            iteration.
          </div>
        </CardContent>
      </Card>
    </main>
  </div>
</template>
