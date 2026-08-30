<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle,
} from '@/components/ui/card'
import ThemeToggle from '@/components/ThemeToggle.vue'
import { useAuth } from '@/stores/auth'
import type { ApiError } from '@/lib/api'

const router = useRouter()
const { setup } = useAuth()

const token = ref('')
const username = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

async function onSubmit() {
  error.value = ''
  busy.value = true
  try {
    await setup(token.value.trim(), username.value.trim(), password.value)
    router.push({ name: 'dashboard' })
  } catch (e) {
    error.value = (e as ApiError).message ?? 'Setup failed'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="relative flex min-h-screen items-center justify-center bg-background p-4">
    <div class="absolute right-4 top-4">
      <ThemeToggle />
    </div>
    <Card class="w-full max-w-md">
      <CardHeader>
        <CardTitle>Welcome to proxmops</CardTitle>
        <CardDescription>
          Create the first admin account. Paste the one-time setup token printed
          in the server logs at startup.
        </CardDescription>
      </CardHeader>
      <form @submit.prevent="onSubmit">
        <CardContent class="grid gap-4">
          <div class="grid gap-2">
            <Label for="token">Setup token</Label>
            <Input id="token" v-model="token" required autocomplete="off" />
          </div>
          <div class="grid gap-2">
            <Label for="username">Username</Label>
            <Input id="username" v-model="username" required autocomplete="username" />
          </div>
          <div class="grid gap-2">
            <Label for="password">Password</Label>
            <Input id="password" v-model="password" type="password" required autocomplete="new-password" />
          </div>
          <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
        </CardContent>
        <CardFooter class="mt-2">
          <Button type="submit" class="w-full" :disabled="busy">
            {{ busy ? 'Creating...' : 'Create admin account' }}
          </Button>
        </CardFooter>
      </form>
    </Card>
  </div>
</template>
