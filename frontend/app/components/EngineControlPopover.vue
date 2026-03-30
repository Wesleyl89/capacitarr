<template>
  <UiPopover>
    <UiPopoverTrigger as-child>
      <UiButton
        variant="ghost"
        size="icon"
        aria-label="Engine controls"
        :class="isRunning ? 'text-primary animate-pulse' : ''"
      >
        <component
          :is="isRunning ? LoaderCircleIcon : PlayIcon"
          :class="['w-5 h-5', isRunning ? 'animate-spin' : '']"
        />
      </UiButton>
    </UiPopoverTrigger>
    <UiPopoverContent align="center" :side="props.side" class="w-72">
      <div v-motion v-bind="scaleIn" class="space-y-4">
        <!-- Header -->
        <div class="flex items-center justify-between">
          <h4 class="font-semibold text-sm">
            {{ $t('engine.control') }}
          </h4>
        </div>

        <!-- Stats -->
        <div class="grid grid-cols-2 gap-2 text-xs">
          <div class="rounded-lg bg-muted px-2.5 py-1.5">
            <div class="text-muted-foreground">
              {{ $t('engine.lastRun') }}
            </div>
            <div class="font-medium">
              <DateDisplay
                v-if="lastRunEpoch"
                :date="new Date(lastRunEpoch * 1000).toISOString()"
              />
              <span v-else>Never</span>
            </div>
          </div>
          <div class="rounded-lg bg-muted px-2.5 py-1.5">
            <div class="text-muted-foreground">
              {{ $t('engine.queue') }}
            </div>
            <div class="font-medium">{{ queueDepth }} items</div>
          </div>
          <div class="rounded-lg bg-muted px-2.5 py-1.5">
            <div class="text-muted-foreground">
              {{ $t('engine.evaluated') }}
            </div>
            <div class="font-medium">
              {{ lastRunEvaluated }}
            </div>
          </div>
          <div class="rounded-lg bg-muted px-2.5 py-1.5">
            <div class="text-muted-foreground">
              {{ $t('engine.candidates') }}
            </div>
            <div class="font-medium">
              {{ lastRunCandidates }}
            </div>
          </div>
        </div>

        <!-- Run Now -->
        <UiButton class="w-full" :disabled="runNowLoading" @click="triggerRunNow">
          <LoaderCircleIcon v-if="runNowLoading" class="w-4 h-4 animate-spin" />
          <PlayIcon v-else class="w-4 h-4" />
          {{ $t('engine.runNow') }}
        </UiButton>
      </div>
    </UiPopoverContent>
  </UiPopover>
</template>

<script setup lang="ts">
import { PlayIcon, LoaderCircleIcon } from 'lucide-vue-next';

const { scaleIn } = useMotionPresets();

const props = withDefaults(
  defineProps<{
    /** Which side the popover opens toward. Defaults to 'bottom'. */
    side?: 'top' | 'bottom' | 'left' | 'right';
  }>(),
  { side: 'bottom' },
);

const {
  lastRunEpoch,
  lastRunEvaluated,
  lastRunCandidates,
  queueDepth,
  isRunning,
  runNowLoading,
  fetchStats,
  triggerRunNow,
} = useEngineControl();

// Fetch stats on mount for initial hydration.
// Ongoing updates arrive via SSE (engine_start / engine_complete / engine_error).
onMounted(() => {
  fetchStats();
});
</script>
