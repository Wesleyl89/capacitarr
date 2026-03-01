<template>
  <div>
    <!-- Header -->
    <div data-slot="page-header" class="mb-8 flex flex-col md:flex-row md:items-center justify-between gap-4">
      <div>
        <h1 class="text-3xl font-bold tracking-tight">Settings</h1>
        <p class="text-muted-foreground mt-1.5">
          Manage your service integrations and disk monitoring.
        </p>
      </div>
      <UiButton @click="openAddModal">
        <component :is="PlusIcon" class="w-4 h-4" />
        Add Integration
      </UiButton>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="flex justify-center py-16">
      <component :is="LoaderCircleIcon" class="w-8 h-8 text-primary animate-spin" />
    </div>

    <!-- Empty state -->
    <div
      v-else-if="integrations.length === 0"
      v-motion
      :initial="{ opacity: 0, y: 8 }"
      :enter="{ opacity: 1, y: 0 }"
      class="text-center py-20"
    >
      <component :is="HardDriveIcon" class="w-16 h-16 text-muted-foreground/40 mx-auto mb-4" />
      <h3 class="text-lg font-medium text-foreground mb-2">No integrations configured</h3>
      <p class="text-muted-foreground mb-6">
        Connect your Plex, Sonarr, Radarr, or Tautulli instances to get started.
      </p>
      <UiButton size="lg" @click="openAddModal">
        <component :is="PlusIcon" class="w-4 h-4" />
        Add Your First Integration
      </UiButton>
    </div>

    <!-- Integration Cards Grid -->
    <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
      <div
        v-for="(integration, idx) in integrations"
        :key="integration.id"
        v-motion
        :initial="{ opacity: 0, y: 12 }"
        :enter="{ opacity: 1, y: 0, transition: { type: 'spring', stiffness: 260, damping: 24, delay: 80 * idx } }"
        data-slot="integration-card"
        class="rounded-xl border border-border bg-card shadow-sm overflow-hidden"
      >
        <!-- Card Header -->
        <div class="px-5 py-4 border-b border-border flex items-center justify-between">
          <div class="flex items-center gap-3">
            <div :class="['w-10 h-10 rounded-lg flex items-center justify-center', typeColor(integration.type)]">
              <component :is="typeIcon(integration.type)" class="w-5 h-5 text-white" />
            </div>
            <div>
              <h3 class="font-semibold">{{ integration.name }}</h3>
              <span class="text-xs uppercase tracking-wider font-medium" :class="typeTextColor(integration.type)">
                {{ integration.type }}
              </span>
            </div>
          </div>
          <span
            class="inline-flex items-center px-2 py-0.5 rounded-md text-xs font-medium"
            :class="integration.enabled
              ? 'bg-emerald-100 dark:bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
              : 'bg-muted text-muted-foreground'"
          >
            {{ integration.enabled ? 'Active' : 'Disabled' }}
          </span>
        </div>

        <!-- Card Body -->
        <div class="px-5 py-4 space-y-2 text-sm text-muted-foreground">
          <div class="flex items-center gap-2">
            <component :is="LinkIcon" class="w-3.5 h-3.5 shrink-0" />
            <span class="truncate">{{ integration.url }}</span>
          </div>
          <div class="flex items-center gap-2">
            <component :is="KeyIcon" class="w-3.5 h-3.5 shrink-0" />
            <span class="font-mono text-xs">{{ integration.apiKey }}</span>
          </div>
          <div v-if="integration.lastSync" class="flex items-center gap-2">
            <component :is="ClockIcon" class="w-3.5 h-3.5 shrink-0" />
            <span>Synced {{ formatRelativeTime(integration.lastSync) }}</span>
          </div>
          <div v-if="integration.lastError" class="flex items-center gap-2 text-red-500">
            <component :is="AlertTriangleIcon" class="w-3.5 h-3.5 shrink-0" />
            <span class="text-xs">{{ integration.lastError }}</span>
          </div>
        </div>

        <!-- Card Footer -->
        <div class="px-5 py-3 border-t border-border flex items-center justify-between">
          <div class="flex gap-2">
            <button
              class="h-7 px-2.5 rounded-md text-xs font-medium border border-input bg-input hover:bg-accent transition-colors"
              @click="testConnection(integration)"
            >
              Test
            </button>
            <button
              class="h-7 px-2.5 rounded-md text-xs font-medium border border-input bg-input hover:bg-accent transition-colors"
              @click="openEditModal(integration)"
            >
              Edit
            </button>
          </div>
          <button
            class="h-7 px-2.5 rounded-md text-xs font-medium text-red-500 hover:bg-red-50 dark:hover:bg-red-500/10 transition-colors"
            @click="deleteIntegration(integration)"
          >
            Delete
          </button>
        </div>
      </div>
    </div>

    <!-- Data Management Section -->
    <div
      v-motion
      :initial="{ opacity: 0, y: 12 }"
      :enter="{ opacity: 1, y: 0, transition: { type: 'spring', stiffness: 260, damping: 24, delay: 200 } }"
      class="mt-10 rounded-xl border border-border bg-card shadow-sm overflow-hidden"
    >
      <div class="px-5 py-4 border-b border-border">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-lg bg-primary flex items-center justify-center">
            <component :is="DatabaseIcon" class="w-5 h-5 text-white" />
          </div>
          <div>
            <h3 class="font-semibold">Data Management</h3>
            <span class="text-xs text-muted-foreground">Configure audit log retention</span>
          </div>
        </div>
      </div>
      <div class="px-5 py-5 space-y-4">
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-foreground">Audit Log Retention</label>
          <select
            v-model.number="retentionDays"
            class="w-full max-w-xs h-9 px-3 rounded-lg border border-input bg-input text-sm text-foreground focus:outline-none focus:ring-2 focus-visible:ring-ring/50"
          >
            <option :value="7">7 days</option>
            <option :value="14">14 days</option>
            <option :value="30">30 days (default)</option>
            <option :value="60">60 days</option>
            <option :value="90">90 days</option>
            <option :value="180">180 days</option>
            <option :value="365">365 days</option>
            <option :value="0">Indefinite</option>
          </select>
          <p class="text-xs text-muted-foreground/70">How long to keep audit log entries before automatic cleanup.</p>
        </div>

        <!-- Indefinite warning -->
        <div
          v-if="retentionDays === 0"
          class="rounded-lg bg-amber-50 dark:bg-amber-500/10 border border-amber-200 dark:border-amber-500/20 px-4 py-3 text-sm text-amber-700 dark:text-amber-400"
        >
          ⚠️ <strong>Warning:</strong> Indefinite retention will cause the database to grow continuously. This may eventually impact performance.
        </div>

        <!-- Save button -->
        <div class="flex items-center gap-3">
          <button
            class="h-9 px-4 rounded-lg bg-primary hover:bg-primary/90 text-white text-sm font-medium shadow-sm transition-colors disabled:opacity-50"
            :disabled="savingRetention"
            @click="saveRetention"
          >
            {{ savingRetention ? 'Saving…' : 'Save Retention' }}
          </button>
          <span v-if="retentionSaved" class="text-sm text-emerald-500 font-medium">✓ Saved</span>
        </div>
      </div>
    </div>

    <!-- Display Preferences Section -->
    <div
      v-motion
      :initial="{ opacity: 0, y: 12 }"
      :enter="{ opacity: 1, y: 0, transition: { type: 'spring', stiffness: 260, damping: 24, delay: 280 } }"
      class="mt-6 mb-8 rounded-xl border border-border bg-card shadow-sm overflow-hidden"
    >
      <div class="px-5 py-4 border-b border-border">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-lg bg-purple-500 flex items-center justify-center">
            <component :is="MonitorIcon" class="w-5 h-5 text-white" />
          </div>
          <div>
            <h3 class="font-semibold">Display</h3>
            <span class="text-xs text-muted-foreground">Timezone and clock format preferences (saved locally)</span>
          </div>
        </div>
      </div>
      <div class="px-5 py-5 space-y-5">
        <!-- Timezone -->
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-foreground">Timezone</label>
          <select
            :value="displayTimezone"
            class="w-full max-w-xs h-9 px-3 rounded-lg border border-input bg-input text-sm text-foreground focus:outline-none focus:ring-2 focus-visible:ring-ring/50"
            @change="setTimezone(($event.target as HTMLSelectElement).value)"
          >
            <option value="local">Local (Browser)</option>
            <option value="UTC">UTC</option>
            <option value="America/New_York">America/New_York (Eastern)</option>
            <option value="America/Chicago">America/Chicago (Central)</option>
            <option value="America/Denver">America/Denver (Mountain)</option>
            <option value="America/Los_Angeles">America/Los_Angeles (Pacific)</option>
            <option value="Europe/London">Europe/London</option>
            <option value="Europe/Paris">Europe/Paris</option>
            <option value="Asia/Tokyo">Asia/Tokyo</option>
            <option value="Australia/Sydney">Australia/Sydney</option>
          </select>
        </div>

        <!-- Clock Format -->
        <div class="space-y-1.5">
          <label class="text-sm font-medium text-foreground">Clock Format</label>
          <div class="flex gap-2">
            <button
              class="h-9 px-4 rounded-lg text-sm font-medium border transition-colors"
              :class="displayClockFormat === '12h'
                ? 'border-primary bg-primary/10 text-primary'
                : 'border-input text-muted-foreground hover:bg-accent'"
              @click="setClockFormat('12h')"
            >
              12-hour
            </button>
            <button
              class="h-9 px-4 rounded-lg text-sm font-medium border transition-colors"
              :class="displayClockFormat === '24h'
                ? 'border-primary bg-primary/10 text-primary'
                : 'border-input text-muted-foreground hover:bg-accent'"
              @click="setClockFormat('24h')"
            >
              24-hour
            </button>
          </div>
        </div>

        <!-- Theme -->
        <div class="space-y-2">
          <label class="text-sm font-medium text-foreground">Theme</label>
          <div class="grid grid-cols-3 sm:grid-cols-6 gap-2">
            <button
              v-for="t in themeList"
              :key="t.id"
              class="flex flex-col items-center gap-1.5 rounded-lg border-2 px-3 py-2.5 transition-colors"
              :class="currentTheme === t.id ? 'border-primary bg-primary/5' : 'border-transparent hover:bg-accent'"
              @click="setTheme(t.id)"
            >
              <span
                class="w-6 h-6 rounded-full"
                :style="{ backgroundColor: `oklch(0.6 0.2 ${t.hue})` }"
              />
              <span class="text-xs font-medium">{{ t.label }}</span>
            </button>
          </div>
        </div>

        <p class="text-xs text-muted-foreground/70">Changes apply immediately and are stored in your browser.</p>
      </div>
    </div>

    <!-- Modal (simple overlay) -->
    <Teleport to="body">
      <div
        v-if="showModal"
        class="fixed inset-0 z-50 flex items-center justify-center p-4"
        @click.self="showModal = false"
      >
        <div class="fixed inset-0 bg-black/50 backdrop-blur-sm" @click="showModal = false" />
        <div
          v-motion
          :initial="{ opacity: 0, scale: 0.95 }"
          :enter="{ opacity: 1, scale: 1, transition: { type: 'spring', stiffness: 350, damping: 25 } }"
          class="relative w-full max-w-md rounded-2xl border border-border bg-card shadow-2xl p-6"
        >
          <h3 class="text-lg font-semibold mb-4">
            {{ editingIntegration ? 'Edit Integration' : 'Add Integration' }}
          </h3>

          <form class="space-y-4" @submit.prevent="onSubmit">
            <div class="space-y-1.5">
              <label class="text-sm font-medium text-foreground">Type</label>
              <select
                v-model="formState.type"
                :disabled="!!editingIntegration"
                class="w-full h-9 px-3 rounded-lg border border-input bg-input text-sm disabled:opacity-60"
              >
                <option value="sonarr">Sonarr</option>
                <option value="radarr">Radarr</option>
                <option value="lidarr">Lidarr</option>
                <option value="plex">Plex</option>
                <option value="tautulli">Tautulli</option>
                <option value="overseerr">Overseerr</option>
              </select>
            </div>

            <div class="space-y-1.5">
              <label class="text-sm font-medium text-foreground">Name</label>
              <input
                v-model="formState.name"
                type="text"
                :placeholder="namePlaceholder"
                class="w-full h-9 px-3 rounded-lg border border-input bg-input text-sm focus:outline-none focus:ring-2 focus-visible:ring-ring/50"
              />
            </div>

            <div class="space-y-1.5">
              <label class="text-sm font-medium text-foreground">URL</label>
              <input
                v-model="formState.url"
                type="text"
                placeholder="http://localhost:8989"
                class="w-full h-9 px-3 rounded-lg border border-input bg-input text-sm focus:outline-none focus:ring-2 focus-visible:ring-ring/50"
              />
            </div>

            <div class="space-y-1.5">
              <label class="text-sm font-medium text-foreground">
                {{ formState.type === 'plex' ? 'Plex Token' : 'API Key' }}
              </label>
              <input
                v-model="formState.apiKey"
                type="password"
                placeholder="Enter API key or token"
                class="w-full h-9 px-3 rounded-lg border border-input bg-input text-sm focus:outline-none focus:ring-2 focus-visible:ring-ring/50"
              />
            </div>

            <!-- Error -->
            <div v-if="formError" class="rounded-lg bg-red-50 dark:bg-red-500/10 border border-red-200 dark:border-red-500/20 px-3 py-2 text-sm text-red-600 dark:text-red-400">
              {{ formError }}
            </div>
          </form>

          <!-- Footer -->
          <div class="flex items-center justify-between mt-6">
            <button
              class="h-9 px-4 rounded-lg border border-input text-sm font-medium hover:bg-accent transition-colors"
              @click="testFormConnection"
            >
              Test Connection
            </button>
            <div class="flex gap-2">
              <button
                class="h-9 px-4 rounded-lg text-sm font-medium text-muted-foreground hover:text-foreground transition-colors"
                @click="showModal = false"
              >
                Cancel
              </button>
              <button
                class="h-9 px-4 rounded-lg bg-primary hover:bg-primary/90 text-white text-sm font-medium shadow-sm transition-colors"
                :disabled="saving"
                @click="onSubmit"
              >
                {{ editingIntegration ? 'Save' : 'Add' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import {
  PlusIcon, HardDriveIcon, LoaderCircleIcon,
  LinkIcon, KeyIcon, ClockIcon, AlertTriangleIcon,
  TvIcon, FilmIcon, PlayCircleIcon, ServerIcon,
  DatabaseIcon, MonitorIcon, ActivityIcon,
  InboxIcon, MusicIcon
} from 'lucide-vue-next'
import { formatRelativeTime } from '~/utils/format'

const api = useApi()
const { timezone: displayTimezone, clockFormat: displayClockFormat, setTimezone, setClockFormat } = useDisplayPrefs()
const { theme: currentTheme, setTheme, themes: themeList } = useTheme()

const loading = ref(true)
const integrations = ref<any[]>([])
const showModal = ref(false)
const editingIntegration = ref<any>(null)
const saving = ref(false)
const formError = ref('')
const { addToast } = useToast()

// Data Management state
const retentionDays = ref(30)
const savingRetention = ref(false)
const retentionSaved = ref(false)

const formState = reactive({
  type: 'sonarr',
  name: '',
  url: '',
  apiKey: ''
})

const namePlaceholder = computed(() => {
  const defaults: Record<string, string> = {
    sonarr: 'My Sonarr', radarr: 'My Radarr', lidarr: 'My Lidarr',
    plex: 'My Plex', tautulli: 'My Tautulli', overseerr: 'My Overseerr'
  }
  return defaults[formState.type] || 'Integration Name'
})

function typeIcon(type: string) {
  switch (type) {
    case 'sonarr': return TvIcon
    case 'radarr': return FilmIcon
    case 'lidarr': return MusicIcon
    case 'plex': return PlayCircleIcon
    case 'tautulli': return ActivityIcon
    case 'overseerr': return InboxIcon
    default: return ServerIcon
  }
}

function typeColor(type: string) {
  switch (type) {
    case 'sonarr': return 'bg-sky-500'
    case 'radarr': return 'bg-amber-500'
    case 'lidarr': return 'bg-green-500'
    case 'plex': return 'bg-orange-500'
    case 'tautulli': return 'bg-teal-500'
    case 'overseerr': return 'bg-indigo-500'
    default: return 'bg-muted-foreground'
  }
}

function typeTextColor(type: string) {
  switch (type) {
    case 'sonarr': return 'text-sky-500'
    case 'radarr': return 'text-amber-500'
    case 'lidarr': return 'text-green-500'
    case 'plex': return 'text-orange-500'
    case 'tautulli': return 'text-teal-500'
    case 'overseerr': return 'text-indigo-500'
    default: return 'text-muted-foreground'
  }
}

async function fetchIntegrations() {
  loading.value = true
  try {
    integrations.value = await api('/api/v1/integrations') as any[]
  } catch (e) {
    console.error('Failed to fetch integrations:', e)
    addToast('Failed to load integrations', 'error')
  } finally {
    loading.value = false
  }
}

function openAddModal() {
  editingIntegration.value = null
  formState.type = 'sonarr'
  formState.name = ''
  formState.url = ''
  formState.apiKey = ''
  formError.value = ''
  showModal.value = true
}

function openEditModal(integration: any) {
  editingIntegration.value = integration
  formState.type = integration.type
  formState.name = integration.name
  formState.url = integration.url
  formState.apiKey = ''
  formError.value = ''
  showModal.value = true
}

async function onSubmit() {
  saving.value = true
  formError.value = ''
  try {
    if (editingIntegration.value) {
      await api(`/api/v1/integrations/${editingIntegration.value.id}`, {
        method: 'PUT',
        body: { ...formState, enabled: editingIntegration.value.enabled }
      })
    } else {
      await api('/api/v1/integrations', {
        method: 'POST',
        body: formState
      })
    }
    showModal.value = false
    addToast('Integration saved', 'success')
    await fetchIntegrations()
  } catch (e: any) {
    formError.value = e?.data?.error || 'Failed to save integration'
    addToast(formError.value, 'error')
  } finally {
    saving.value = false
  }
}

async function deleteIntegration(integration: any) {
  if (!confirm(`Delete ${integration.name}? This cannot be undone.`)) return
  try {
    await api(`/api/v1/integrations/${integration.id}`, { method: 'DELETE' })
    addToast('Integration deleted', 'success')
    await fetchIntegrations()
  } catch (e) {
    console.error('Failed to delete:', e)
    addToast('Failed to delete integration', 'error')
  }
}

async function testConnection(integration: any) {
  try {
    const result = await api('/api/v1/integrations/test', {
      method: 'POST',
      body: { type: integration.type, url: integration.url, apiKey: integration.apiKey }
    }) as any
    addToast(result.success ? 'Connection successful!' : `Connection failed: ${result.error}`, result.success ? 'success' : 'error')
  } catch {
    addToast('Connection test failed', 'error')
  }
}

async function testFormConnection() {
  try {
    const result = await api('/api/v1/integrations/test', {
      method: 'POST',
      body: { type: formState.type, url: formState.url, apiKey: formState.apiKey }
    }) as any
    if (result.success) {
      formError.value = ''
      addToast('Connection successful!', 'success')
    } else {
      formError.value = result.error || 'Connection failed'
      addToast(formError.value, 'error')
    }
  } catch {
    formError.value = 'Connection test failed'
    addToast('Connection test failed', 'error')
  }
}

// ─── Data Management (Retention) ─────────────────────────────────────────────
async function fetchPreferences() {
  try {
    const prefs = await api('/api/v1/preferences') as any
    if (prefs?.auditLogRetentionDays !== undefined) {
      retentionDays.value = prefs.auditLogRetentionDays
    }
  } catch (e) {
    console.error('Failed to fetch preferences:', e)
  }
}

async function saveRetention() {
  savingRetention.value = true
  retentionSaved.value = false
  try {
    // Fetch current prefs, merge retention, and save
    const currentPrefs = await api('/api/v1/preferences') as any
    await api('/api/v1/preferences', {
      method: 'PUT',
      body: { ...currentPrefs, auditLogRetentionDays: retentionDays.value }
    })
    retentionSaved.value = true
    addToast('Settings saved', 'success')
    setTimeout(() => { retentionSaved.value = false }, 3000)
  } catch (e) {
    console.error('Failed to save retention:', e)
    addToast('Failed to save retention settings', 'error')
  } finally {
    savingRetention.value = false
  }
}

onMounted(() => {
  fetchIntegrations()
  fetchPreferences()
})
</script>
