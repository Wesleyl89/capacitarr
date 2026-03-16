<template>
  <div
    v-if="failingIntegrations.length > 0 && !dismissed"
    v-motion
    :initial="{ opacity: 0, y: -8 }"
    :enter="{ opacity: 1, y: 0, transition: { type: 'spring', stiffness: 300, damping: 26 } }"
    data-slot="integration-error-banner"
    class="mb-6"
  >
    <UiCollapsible v-model:open="detailsOpen">
      <UiAlert variant="destructive">
        <AlertCircleIcon class="w-4 h-4" />
        <UiAlertTitle class="flex items-center justify-between">
          <UiCollapsibleTrigger as-child>
            <button class="flex items-center gap-1.5 hover:underline underline-offset-2 text-left">
              <component
                :is="detailsOpen ? ChevronUpIcon : ChevronDownIcon"
                class="w-3.5 h-3.5 shrink-0"
              />
              {{ $t('dashboard.errorBanner.title', { count: failingIntegrations.length }) }}
            </button>
          </UiCollapsibleTrigger>
          <button
            class="text-destructive/60 hover:text-destructive transition-colors ml-2 shrink-0"
            :title="$t('common.close')"
            @click="dismissed = true"
          >
            <XIcon class="w-4 h-4" />
          </button>
        </UiAlertTitle>
        <UiAlertDescription>
          {{ $t('dashboard.errorBanner.description') }}
        </UiAlertDescription>

        <UiCollapsibleContent>
          <div class="mt-3 space-y-2">
            <div
              v-for="integration in failingIntegrations"
              :key="integration.id"
              class="rounded-md bg-destructive/10 px-3 py-2 text-sm"
            >
              <div class="flex items-center gap-2">
                <span class="font-medium">{{ integration.name }}</span>
                <UiBadge variant="outline" class="text-[10px]">{{ integration.type }}</UiBadge>
              </div>
              <p class="text-xs text-destructive/80 mt-0.5 break-words">
                {{ integration.lastError }}
              </p>
            </div>
          </div>
          <NuxtLink
            to="/settings"
            class="inline-flex items-center gap-1 text-xs text-destructive hover:text-destructive/80 font-medium mt-3 transition-colors"
          >
            <SettingsIcon class="w-3 h-3" />
            {{ $t('dashboard.errorBanner.action') }}
          </NuxtLink>
        </UiCollapsibleContent>
      </UiAlert>
    </UiCollapsible>
  </div>
</template>

<script setup lang="ts">
import {
  AlertCircleIcon,
  ChevronDownIcon,
  ChevronUpIcon,
  SettingsIcon,
  XIcon,
} from 'lucide-vue-next';
import type { IntegrationConfig } from '~/types/api';

const props = defineProps<{
  integrations: IntegrationConfig[];
}>();

const failingIntegrations = computed(() => props.integrations.filter((i) => i.lastError));

const detailsOpen = ref(false);
const dismissed = ref(false);
</script>
