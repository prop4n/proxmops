<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle,
} from '@/components/ui/card'
import { useAuth } from '@/stores/auth'
import type { ApiError } from '@/lib/api'

const router = useRouter()
const { login } = useAuth()

const username = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

async function onSubmit() {
  error.value = ''
  busy.value = true
  try {
    await login(username.value.trim(), password.value)
    router.push({ name: 'dashboard' })
  } catch (e) {
    error.value = (e as ApiError).message ?? 'Login failed'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-background p-4">
    <Card class="w-full max-w-sm">
      <CardHeader>
        <CardTitle>Sign in</CardTitle>
        <CardDescription>Access your proxmops dashboard.</CardDescription>
      </CardHeader>
      <form @submit.prevent="onSubmit">
        <CardContent class="grid gap-4">
          <div class="grid gap-2">
            <Label for="username">Username</Label>
            <Input id="username" v-model="username" required autocomplete="username" />
          </div>
          <div class="grid gap-2">
            <Label for="password">Password</Label>
            <Input id="password" v-model="password" type="password" required autocomplete="current-password" />
          </div>
          <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
        </CardContent>
        <CardFooter class="mt-2">
          <Button type="submit" class="w-full" :disabled="busy">
            {{ busy ? 'Signing in…' : 'Sign in' }}
          </Button>
        </CardFooter>
      </form>
    </Card>
  </div>
</template>
