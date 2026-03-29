<script setup lang="ts">
import { SunsetIcon, XCircleIcon } from 'lucide-vue-next';
import { formatBytes } from '~/utils/format';

const { t } = useI18n();
const { sunsetItems, fetchSunsetItems, cancelItem } = useSunsetQueue();

// Fetch on mount
onMounted(() => {
  fetchSunsetItems();
});

/**
 * Format days remaining as a human-readable countdown.
 * e.g. "30 days", "1 day", "Last day"
 */
function formatDaysRemaining(days: number): string {
  if (days <= 0) return t('sunset.lastDay');
  if (days === 1) return t('sunset.leavingTomorrow');
  return t('sunset.leavingInDays', { days });
}
</script>

<template>
  <UiCard
    v-if="sunsetItems.length > 0"
    v-motion
    :initial="{ opacity: 0, y: 12 }"
    :enter="{ opacity: 1, y: 0, transition: { type: 'spring', stiffness: 260, damping: 24 } }"
    class="mb-6"
  >
    <UiCardHeader>
      <div class="flex items-center justify-between">
        <div>
          <UiCardTitle class="flex items-center gap-2">
            <SunsetIcon class="w-4.5 h-4.5" />
            {{ t('sunset.title') }}
          </UiCardTitle>
          <UiCardDescription class="mt-1">
            {{ t('sunset.subtitle') }}
          </UiCardDescription>
        </div>
        <UiBadge variant="secondary" class="text-xs">
          {{ t('sunset.count', { count: sunsetItems.length }) }}
        </UiBadge>
      </div>
    </UiCardHeader>
    <UiCardContent>
      <div class="space-y-1.5">
        <div
          v-for="item in sunsetItems"
          :key="item.id"
          v-motion
          :initial="{ opacity: 0, x: -8 }"
          :enter="{
            opacity: 1,
            x: 0,
            transition: { type: 'spring', stiffness: 260, damping: 24 },
          }"
          :leave="{ opacity: 0, x: 8 }"
          class="flex items-center gap-3 rounded-lg border border-border bg-muted/30 px-3 py-2"
        >
          <div class="flex-1 min-w-0">
            <span class="text-sm font-medium truncate block">{{ item.mediaName }}</span>
            <span class="text-xs text-muted-foreground">
              {{ item.mediaType }} · {{ formatBytes(item.sizeBytes) }}
              <span class="ml-1 text-orange-500 dark:text-orange-400">
                · {{ formatDaysRemaining(item.daysRemaining) }}
              </span>
            </span>
          </div>
          <UiButton
            variant="ghost"
            size="sm"
            class="h-7 p-0 px-2 text-muted-foreground hover:text-foreground shrink-0"
            :aria-label="t('sunset.cancel')"
            :title="t('sunset.cancel')"
            @click="cancelItem(item.id)"
          >
            <XCircleIcon class="h-3.5 w-3.5 mr-1" />
            <span class="text-xs">{{ t('sunset.cancel') }}</span>
          </UiButton>
        </div>
      </div>
    </UiCardContent>
  </UiCard>
</template>
