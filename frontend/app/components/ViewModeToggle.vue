<script setup lang="ts">
import { LayoutGrid, List } from 'lucide-vue-next';

const { viewMode, setViewMode } = useDisplayPrefs();
const { t } = useI18n();
</script>

<template>
  <div class="flex items-center gap-0.5 rounded-md border bg-muted/30 p-0.5">
    <UiButton
      variant="ghost"
      size="sm"
      class="h-7 w-7 p-0"
      :class="{ 'bg-background shadow-sm': viewMode === 'list' }"
      :title="t('common.viewList')"
      :aria-label="t('common.viewList')"
      :aria-pressed="viewMode === 'list'"
      @click="setViewMode('list')"
    >
      <List class="w-3.5 h-3.5" />
    </UiButton>
    <UiButton
      variant="ghost"
      size="sm"
      class="h-7 w-7 p-0"
      :class="{ 'bg-background shadow-sm': viewMode === 'grid' }"
      :title="t('common.viewGrid')"
      :aria-label="t('common.viewGrid')"
      :aria-pressed="viewMode === 'grid'"
      @click="setViewMode('grid')"
    >
      <LayoutGrid class="w-3.5 h-3.5" />
    </UiButton>
  </div>
</template>
