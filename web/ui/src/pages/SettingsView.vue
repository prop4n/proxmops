<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { CheckCircle2, XCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from '@/components/ui/card'
import {
  api, type SettingsSnapshot, type SettingsTestResult,
} from '@/lib/api'

const loading = ref(true)
const saving = ref(false)
const testing = ref(false)
const message = ref<{ kind: 'ok' | 'error', text: string } | null>(null)
const testResult = ref<SettingsTestResult | null>(null)

const form = reactive<SettingsSnapshot>({
  configured: false,
  cluster: { endpoint: '', tokenId: '', tokenSecret: '', tokenSecretSet: false, insecureSkipVerify: false },
  source: { repoURL: '', path: '', revision: 'main', username: '', token: '', tokenSet: false },
  reconcile: { intervalSeconds: 60, autoSync: true, prune: true, dryRun: false, concurrency: 4 },
})

function applySnapshot(s: SettingsSnapshot) {
  form.configured = s.configured
  form.cluster = { ...s.cluster, tokenSecret: '' }
  form.source = { ...s.source, token: '' }
  form.reconcile = { ...s.reconcile }
}

async function load() {
  try {
    applySnapshot(await api.getSettings())
  } finally {
    loading.value = false
  }
}

async function save(): Promise<boolean> {
  saving.value = true
  message.value = null
  try {
    applySnapshot(await api.saveSettings({
      configured: form.configured,
      cluster: { ...form.cluster },
      source: { ...form.source },
      reconcile: { ...form.reconcile },
    }))
    message.value = { kind: 'ok', text: 'Settings saved. They apply from the next reconciliation pass.' }
    return true
  } catch (e) {
    message.value = { kind: 'error', text: (e as { message?: string }).message ?? 'Failed to save settings' }
    return false
  } finally {
    saving.value = false
  }
}

async function test() {
  if (!(await save())) return
  testing.value = true
  try {
    testResult.value = await api.testSettings()
  } catch (e) {
    message.value = { kind: 'error', text: (e as { message?: string }).message ?? 'Connection test failed' }
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-6">
    <header>
      <h1 class="text-2xl font-semibold tracking-tight">Settings</h1>
      <p class="mt-1 text-sm text-muted-foreground">
        Cluster, repository, and reconciliation, applied without a restart.
      </p>
    </header>

    <p v-if="message" :class="[
      'rounded-md px-3 py-2 text-sm',
      message.kind === 'ok'
        ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
        : 'bg-destructive/10 text-destructive',
    ]">
      {{ message.text }}
    </p>

    <Card v-if="!loading && !form.configured">
      <CardContent class="pt-6">
        <p class="text-sm text-muted-foreground">
          The daemon is not configured yet. Fill in the cluster connection and
          the Git repository below; no restart is needed.
        </p>
      </CardContent>
    </Card>

    <form class="space-y-6" novalidate @submit.prevent="save">
      <Card>
        <CardHeader>
          <CardTitle>Proxmox cluster</CardTitle>
          <CardDescription>Connection to the Proxmox VE API.</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-4 sm:grid-cols-2">
          <div class="grid gap-2 sm:col-span-2">
            <Label for="endpoint">Endpoint</Label>
            <Input id="endpoint" v-model="form.cluster.endpoint" type="text" placeholder="https://pve.example.com:8006/api2/json" />
          </div>
          <div class="grid gap-2">
            <Label for="tokenId">Token ID</Label>
            <Input id="tokenId" v-model="form.cluster.tokenId" placeholder="proxmops@pve!gitops" />
          </div>
          <div class="grid gap-2">
            <div class="flex items-center justify-between">
              <Label for="tokenSecret">Token secret</Label>
              <Badge v-if="form.cluster.tokenSecretSet" variant="secondary">saved</Badge>
            </div>
            <Input id="tokenSecret" v-model="form.cluster.tokenSecret" type="password" autocomplete="new-password"
              :placeholder="form.cluster.tokenSecretSet ? 'Leave empty to keep the saved secret' : 'API token secret'" />
          </div>
          <label class="flex items-center gap-2 text-sm">
            <input v-model="form.cluster.insecureSkipVerify" type="checkbox" class="size-4 accent-current" />
            Skip TLS certificate verification
          </label>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Git repository</CardTitle>
          <CardDescription>Where the desired state lives.</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-4 sm:grid-cols-2">
          <div class="grid gap-2 sm:col-span-2">
            <Label for="repoURL">Repository URL</Label>
            <Input id="repoURL" v-model="form.source.repoURL" placeholder="https://github.com/you/homelab.git" />
            <p class="text-xs text-muted-foreground">
              A Git remote (https://..., git@...) is cloned; anything else, e.g.
              <code>local</code>, reads the Path below directly from the
              local filesystem.
            </p>
          </div>
          <div class="grid gap-2">
            <Label for="path">Path</Label>
            <Input id="path" v-model="form.source.path" placeholder="proxmox" />
          </div>
          <div class="grid gap-2">
            <Label for="revision">Revision</Label>
            <Input id="revision" v-model="form.source.revision" placeholder="main" />
          </div>
          <div class="grid gap-2">
            <Label for="username">Username</Label>
            <Input id="username" v-model="form.source.username" placeholder="git" />
          </div>
          <div class="grid gap-2">
            <div class="flex items-center justify-between">
              <Label for="token">Access token</Label>
              <Badge v-if="form.source.tokenSet" variant="secondary">saved</Badge>
            </div>
            <Input id="token" v-model="form.source.token" type="password" autocomplete="new-password"
              :placeholder="form.source.tokenSet ? 'Leave empty to keep the saved token' : 'Token for private repositories'" />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Reconciliation</CardTitle>
          <CardDescription>How often and how aggressively the cluster is reconciled.</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-4">
          <div class="grid gap-4 sm:max-w-md sm:grid-cols-2">
            <div class="grid gap-2">
              <Label for="interval">Interval (seconds)</Label>
              <Input id="interval" v-model.number="form.reconcile.intervalSeconds" type="number" min="1" />
            </div>
            <div class="grid gap-2">
              <Label for="concurrency">Parallel actions</Label>
              <Input id="concurrency" v-model.number="form.reconcile.concurrency" type="number" min="1" />
            </div>
          </div>
          <div class="flex flex-wrap gap-6">
            <label class="flex items-center gap-2 text-sm">
              <input v-model="form.reconcile.autoSync" type="checkbox" class="size-4 accent-current" />
              Auto-sync (apply the plan)
            </label>
            <label class="flex items-center gap-2 text-sm">
              <input v-model="form.reconcile.prune" type="checkbox" class="size-4 accent-current" />
              Prune (delete removed owned resources)
            </label>
            <label class="flex items-center gap-2 text-sm">
              <input v-model="form.reconcile.dryRun" type="checkbox" class="size-4 accent-current" />
              Dry run (log without applying)
            </label>
          </div>
        </CardContent>
      </Card>

      <div class="flex items-center gap-3">
        <Button type="submit" :disabled="saving || loading">
          {{ saving ? 'Saving...' : 'Save settings' }}
        </Button>
        <Button type="button" variant="outline" :disabled="testing || saving || loading" @click="test">
          {{ testing ? 'Testing...' : 'Save and test connections' }}
        </Button>
      </div>
    </form>

    <Card v-if="testResult">
      <CardHeader>
        <CardTitle>Connection test</CardTitle>
      </CardHeader>
      <CardContent class="grid gap-2">
        <div class="flex items-start gap-2 text-sm">
          <CheckCircle2 v-if="testResult.cluster.ok" class="mt-0.5 size-4 text-emerald-600 dark:text-emerald-400" />
          <XCircle v-else class="mt-0.5 size-4 text-destructive" />
          <div>
            <span class="font-medium">Proxmox cluster</span>
            <span v-if="!testResult.cluster.error" class="text-muted-foreground">: reachable, token accepted</span>
            <p v-else class="text-destructive">{{ testResult.cluster.error }}</p>
          </div>
        </div>
        <div class="flex items-start gap-2 text-sm">
          <CheckCircle2 v-if="testResult.source.ok" class="mt-0.5 size-4 text-emerald-600 dark:text-emerald-400" />
          <XCircle v-else class="mt-0.5 size-4 text-destructive" />
          <div>
            <span class="font-medium">Git repository</span>
            <span v-if="!testResult.source.error" class="text-muted-foreground">: reachable, credentials accepted</span>
            <p v-else class="text-destructive">{{ testResult.source.error }}</p>
          </div>
        </div>
      </CardContent>
    </Card>
  </div>
</template>
