<template>
  <!-- Display Preferences Section -->
  <UiCard
    v-motion
    :initial="{ opacity: 0, y: 12 }"
    :enter="{
      opacity: 1,
      y: 0,
      transition: { type: 'spring', stiffness: 260, damping: 24, delay: 200 },
    }"
    class="overflow-hidden"
  >
    <UiCardHeader class="border-b border-border">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 rounded-lg bg-purple-500 flex items-center justify-center">
          <component :is="MonitorIcon" class="w-5 h-5 text-white" />
        </div>
        <div>
          <UiCardTitle class="text-base">
            {{ $t('settings.display') }}
          </UiCardTitle>
          <UiCardDescription>{{ $t('settings.displayDesc') }}</UiCardDescription>
        </div>
      </div>
    </UiCardHeader>
    <UiCardContent class="pt-5 space-y-5">
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-5">
        <!-- Timezone -->
        <div class="space-y-1.5">
          <UiLabel>{{ $t('settings.timezone') }}</UiLabel>
          <div class="flex gap-1">
            <UiButton
              :variant="displayTimezone === 'local' ? 'default' : 'outline'"
              size="sm"
              class="flex-1"
              @click="setTimezone('local')"
            >
              {{ $t('settings.timezoneLocal') }}
            </UiButton>
            <UiButton
              :variant="displayTimezone === 'UTC' ? 'default' : 'outline'"
              size="sm"
              class="flex-1"
              @click="setTimezone('UTC')"
            >
              {{ $t('settings.timezoneUTC') }}
            </UiButton>
          </div>
        </div>

        <!-- Clock Format -->
        <div class="space-y-1.5">
          <UiLabel>{{ $t('settings.clockFormat') }}</UiLabel>
          <div class="flex gap-1">
            <UiButton
              :variant="displayClockFormat === '12h' ? 'default' : 'outline'"
              size="sm"
              class="flex-1"
              @click="setClockFormat('12h')"
            >
              {{ $t('settings.clock12h') }}
            </UiButton>
            <UiButton
              :variant="displayClockFormat === '24h' ? 'default' : 'outline'"
              size="sm"
              class="flex-1"
              @click="setClockFormat('24h')"
            >
              {{ $t('settings.clock24h') }}
            </UiButton>
          </div>
        </div>

        <!-- Date Style -->
        <div class="space-y-1.5">
          <UiLabel>{{ $t('settings.exactDates') }}</UiLabel>
          <div class="flex gap-1">
            <UiButton
              :variant="!showExactDates ? 'default' : 'outline'"
              size="sm"
              class="flex-1"
              @click="setShowExactDates(false)"
            >
              {{ $t('settings.dateRelative') }}
            </UiButton>
            <UiButton
              :variant="showExactDates ? 'default' : 'outline'"
              size="sm"
              class="flex-1"
              @click="setShowExactDates(true)"
            >
              {{ $t('settings.dateExact') }}
            </UiButton>
          </div>
        </div>
      </div>
    </UiCardContent>
  </UiCard>

  <!-- Engine Behavior Section -->
  <UiCard
    v-motion
    :initial="{ opacity: 0, y: 12 }"
    :enter="{
      opacity: 1,
      y: 0,
      transition: { type: 'spring', stiffness: 260, damping: 24, delay: 300 },
    }"
    class="overflow-hidden"
  >
    <UiCardHeader class="border-b border-border">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 rounded-lg bg-primary flex items-center justify-center">
          <component :is="CogIcon" class="w-5 h-5 text-white" />
        </div>
        <div>
          <UiCardTitle class="text-base">
            {{ $t('settings.engineBehavior') }}
          </UiCardTitle>
          <UiCardDescription>{{ $t('settings.engineBehaviorDesc') }}</UiCardDescription>
        </div>
      </div>
    </UiCardHeader>
    <UiCardContent class="pt-5 space-y-6">
      <!-- Score Tiebreaker + Snooze Duration — side by side -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
        <!-- Score Tiebreaker -->
        <div class="space-y-1.5">
          <div class="flex items-center gap-2">
            <UiLabel>{{ $t('settings.scoreTiebreaker') }}</UiLabel>
            <SaveIndicator :status="saveStatus.tiebreaker ?? 'idle'" />
          </div>
          <p class="text-xs text-muted-foreground mb-1">
            When items have the same score, how should they be ordered?
          </p>
          <UiSelect v-model="engineTiebreakerMethod">
            <UiSelectTrigger class="w-full">
              <UiSelectValue placeholder="Select tiebreaker" />
            </UiSelectTrigger>
            <UiSelectContent>
              <UiSelectItem value="size_desc"> Largest first (free more space) </UiSelectItem>
              <UiSelectItem value="size_asc"> Smallest first </UiSelectItem>
              <UiSelectItem value="name_asc"> Alphabetical (A → Z) </UiSelectItem>
              <UiSelectItem value="oldest_first"> Oldest in library first </UiSelectItem>
              <UiSelectItem value="newest_first"> Newest in library first </UiSelectItem>
            </UiSelectContent>
          </UiSelect>
        </div>

        <!-- Snooze Duration -->
        <div class="space-y-1.5">
          <div class="flex items-center gap-2">
            <UiLabel>{{ $t('settings.snoozeDurationHours') }}</UiLabel>
            <SaveIndicator :status="saveStatus.snoozeDuration ?? 'idle'" />
          </div>
          <p class="text-xs text-muted-foreground mb-1">
            {{ $t('settings.snoozeDurationDesc') }}
          </p>
          <UiSelect
            :model-value="String(snoozeDurationHours)"
            @update:model-value="
              (v: AcceptableValue) => {
                snoozeDurationHours = Number(v);
                autoSavePreference('snoozeDuration', 'snoozeDurationHours', Number(v));
              }
            "
          >
            <UiSelectTrigger class="w-full">
              <UiSelectValue placeholder="Select duration" />
            </UiSelectTrigger>
            <UiSelectContent>
              <UiSelectItem value="1"> 1 hour </UiSelectItem>
              <UiSelectItem value="6"> 6 hours </UiSelectItem>
              <UiSelectItem value="12"> 12 hours </UiSelectItem>
              <UiSelectItem value="24"> 24 hours (1 day) </UiSelectItem>
              <UiSelectItem value="48"> 48 hours (2 days) </UiSelectItem>
              <UiSelectItem value="72"> 72 hours (3 days) </UiSelectItem>
              <UiSelectItem value="168"> 1 week </UiSelectItem>
              <UiSelectItem value="336"> 2 weeks </UiSelectItem>
              <UiSelectItem value="720"> 30 days </UiSelectItem>
            </UiSelectContent>
          </UiSelect>
        </div>
      </div>
    </UiCardContent>
  </UiCard>

  <!-- Sunset Settings -->
  <UiCard
    v-motion
    :initial="{ opacity: 0, y: 12 }"
    :enter="{
      opacity: 1,
      y: 0,
      transition: { type: 'spring', stiffness: 260, damping: 24, delay: 100 },
    }"
  >
    <UiCardHeader>
      <UiCardTitle>{{ $t('settings.sunsetSettings') }}</UiCardTitle>
      <UiCardDescription>{{ $t('settings.sunsetSettingsDesc') }}</UiCardDescription>
    </UiCardHeader>
    <UiCardContent class="space-y-5">
      <!-- Sunset Days -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
        <div class="space-y-1.5">
          <div class="flex items-center gap-2">
            <UiLabel>{{ $t('settings.sunsetDays') }}</UiLabel>
            <SaveIndicator :status="saveStatus.sunsetDays ?? 'idle'" />
          </div>
          <p class="text-xs text-muted-foreground mb-1">
            {{ $t('settings.sunsetDaysDesc') }}
          </p>
          <UiSelect
            :model-value="String(sunsetDays)"
            @update:model-value="
              (v: AcceptableValue) => {
                sunsetDays = Number(v);
                autoSavePreference('sunsetDays', 'sunsetDays', Number(v));
              }
            "
          >
            <UiSelectTrigger class="w-full">
              <UiSelectValue placeholder="Select duration" />
            </UiSelectTrigger>
            <UiSelectContent>
              <UiSelectItem value="7">7 days</UiSelectItem>
              <UiSelectItem value="14">14 days</UiSelectItem>
              <UiSelectItem value="21">21 days</UiSelectItem>
              <UiSelectItem value="30">30 days</UiSelectItem>
              <UiSelectItem value="45">45 days</UiSelectItem>
              <UiSelectItem value="60">60 days</UiSelectItem>
              <UiSelectItem value="90">90 days</UiSelectItem>
            </UiSelectContent>
          </UiSelect>
        </div>

        <!-- Sunset Label -->
        <div class="space-y-1.5">
          <div class="flex items-center gap-2">
            <UiLabel>{{ $t('settings.sunsetLabel') }}</UiLabel>
            <SaveIndicator :status="saveStatus.sunsetLabel ?? 'idle'" />
          </div>
          <p class="text-xs text-muted-foreground mb-1">
            {{ $t('settings.sunsetLabelDesc') }}
          </p>
          <UiInput
            :model-value="sunsetLabel"
            placeholder="capacitarr-sunset"
            @update:model-value="
              (v: string | number) => {
                sunsetLabel = String(v);
              }
            "
            @change="autoSavePreference('sunsetLabel', 'sunsetLabel', sunsetLabel)"
          />
        </div>
      </div>

      <!-- Poster Overlay Toggle + Restore -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
        <div class="space-y-1.5">
          <div class="flex items-center gap-2">
            <UiLabel>{{ $t('settings.posterOverlay') }}</UiLabel>
          </div>
          <p class="text-xs text-muted-foreground mb-1">
            {{ $t('settings.posterOverlayDesc') }}
          </p>
          <UiSwitch
            :checked="posterOverlayEnabled"
            @update:checked="
              (v: boolean) => {
                posterOverlayEnabled = v;
                autoSavePreference('posterOverlay', 'posterOverlayEnabled', v);
              }
            "
          />
        </div>
        <div class="space-y-1.5">
          <UiLabel>{{ $t('settings.refreshPosters') }}</UiLabel>
          <p class="text-xs text-muted-foreground mb-1">
            {{ $t('settings.refreshPostersDesc') }}
          </p>
          <UiButton variant="outline" size="sm" @click="refreshAllPosters">
            {{ $t('settings.refreshPosters') }}
          </UiButton>
        </div>
      </div>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
        <div class="space-y-1.5">
          <UiLabel>{{ $t('settings.restoreAllPosters') }}</UiLabel>
          <p class="text-xs text-muted-foreground mb-1">
            {{ $t('settings.restoreAllPostersDesc') }}
          </p>
          <UiButton variant="destructive" size="sm" @click="restoreAllPosters">
            {{ $t('settings.restoreAllPosters') }}
          </UiButton>
        </div>
      </div>
    </UiCardContent>
  </UiCard>
</template>

<script setup lang="ts">
import { MonitorIcon, CogIcon } from 'lucide-vue-next';
import type { PreferenceSet } from '~/types/api';
import { TIEBREAKER_SIZE_DESC } from '~/constants';
import type { AcceptableValue } from 'reka-ui';
import SaveIndicator from '~/components/settings/SaveIndicator.vue';

const api = useApi();
const {
  timezone: displayTimezone,
  clockFormat: displayClockFormat,
  showExactDates,
  setTimezone,
  setClockFormat,
  setShowExactDates,
} = useDisplayPrefs();
const { saveStatus, initFields, autoSavePreference } = useAutoSave();

initFields(['tiebreaker', 'snoozeDuration', 'posterOverlay', 'sunsetLabel', 'sunsetDays']);
const { addToast } = useToast();

// Engine behavior state
const engineTiebreakerMethod = ref<string>(TIEBREAKER_SIZE_DESC);
const snoozeDurationHours = ref(24);
const posterOverlayEnabled = ref(true);
const sunsetLabel = ref('capacitarr-sunset');
const sunsetDays = ref(30);

// Watch tiebreaker — immediate save on select change
watch(engineTiebreakerMethod, (newVal, oldVal) => {
  if (oldVal !== undefined && newVal !== oldVal) {
    autoSavePreference('tiebreaker', 'tiebreakerMethod', newVal);
  }
});

// ─── Fetch preferences on mount ──────────────────────────────────────────────
async function fetchPreferences() {
  try {
    const prefs = (await api('/api/v1/preferences')) as PreferenceSet;
    if (prefs?.tiebreakerMethod) {
      engineTiebreakerMethod.value = prefs.tiebreakerMethod;
    }
    if (prefs?.snoozeDurationHours !== undefined) {
      snoozeDurationHours.value = prefs.snoozeDurationHours;
    }
    if (prefs?.posterOverlayEnabled !== undefined) {
      posterOverlayEnabled.value = prefs.posterOverlayEnabled;
    }
    if (prefs?.sunsetLabel) {
      sunsetLabel.value = prefs.sunsetLabel;
    }
    if (prefs?.sunsetDays !== undefined) {
      sunsetDays.value = prefs.sunsetDays;
    }
  } catch (err) {
    console.warn('[SettingsGeneral] fetchPreferences failed:', err);
  }
}

async function refreshAllPosters() {
  try {
    const result = (await api('/api/v1/sunset-queue/refresh-posters', {
      method: 'POST',
    })) as { updated: number };
    addToast(`Refreshed ${result.updated} poster(s)`, 'success');
  } catch {
    addToast('Failed to refresh posters', 'error');
  }
}

async function restoreAllPosters() {
  try {
    const result = (await api('/api/v1/sunset-queue/restore-posters', {
      method: 'POST',
    })) as { restored: number };
    addToast(`Restored ${result.restored} poster(s)`, 'success');
  } catch {
    addToast('Failed to restore posters', 'error');
  }
}

onMounted(() => {
  fetchPreferences();
});
</script>
