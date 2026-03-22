<template>
  <div class="space-y-4">
    <!-- Preferences diff -->
    <UiCollapsible v-if="preview.preferences" v-model:open="openSections.preferences">
      <UiCollapsibleTrigger
        class="flex w-full items-center justify-between rounded-lg border p-4 hover:bg-muted/50"
      >
        <div class="flex items-center gap-2">
          <component :is="SettingsIcon" class="w-4 h-4 text-muted-foreground" />
          <span class="font-medium text-sm">{{ $t('settings.sectionPreferences') }}</span>
        </div>
        <UiBadge :variant="actionBadgeVariant(preview.preferences.action)">
          {{ preview.preferences.action }}
        </UiBadge>
      </UiCollapsibleTrigger>
      <UiCollapsibleContent class="mt-2">
        <div v-if="preview.preferences.changes?.length" class="rounded-lg border p-3 space-y-2">
          <div
            v-for="change in preview.preferences.changes"
            :key="change.field"
            class="flex items-center gap-2 text-sm"
          >
            <span class="font-mono text-xs text-muted-foreground min-w-[160px]">
              {{ change.field }}
            </span>
            <span class="text-red-500 line-through">{{ change.oldValue }}</span>
            <component :is="ArrowRightIcon" class="w-3 h-3 text-muted-foreground" />
            <span class="text-green-500">{{ change.newValue }}</span>
          </div>
        </div>
        <p v-else class="text-xs text-muted-foreground p-3">
          {{ $t('settings.importDiffNoChanges') }}
        </p>
      </UiCollapsibleContent>
    </UiCollapsible>

    <!-- Integrations diff -->
    <UiCollapsible v-if="preview.integrations?.length" v-model:open="openSections.integrations">
      <UiCollapsibleTrigger
        class="flex w-full items-center justify-between rounded-lg border p-4 hover:bg-muted/50"
      >
        <div class="flex items-center gap-2">
          <component :is="PlugIcon" class="w-4 h-4 text-muted-foreground" />
          <span class="font-medium text-sm">{{ $t('settings.sectionIntegrations') }}</span>
          <span class="text-xs text-muted-foreground">
            ({{ summarize(preview.integrations) }})
          </span>
        </div>
        <component :is="ChevronDownIcon" class="w-4 h-4 text-muted-foreground" />
      </UiCollapsibleTrigger>
      <UiCollapsibleContent class="mt-2 space-y-2">
        <div
          v-for="item in preview.integrations"
          :key="item.name"
          class="rounded-lg border p-3 flex items-start justify-between gap-2"
        >
          <div>
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium">{{ item.name }}</span>
              <span class="text-xs text-muted-foreground">({{ item.type }})</span>
            </div>
            <div v-if="item.changes?.length" class="mt-1 space-y-1">
              <div
                v-for="change in item.changes"
                :key="change.field"
                class="flex items-center gap-2 text-xs"
              >
                <span class="font-mono text-muted-foreground">{{ change.field }}:</span>
                <span class="text-red-500 line-through">{{ change.oldValue }}</span>
                <component :is="ArrowRightIcon" class="w-3 h-3 text-muted-foreground" />
                <span class="text-green-500">{{ change.newValue }}</span>
              </div>
            </div>
          </div>
          <UiBadge :variant="actionBadgeVariant(item.action)">
            {{ item.action }}
          </UiBadge>
        </div>
      </UiCollapsibleContent>
    </UiCollapsible>

    <!-- Rules diff (with inline resolution) -->
    <UiCollapsible v-if="preview.rules?.length" v-model:open="openSections.rules">
      <UiCollapsibleTrigger
        class="flex w-full items-center justify-between rounded-lg border p-4 hover:bg-muted/50"
      >
        <div class="flex items-center gap-2">
          <component :is="FilterIcon" class="w-4 h-4 text-muted-foreground" />
          <span class="font-medium text-sm">{{ $t('settings.sectionRules') }}</span>
          <span class="text-xs text-muted-foreground"> ({{ preview.rules.length }}) </span>
        </div>
        <component :is="ChevronDownIcon" class="w-4 h-4 text-muted-foreground" />
      </UiCollapsibleTrigger>
      <UiCollapsibleContent class="mt-2 space-y-2">
        <div
          v-for="rule in preview.rules"
          :key="rule.index"
          class="rounded-lg border p-3 space-y-2"
          :class="{
            'border-green-500/30 bg-green-500/5': rule.resolution === 'matched',
            'border-yellow-500/30 bg-yellow-500/5': rule.resolution === 'type_fallback',
            'border-red-500/30 bg-red-500/5': rule.resolution === 'unmatched',
          }"
        >
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-2 text-sm">
              <span class="font-mono text-xs text-muted-foreground"> #{{ rule.index + 1 }} </span>
              <UiBadge variant="outline" class="text-xs">
                {{ rule.rule.field }}
              </UiBadge>
              <span class="text-muted-foreground">{{ rule.rule.operator }}</span>
              <span>{{ rule.rule.value }}</span>
              <UiBadge :variant="effectVariant(rule.rule.effect)" class="text-xs">
                {{ rule.rule.effect }}
              </UiBadge>
            </div>
            <UiBadge :variant="statusVariant(rule.resolution)" class="text-xs">
              {{ statusLabel(rule.resolution) }}
            </UiBadge>
          </div>

          <p
            v-if="rule.rule.integrationName || rule.rule.integrationType"
            class="text-xs text-muted-foreground"
          >
            {{
              $t('settings.importResolveOriginal', {
                name: rule.rule.integrationName || '?',
                type: rule.rule.integrationType || '?',
              })
            }}
          </p>

          <!-- Inline resolution controls for unmatched / type_fallback -->
          <div v-if="rule.resolution !== 'matched'" class="flex items-center gap-3 pt-1">
            <UiSelect
              :model-value="getOverrideValue(rule.index)"
              @update:model-value="(v) => setOverride(rule.index, String(v))"
            >
              <UiSelectTrigger class="w-[280px]">
                <UiSelectValue />
              </UiSelectTrigger>
              <UiSelectContent>
                <UiSelectItem
                  v-if="rule.matchedIntegrationId != null"
                  :value="String(rule.matchedIntegrationId)"
                >
                  {{ rule.matchedIntegrationName }}
                  <span class="text-muted-foreground ml-1">(auto-matched)</span>
                </UiSelectItem>
                <UiSelectItem
                  v-for="c in rule.candidates.filter(
                    (c: IntCandidate) => c.id !== rule.matchedIntegrationId,
                  )"
                  :key="c.id"
                  :value="String(c.id)"
                >
                  {{ c.name }}
                </UiSelectItem>
                <UiSelectItem value="skip">
                  {{ $t('settings.importResolveSkip') }}
                </UiSelectItem>
              </UiSelectContent>
            </UiSelect>
          </div>
        </div>
      </UiCollapsibleContent>
    </UiCollapsible>

    <!-- Notifications diff -->
    <UiCollapsible v-if="preview.notifications?.length" v-model:open="openSections.notifications">
      <UiCollapsibleTrigger
        class="flex w-full items-center justify-between rounded-lg border p-4 hover:bg-muted/50"
      >
        <div class="flex items-center gap-2">
          <component :is="BellIcon" class="w-4 h-4 text-muted-foreground" />
          <span class="font-medium text-sm">{{ $t('settings.sectionNotifications') }}</span>
          <span class="text-xs text-muted-foreground">
            ({{ summarize(preview.notifications) }})
          </span>
        </div>
        <component :is="ChevronDownIcon" class="w-4 h-4 text-muted-foreground" />
      </UiCollapsibleTrigger>
      <UiCollapsibleContent class="mt-2 space-y-2">
        <div
          v-for="item in preview.notifications"
          :key="item.name"
          class="rounded-lg border p-3 flex items-center justify-between gap-2"
        >
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium">{{ item.name }}</span>
            <span class="text-xs text-muted-foreground">({{ item.type }})</span>
          </div>
          <UiBadge :variant="actionBadgeVariant(item.action)">
            {{ item.action }}
          </UiBadge>
        </div>
      </UiCollapsibleContent>
    </UiCollapsible>

    <!-- Disk Groups diff -->
    <UiCollapsible v-if="preview.diskGroups?.length" v-model:open="openSections.diskGroups">
      <UiCollapsibleTrigger
        class="flex w-full items-center justify-between rounded-lg border p-4 hover:bg-muted/50"
      >
        <div class="flex items-center gap-2">
          <component :is="HardDriveIcon" class="w-4 h-4 text-muted-foreground" />
          <span class="font-medium text-sm">{{ $t('settings.sectionDiskGroups') }}</span>
          <span class="text-xs text-muted-foreground"> ({{ summarize(preview.diskGroups) }}) </span>
        </div>
        <component :is="ChevronDownIcon" class="w-4 h-4 text-muted-foreground" />
      </UiCollapsibleTrigger>
      <UiCollapsibleContent class="mt-2 space-y-2">
        <div
          v-for="item in preview.diskGroups"
          :key="item.name"
          class="rounded-lg border p-3 flex items-start justify-between gap-2"
        >
          <div>
            <span class="text-sm font-medium font-mono">{{ item.name }}</span>
            <div v-if="item.changes?.length" class="mt-1 space-y-1">
              <div
                v-for="change in item.changes"
                :key="change.field"
                class="flex items-center gap-2 text-xs"
              >
                <span class="font-mono text-muted-foreground">{{ change.field }}:</span>
                <span class="text-red-500">{{ change.oldValue }}</span>
                <component :is="ArrowRightIcon" class="w-3 h-3 text-muted-foreground" />
                <span class="text-green-500">{{ change.newValue }}</span>
              </div>
            </div>
          </div>
          <UiBadge :variant="actionBadgeVariant(item.action)">
            {{ item.action }}
          </UiBadge>
        </div>
      </UiCollapsibleContent>
    </UiCollapsible>

    <!-- Deletion preview (sync mode) -->
    <UiAlert v-if="preview.deletions" variant="destructive">
      <component :is="TrashIcon" class="w-4 h-4" />
      <UiAlertTitle>{{ $t('settings.importDiffDeletionsTitle') }}</UiAlertTitle>
      <UiAlertDescription class="space-y-1 mt-2">
        <template v-if="preview.deletions.integrations?.length">
          <p class="font-medium text-xs">{{ $t('settings.sectionIntegrations') }}:</p>
          <p v-for="name in preview.deletions.integrations" :key="name" class="text-xs ml-2">
            • {{ name }}
          </p>
        </template>
        <template v-if="preview.deletions.notifications?.length">
          <p class="font-medium text-xs">{{ $t('settings.sectionNotifications') }}:</p>
          <p v-for="name in preview.deletions.notifications" :key="name" class="text-xs ml-2">
            • {{ name }}
          </p>
        </template>
        <template v-if="preview.deletions.diskGroups?.length">
          <p class="font-medium text-xs">{{ $t('settings.sectionDiskGroups') }}:</p>
          <p
            v-for="path in preview.deletions.diskGroups"
            :key="path"
            class="text-xs ml-2 font-mono"
          >
            • {{ path }}
          </p>
        </template>
        <template v-if="preview.deletions.rules?.length">
          <p class="font-medium text-xs">{{ $t('settings.sectionRules') }}:</p>
          <p v-for="desc in preview.deletions.rules" :key="desc" class="text-xs ml-2 font-mono">
            • {{ desc }}
          </p>
        </template>
      </UiAlertDescription>
    </UiAlert>
  </div>
</template>

<script setup lang="ts">
import {
  SettingsIcon,
  PlugIcon,
  FilterIcon,
  BellIcon,
  HardDriveIcon,
  ArrowRightIcon,
  ChevronDownIcon,
  TrashIcon,
} from 'lucide-vue-next';
import type { ImportPreview, ItemResolution, RuleOverride, IntCandidate } from '~/types/api';

const { t } = useI18n();

const props = defineProps<{
  preview: ImportPreview;
}>();

const emit = defineEmits<{
  'update:overrides': [overrides: RuleOverride[]];
}>();

// Track collapsible open state
const openSections = reactive({
  preferences: true,
  integrations: true,
  rules: true,
  notifications: true,
  diskGroups: true,
});

// Track rule override decisions
const decisions = reactive<Record<number, string>>({});

// Initialize decisions when preview changes
watch(
  () => props.preview.rules,
  (rules) => {
    if (!rules) return;
    for (const r of rules) {
      if (r.resolution === 'matched' && r.matchedIntegrationId != null) {
        decisions[r.index] = String(r.matchedIntegrationId);
      } else if (r.resolution === 'type_fallback' && r.matchedIntegrationId != null) {
        decisions[r.index] = String(r.matchedIntegrationId);
      } else {
        decisions[r.index] =
          r.candidates.length === 1 ? String(r.candidates[0]?.id ?? 'skip') : 'skip';
      }
    }
    emitOverrides();
  },
  { immediate: true },
);

function getOverrideValue(index: number): string {
  return decisions[index] ?? 'skip';
}

function setOverride(index: number, value: string) {
  decisions[index] = value;
  emitOverrides();
}

function emitOverrides() {
  if (!props.preview.rules) return;
  const overrides: RuleOverride[] = [];
  for (const r of props.preview.rules) {
    const decision = decisions[r.index];
    if (r.resolution === 'matched' && decision === String(r.matchedIntegrationId)) {
      continue;
    }
    if (decision === 'skip') {
      overrides.push({ index: r.index, integrationId: null, skip: true });
    } else {
      overrides.push({ index: r.index, integrationId: Number(decision), skip: false });
    }
  }
  emit('update:overrides', overrides);
}

function actionBadgeVariant(action: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (action === 'create') return 'default';
  if (action === 'update') return 'secondary';
  return 'outline';
}

function effectVariant(effect: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (effect.includes('keep')) return 'default';
  if (effect.includes('remove')) return 'destructive';
  return 'secondary';
}

function statusVariant(resolution: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (resolution === 'matched') return 'default';
  if (resolution === 'type_fallback') return 'secondary';
  return 'destructive';
}

function statusLabel(resolution: string): string {
  if (resolution === 'matched') return t('settings.importResolveMatched');
  if (resolution === 'type_fallback') return t('settings.importResolveFallback');
  return t('settings.importResolveUnmatched');
}

function summarize(items: ItemResolution[]): string {
  const creates = items.filter((i) => i.action === 'create').length;
  const updates = items.filter((i) => i.action === 'update').length;
  const unchanged = items.filter((i) => i.action === 'unchanged').length;
  const parts: string[] = [];
  if (creates > 0) parts.push(`${creates} new`);
  if (updates > 0) parts.push(`${updates} updated`);
  if (unchanged > 0) parts.push(`${unchanged} unchanged`);
  return parts.join(', ');
}
</script>
