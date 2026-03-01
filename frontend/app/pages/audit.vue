<template>
  <div>
    <div data-slot="page-header" class="mb-8 flex flex-col md:flex-row md:items-center justify-between gap-4">
      <div>
        <h1 class="text-3xl font-bold tracking-tight">Audit History</h1>
        <p class="text-muted-foreground mt-1.5">Historical log of all scoring engine decisions and space reclaimed.</p>
      </div>
      <UiButton variant="outline" @click="fetchLogs">
        <LoaderCircleIcon v-if="pending" class="w-4 h-4 animate-spin" />
        <RefreshCwIcon v-else class="w-4 h-4" />
        Refresh
      </UiButton>
    </div>

    <UiCard v-motion :initial="{ opacity: 0, y: 8 }" :enter="{ opacity: 1, y: 0, transition: { type: 'spring', stiffness: 260, damping: 24 } }" class="overflow-hidden">
      <div v-if="pending && logs.length === 0" class="p-4">
        <SkeletonTable :rows="8" :column-widths="['28%', '10%', '10%', '15%', '22%', '8%']" />
      </div>

      <div v-else-if="!pending && logs.length === 0" class="flex flex-col items-center justify-center py-20 text-muted-foreground">
        <ClockIcon class="w-10 h-10 mb-3" />
        <span class="text-sm font-medium">No history found</span>
      </div>

      <div v-else class="overflow-x-auto">
        <UiTable>
          <UiTableHeader>
            <UiTableRow>
              <UiTableHead class="w-8"></UiTableHead>
              <UiTableHead>Timestamp</UiTableHead>
              <UiTableHead>Title</UiTableHead>
              <UiTableHead>Type</UiTableHead>
              <UiTableHead>Result</UiTableHead>
              <UiTableHead>Score</UiTableHead>
              <UiTableHead class="text-right">Space</UiTableHead>
            </UiTableRow>
          </UiTableHeader>
          <UiTableBody>
            <template v-for="group in groupedLogs" :key="group.key">
              <UiTableRow class="cursor-pointer" @click="selectItem(group.entry); group.seasons.length > 0 && toggleGroup(group.key)">
                <UiTableCell class="w-8">
                  <button v-if="group.seasons.length > 0" class="text-muted-foreground hover:text-foreground transition-colors" @click.stop="toggleGroup(group.key)">
                    <ChevronRightIcon class="w-4 h-4 transition-transform duration-200" :class="{ 'rotate-90': expandedGroups.has(group.key) }" />
                  </button>
                </UiTableCell>
                <UiTableCell class="text-xs text-muted-foreground whitespace-nowrap">{{ formatTimestamp(group.entry.createdAt) }}</UiTableCell>
                <UiTableCell class="font-medium whitespace-nowrap">
                  {{ group.entry.mediaName }}
                  <span v-if="group.seasons.length > 0" class="ml-1.5 text-xs text-muted-foreground font-normal">({{ group.seasons.length }} season{{ group.seasons.length !== 1 ? 's' : '' }})</span>
                </UiTableCell>
                <UiTableCell>
                  <UiBadge variant="secondary" class="capitalize">{{ group.entry.mediaType }}</UiBadge>
                </UiTableCell>
                <UiTableCell>
                  <UiBadge :variant="actionBadgeVariant(group.entry.action)">{{ group.entry.action }}</UiBadge>
                </UiTableCell>
                <UiTableCell>
                  <ScoreBreakdown :reason="group.entry.reason" :score-details="group.entry.scoreDetails || ''" />
                </UiTableCell>
                <UiTableCell class="text-right font-mono text-xs tabular-nums">{{ (group.entry.sizeBytes / 1024 / 1024 / 1024).toFixed(2) }} GB</UiTableCell>
              </UiTableRow>
              <template v-if="expandedGroups.has(group.key)">
                <UiTableRow v-for="season in group.seasons" :key="season.id" class="bg-muted/30 cursor-pointer" @click.stop="selectItem(season)">
                  <UiTableCell class="w-8"></UiTableCell>
                  <UiTableCell class="text-xs text-muted-foreground whitespace-nowrap pl-8">{{ formatTimestamp(season.createdAt) }}</UiTableCell>
                  <UiTableCell class="text-muted-foreground whitespace-nowrap pl-8">
                    <span class="inline-flex items-center gap-1.5">
                      <span class="w-3 h-px bg-border inline-block"></span>
                      {{ extractSeasonLabel(season.mediaName) }}
                    </span>
                  </UiTableCell>
                  <UiTableCell>
                    <UiBadge variant="secondary" class="capitalize">{{ season.mediaType }}</UiBadge>
                  </UiTableCell>
                  <UiTableCell>
                    <UiBadge :variant="actionBadgeVariant(season.action)">{{ season.action }}</UiBadge>
                  </UiTableCell>
                  <UiTableCell>
                    <ScoreBreakdown :reason="season.reason" :score-details="season.scoreDetails || ''" size="sm" />
                  </UiTableCell>
                  <UiTableCell class="text-right font-mono text-xs tabular-nums text-muted-foreground">{{ (season.sizeBytes / 1024 / 1024 / 1024).toFixed(2) }} GB</UiTableCell>
                </UiTableRow>
              </template>
            </template>
          </UiTableBody>
        </UiTable>
      </div>

      <div v-if="total > limit" class="flex items-center justify-between px-5 py-3 border-t border-border">
        <span class="text-xs text-muted-foreground">{{ offset + 1 }}&ndash;{{ Math.min(offset + limit, total) }} of {{ total }}</span>
        <div class="flex gap-1">
          <UiButton variant="outline" size="sm" :disabled="page <= 1" @click="page--">Previous</UiButton>
          <UiButton variant="outline" size="sm" :disabled="offset + limit >= total" @click="page++">Next</UiButton>
        </div>
      </div>
    </UiCard>

    <ScoreDetailModal
      v-if="selectedItem"
      :visible="!!selectedItem"
      :media-name="selectedItem.mediaName"
      :media-type="selectedItem.mediaType"
      :score="selectedItem._score ?? 0"
      :score-details="selectedItem.scoreDetails || ''"
      :size-bytes="selectedItem.sizeBytes"
      :action="selectedItem.action"
      :created-at="selectedItem.createdAt"
      @close="selectedItem = null"
    />
  </div>
</template>

<script setup lang="ts">
import { RefreshCwIcon, LoaderCircleIcon, ClockIcon, ChevronRightIcon } from 'lucide-vue-next'

const api = useApi()
const { formatTimestamp } = useDisplayPrefs()

const logs = ref<any[]>([])
const total = ref(0)
const pending = ref(false)
const page = ref(1)
const limit = 50
const selectedItem = ref<any | null>(null)

function selectItem(entry: any) {
  const scoreMatch = entry.reason?.match(/^Score:\s*([\d.]+)/)
  const score = scoreMatch ? parseFloat(scoreMatch[1]) : 0
  selectedItem.value = { ...entry, _score: score }
}

const offset = computed(() => (page.value - 1) * limit)

async function fetchLogs() {
  pending.value = true
  try {
    const data = await api(`/api/v1/audit?limit=${limit}&offset=${offset.value}`) as any
    if (data?.data) {
      logs.value = data.data
      total.value = data.total
    }
  } catch (err) {
    console.error(err)
  } finally {
    pending.value = false
  }
}

watch(page, fetchLogs)
onMounted(fetchLogs)

// ─── Show/Season Grouping ─────────────────────────────────────────────────────
interface AuditGroup {
  key: string
  entry: any
  seasons: any[]
}

const groupedLogs = computed<AuditGroup[]>(() => {
  const groups: AuditGroup[] = []
  const showMap = new Map<string, number>()

  for (const log of logs.value) {
    if (log.mediaType === 'season' && log.mediaName.includes(' - Season ')) {
      const showName = log.mediaName.split(' - Season ')[0]
      const groupIdx = showMap.get(showName)
      if (groupIdx !== undefined) {
        groups[groupIdx].seasons.push(log)
        continue
      }
    }

    const key = `${log.id}-${log.mediaName}`
    if (log.mediaType === 'show') {
      showMap.set(log.mediaName, groups.length)
    }
    groups.push({ key, entry: log, seasons: [] })
  }

  return groups
})

// ─── Expand/Collapse state ────────────────────────────────────────────────────
const expandedGroups = ref(new Set<string>())

function toggleGroup(key: string) {
  const next = new Set(expandedGroups.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  expandedGroups.value = next
}

function extractSeasonLabel(mediaName: string): string {
  const parts = mediaName.split(' - Season ')
  return parts.length > 1 ? `Season ${parts[parts.length - 1]}` : mediaName
}

// ─── Action badge variant mapping ─────────────────────────────────────────────
function actionBadgeVariant(action: string): 'destructive' | 'outline' | 'secondary' | 'default' {
  if (action === 'Deleted') return 'destructive'
  if (action === 'Queued for Approval') return 'outline'
  if (action === 'Queued for Deletion') return 'outline'
  return 'default'
}
</script>
