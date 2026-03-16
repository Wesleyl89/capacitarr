<template>
  <div
    v-motion
    :initial="{ opacity: 0, y: 8 }"
    :enter="{ opacity: 1, y: 0 }"
    data-slot="dashboard-empty-state"
    class="rounded-xl border-2 border-dashed border-border p-12 text-center mb-6"
  >
    <!-- Setup CTA: no integrations configured -->
    <template v-if="integrations.length === 0">
      <PlusCircleIcon class="w-12 h-12 text-muted-foreground/40 mx-auto mb-4" />
      <h3 class="text-muted-foreground font-medium mb-1.5">
        {{ $t('dashboard.emptySetup.title') }}
      </h3>
      <p class="text-sm text-muted-foreground/70 mb-4 max-w-md mx-auto">
        {{ $t('dashboard.emptySetup.description') }}
      </p>
      <NuxtLink to="/settings">
        <UiButton>
          <SettingsIcon class="w-4 h-4" />
          {{ $t('dashboard.emptySetup.action') }}
        </UiButton>
      </NuxtLink>
    </template>

    <!-- Waiting for poll: integrations exist but no disk groups yet -->
    <template v-else>
      <LoaderCircleIcon class="w-12 h-12 text-muted-foreground/40 mx-auto mb-4 animate-spin" />
      <h3 class="text-muted-foreground font-medium mb-1.5">
        {{ $t('dashboard.emptyWait.title') }}
      </h3>
      <p class="text-sm text-muted-foreground/70 mb-4 max-w-md mx-auto">
        {{ $t('dashboard.emptyWait.description', { minutes: pollMinutes }) }}
      </p>
      <UiButton variant="outline" :disabled="runNowLoading" @click="triggerRunNow">
        <LoaderCircleIcon v-if="runNowLoading" class="w-4 h-4 animate-spin" />
        <PlayIcon v-else class="w-4 h-4" />
        {{ $t('dashboard.emptyWait.runNow') }}
      </UiButton>
    </template>
  </div>
</template>

<script setup lang="ts">
import { LoaderCircleIcon, PlusCircleIcon, PlayIcon, SettingsIcon } from 'lucide-vue-next';
import type { IntegrationConfig } from '~/types/api';

const props = defineProps<{
  integrations: IntegrationConfig[];
  pollIntervalSeconds: number;
}>();

const { runNowLoading, triggerRunNow } = useEngineControl();

const pollMinutes = computed(() => Math.max(1, Math.ceil(props.pollIntervalSeconds / 60)));
</script>
