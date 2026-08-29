import { createRouter, createWebHistory, type RouteLocationNormalized } from 'vue-router'
import { useAuth } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: () => import('@/pages/DashboardView.vue'), meta: { requiresAuth: true } },
    { path: '/login', name: 'login', component: () => import('@/pages/LoginView.vue') },
    { path: '/setup', name: 'setup', component: () => import('@/pages/SetupView.vue') },
  ],
})

// Guard: force first-run setup, then require authentication for protected routes.
router.beforeEach(async (to: RouteLocationNormalized) => {
  const { state, refresh } = useAuth()
  if (!state.loaded) {
    await refresh().catch(() => undefined)
  }

  if (state.needsSetup) {
    return to.name === 'setup' ? true : { name: 'setup' }
  }
  if (to.name === 'setup') {
    return { name: state.user ? 'dashboard' : 'login' }
  }
  if (to.meta.requiresAuth && !state.user) {
    return { name: 'login' }
  }
  if (to.name === 'login' && state.user) {
    return { name: 'dashboard' }
  }
  return true
})

export default router
