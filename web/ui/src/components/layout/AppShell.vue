<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { LayoutDashboard, LogOut, Settings2 } from 'lucide-vue-next'
import ThemeToggle from '@/components/ThemeToggle.vue'
import StatusLed from '@/components/StatusLed.vue'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/stores/auth'
import { useSync } from '@/stores/sync'

// The application shell: one quiet sidebar sharing the canvas background,
// separated by a border only, carrying the brand, the navigation, and the
// daemon heartbeat in its footer.

const route = useRoute()
const router = useRouter()
const { state, logout } = useAuth()
const sync = useSync()
const { ledState, statusLabel, inSync } = sync

onMounted(() => sync.connect())
onUnmounted(() => sync.disconnect())

const nav = [
  { name: 'dashboard', label: 'Overview', icon: LayoutDashboard, to: '/' },
  { name: 'settings', label: 'Settings', icon: Settings2, to: '/settings' },
]

function isActive(name: string): boolean {
  return route.name === name
}

async function onLogout() {
  await logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="min-h-screen bg-background text-foreground">
    <!-- Desktop sidebar -->
    <aside class="fixed inset-y-0 left-0 z-10 hidden w-60 flex-col border-r md:flex">
      <RouterLink to="/" class="flex items-center gap-2 px-5 pt-5 pb-4">
        <span class="text-lg font-semibold tracking-tight">proxmops</span>
        <span class="size-1.5 rounded-full bg-brand" aria-hidden="true" />
      </RouterLink>

      <nav class="flex flex-col gap-0.5 px-3" aria-label="Main">
        <RouterLink
          v-for="item in nav"
          :key="item.name"
          :to="item.to"
          :class="[
            'relative flex items-center gap-2.5 rounded-md px-3 py-2 text-sm transition-colors',
            isActive(item.name)
              ? 'bg-accent font-medium text-accent-foreground'
              : 'text-muted-foreground hover:bg-accent/60 hover:text-foreground',
          ]"
        >
          <span
            v-if="isActive(item.name)"
            class="absolute top-1/2 left-0 h-4 w-0.5 -translate-y-1/2 rounded-full bg-brand"
            aria-hidden="true"
          />
          <component :is="item.icon" class="size-4" />
          {{ item.label }}
        </RouterLink>
      </nav>

      <div class="mt-auto space-y-1 border-t px-3 py-3">
        <RouterLink
          to="/"
          class="flex items-center gap-2.5 rounded-md px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent/60 hover:text-foreground"
        >
          <StatusLed :state="ledState" :pulse="!inSync" />
          <span class="truncate">{{ statusLabel }}</span>
        </RouterLink>
        <div class="flex items-center justify-between gap-2 px-3 pt-1.5">
          <span class="truncate text-xs text-muted-foreground">{{ state.user?.username }}</span>
          <div class="flex items-center gap-0.5">
            <ThemeToggle />
            <Button variant="ghost" size="icon" aria-label="Sign out" @click="onLogout">
              <LogOut class="size-4" />
            </Button>
          </div>
        </div>
      </div>
    </aside>

    <!-- Mobile topbar -->
    <header class="sticky top-0 z-10 flex items-center justify-between border-b bg-background px-4 py-3 md:hidden">
      <RouterLink to="/" class="flex items-center gap-2">
        <span class="text-base font-semibold tracking-tight">proxmops</span>
        <span class="size-1.5 rounded-full bg-brand" aria-hidden="true" />
      </RouterLink>
      <div class="flex items-center gap-1">
        <Button
          v-for="item in nav"
          :key="item.name"
          :variant="isActive(item.name) ? 'secondary' : 'ghost'"
          size="icon"
          :aria-label="item.label"
          as-child
        >
          <RouterLink :to="item.to">
            <component :is="item.icon" class="size-4" />
          </RouterLink>
        </Button>
        <ThemeToggle />
        <Button variant="ghost" size="icon" aria-label="Sign out" @click="onLogout">
          <LogOut class="size-4" />
        </Button>
      </div>
    </header>

    <main class="md:pl-60">
      <div class="mx-auto max-w-[1600px] px-6 py-8">
        <RouterView />
      </div>
    </main>
  </div>
</template>
