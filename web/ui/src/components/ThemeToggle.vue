<script setup lang="ts">
import { Moon, Sun } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/composables/useTheme'

const { theme, toggle } = useTheme()
</script>

<template>
  <Button variant="ghost" size="icon" aria-label="Toggle theme" @click="toggle">
    <Sun v-if="theme === 'dark'" class="size-5" />
    <Moon v-else class="size-5" />
  </Button>
</template>
