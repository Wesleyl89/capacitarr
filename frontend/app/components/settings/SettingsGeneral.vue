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
                patchPreference('snoozeDuration', 'engine', 'snoozeDurationHours', Number(v));
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
    <UiCardContent class="space-y-6">
      <!-- ── Countdown & Labels ───────────────────────────────────── -->
      <div class="space-y-4">
        <h4 class="text-sm font-medium text-foreground">
          {{ $t('settings.sunsetCountdownGroup') }}
        </h4>
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
                  patchPreference('sunsetDays', 'sunset', 'sunsetDays', Number(v));
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
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
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
              @change="patchPreference('sunsetLabel', 'sunset', 'sunsetLabel', sunsetLabel)"
            />
          </div>
          <div class="space-y-1.5">
            <div class="flex items-center gap-2">
              <UiLabel>{{ $t('settings.savedLabel') }}</UiLabel>
              <SaveIndicator :status="saveStatus.savedLabel ?? 'idle'" />
            </div>
            <p class="text-xs text-muted-foreground mb-1">
              {{ $t('settings.savedLabelDesc') }}
            </p>
            <UiInput
              :model-value="savedLabel"
              placeholder="capacitarr-saved"
              @update:model-value="
                (v: string | number) => {
                  savedLabel = String(v);
                }
              "
              @change="patchPreference('savedLabel', 'sunset', 'savedLabel', savedLabel)"
            />
          </div>
        </div>
      </div>

      <UiSeparator />

      <!-- ── Poster Overlays ──────────────────────────────────────── -->
      <div class="space-y-4">
        <div class="space-y-1">
          <h4 class="text-sm font-medium text-foreground">
            {{ $t('settings.posterOverlayGroup') }}
          </h4>
          <p class="text-xs text-muted-foreground">
            {{ $t('settings.posterOverlayGroupDesc') }}
          </p>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
          <div class="space-y-1.5">
            <div class="flex items-center gap-2">
              <UiLabel>{{ $t('settings.posterOverlay') }}</UiLabel>
              <SaveIndicator :status="saveStatus.posterOverlay ?? 'idle'" />
            </div>
            <p class="text-xs text-muted-foreground mb-1">
              {{ $t('settings.posterOverlayDesc') }}
            </p>
            <UiSwitch
              :model-value="posterOverlayEnabled"
              @update:model-value="
                (v: boolean) => {
                  posterOverlayEnabled = v;
                  patchPreference('posterOverlay', 'sunset', 'posterOverlayEnabled', v);
                }
              "
            />
          </div>
          <div class="space-y-3">
            <div class="space-y-1.5">
              <UiLabel>{{ $t('settings.refreshPosters') }}</UiLabel>
              <p class="text-xs text-muted-foreground mb-1">
                {{ $t('settings.refreshPostersDesc') }}
              </p>
              <UiButton
                variant="outline"
                size="sm"
                :disabled="refreshingPosters"
                @click="refreshAllPosters"
              >
                <LoaderCircleIcon
                  v-if="refreshingPosters"
                  class="w-3.5 h-3.5 mr-1.5 animate-spin"
                />
                {{ $t('settings.refreshPosters') }}
              </UiButton>
            </div>
            <div class="space-y-1.5">
              <UiLabel>{{ $t('settings.restoreAllPosters') }}</UiLabel>
              <p class="text-xs text-muted-foreground mb-1">
                {{ $t('settings.restoreAllPostersDesc') }}
              </p>
              <UiButton
                variant="destructive"
                size="sm"
                :disabled="restoringPosters"
                @click="confirmRestorePosters"
              >
                <LoaderCircleIcon v-if="restoringPosters" class="w-3.5 h-3.5 mr-1.5 animate-spin" />
                {{ $t('settings.restoreAllPosters') }}
              </UiButton>
            </div>
          </div>
        </div>
      </div>

      <UiSeparator />

      <!-- ── Score Protection ─────────────────────────────────────── -->
      <div class="space-y-4">
        <div class="space-y-1">
          <h4 class="text-sm font-medium text-foreground">
            {{ $t('settings.scoreProtectionGroup') }}
          </h4>
          <p class="text-xs text-muted-foreground">
            {{ $t('settings.scoreProtectionGroupDesc') }}
          </p>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
          <div class="space-y-1.5">
            <div class="flex items-center gap-2">
              <UiLabel>{{ $t('settings.dailyScoreCheck') }}</UiLabel>
              <SaveIndicator :status="saveStatus.sunsetRescore ?? 'idle'" />
            </div>
            <p class="text-xs text-muted-foreground mb-1">
              {{ $t('settings.dailyScoreCheckDesc') }}
            </p>
            <UiSwitch
              :model-value="sunsetRescoreEnabled"
              @update:model-value="
                (v: boolean) => {
                  sunsetRescoreEnabled = v;
                  patchPreference('sunsetRescore', 'sunset', 'sunsetRescoreEnabled', v);
                }
              "
            />
          </div>
          <div class="space-y-1.5">
            <div class="flex items-center gap-2">
              <UiLabel>{{ $t('settings.savedDuration') }}</UiLabel>
              <SaveIndicator :status="saveStatus.savedDuration ?? 'idle'" />
            </div>
            <p class="text-xs text-muted-foreground mb-1">
              {{ $t('settings.savedDurationDesc') }}
            </p>
            <UiSelect
              :model-value="String(savedDurationDays)"
              @update:model-value="
                (v: AcceptableValue) => {
                  savedDurationDays = Number(v);
                  patchPreference('savedDuration', 'sunset', 'savedDurationDays', Number(v));
                }
              "
            >
              <UiSelectTrigger class="w-full">
                <UiSelectValue :placeholder="$t('settings.selectDuration')" />
              </UiSelectTrigger>
              <UiSelectContent>
                <UiSelectItem value="3">{{ $t('settings.nDays', { n: 3 }) }}</UiSelectItem>
                <UiSelectItem value="5">{{ $t('settings.nDays', { n: 5 }) }}</UiSelectItem>
                <UiSelectItem value="7">{{ $t('settings.nDays', { n: 7 }) }}</UiSelectItem>
                <UiSelectItem value="14">{{ $t('settings.nDays', { n: 14 }) }}</UiSelectItem>
                <UiSelectItem value="30">{{ $t('settings.nDays', { n: 30 }) }}</UiSelectItem>
              </UiSelectContent>
            </UiSelect>
          </div>
        </div>
      </div>
    </UiCardContent>
  </UiCard>

  <!-- Restore All Posters Confirmation Dialog -->
  <UiDialog
    :open="showRestorePostersDialog"
    @update:open="
      (val: boolean) => {
        showRestorePostersDialog = val;
      }
    "
  >
    <UiDialogContent class="max-w-md">
      <UiDialogHeader>
        <UiDialogTitle>{{ $t('settings.restoreAllPostersDialogTitle') }}</UiDialogTitle>
        <UiDialogDescription>
          {{ $t('settings.restoreAllPostersConfirm') }}
        </UiDialogDescription>
      </UiDialogHeader>
      <UiDialogFooter class="flex gap-2 justify-end">
        <UiButton variant="outline" @click="showRestorePostersDialog = false">
          {{ $t('common.cancel') }}
        </UiButton>
        <UiButton variant="destructive" :disabled="restoringPosters" @click="restoreAllPosters">
          <LoaderCircleIcon v-if="restoringPosters" class="w-3.5 h-3.5 mr-1.5 animate-spin" />
          {{ $t('settings.restoreAllPosters') }}
        </UiButton>
      </UiDialogFooter>
    </UiDialogContent>
  </UiDialog>
</template>

<script setup lang="ts">
import { MonitorIcon, CogIcon, LoaderCircleIcon } from 'lucide-vue-next';
import type { PreferenceSet } from '~/types/api';
import { TIEBREAKER_SIZE_DESC } from '~/constants';
import type { AcceptableValue } from 'reka-ui';
import SaveIndicator from '~/components/settings/SaveIndicator.vue';

const { t } = useI18n();
const api = useApi();
const {
  timezone: displayTimezone,
  clockFormat: displayClockFormat,
  showExactDates,
  setTimezone,
  setClockFormat,
  setShowExactDates,
} = useDisplayPrefs();
const { saveStatus, initFields, patchPreference } = useAutoSave();

initFields([
  'tiebreaker',
  'snoozeDuration',
  'posterOverlay',
  'sunsetLabel',
  'sunsetDays',
  'sunsetRescore',
  'savedDuration',
  'savedLabel',
]);
const { addToast } = useToast();

// Engine behavior state
const engineTiebreakerMethod = ref<string>(TIEBREAKER_SIZE_DESC);
const snoozeDurationHours = ref(24);
const posterOverlayEnabled = ref(true);
const sunsetLabel = ref('capacitarr-sunset');
const sunsetDays = ref(30);
const sunsetRescoreEnabled = ref(true);
const savedDurationDays = ref(7);
const savedLabel = ref('capacitarr-saved');

// Poster action loading states
const refreshingPosters = ref(false);
const restoringPosters = ref(false);
const showRestorePostersDialog = ref(false);

// Watch tiebreaker — immediate save on select change
watch(engineTiebreakerMethod, (newVal, oldVal) => {
  if (oldVal !== undefined && newVal !== oldVal) {
    patchPreference('tiebreaker', 'engine', 'tiebreakerMethod', newVal);
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
    if (prefs?.sunsetRescoreEnabled !== undefined) {
      sunsetRescoreEnabled.value = prefs.sunsetRescoreEnabled;
    }
    if (prefs?.savedDurationDays !== undefined) {
      savedDurationDays.value = prefs.savedDurationDays;
    }
    if (prefs?.savedLabel) {
      savedLabel.value = prefs.savedLabel;
    }
  } catch (err) {
    console.warn('[SettingsGeneral] fetchPreferences failed:', err);
  }
}

async function refreshAllPosters() {
  refreshingPosters.value = true;
  try {
    const result = (await api('/api/v1/sunset-queue/refresh-posters', {
      method: 'POST',
    })) as { updated: number };
    addToast(t('settings.refreshPostersSuccess', { count: result.updated }), 'success');
  } catch {
    addToast(t('settings.refreshPostersError'), 'error');
  } finally {
    refreshingPosters.value = false;
  }
}

function confirmRestorePosters() {
  showRestorePostersDialog.value = true;
}

async function restoreAllPosters() {
  restoringPosters.value = true;
  try {
    const result = (await api('/api/v1/sunset-queue/restore-posters', {
      method: 'POST',
    })) as { restored: number };
    addToast(t('settings.restorePostersSuccess', { count: result.restored }), 'success');
  } catch {
    addToast(t('settings.restorePostersError'), 'error');
  } finally {
    restoringPosters.value = false;
    showRestorePostersDialog.value = false;
  }
}

onMounted(() => {
  fetchPreferences();
});
</script>
