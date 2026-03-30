import type { SunsetQueueItem } from '~/types/api';

// Module-level flag — SSE handlers are registered once globally,
// not per component instance. Same pattern as useSnoozedItems.ts.
let _sseRegistered = false;

export function useSunsetQueue() {
  const api = useApi();
  const { on } = useEventStream();
  const { addToast } = useToast();
  const { runCompletionCounter } = useEngineControl();

  const sunsetItems = useState<SunsetQueueItem[]>('sunsetItems', () => []);
  const loading = useState<boolean>('sunsetLoading', () => false);

  async function fetchSunsetItems() {
    loading.value = true;
    try {
      const data = (await api('/api/v1/sunset-queue')) as SunsetQueueItem[];
      sunsetItems.value = data ?? [];
    } catch (err) {
      console.warn('[useSunsetQueue] fetchSunsetItems failed:', err);
    } finally {
      loading.value = false;
    }
  }

  async function cancelItem(id: number) {
    // Optimistic removal
    const prev = [...sunsetItems.value];
    sunsetItems.value = sunsetItems.value.filter((item) => item.id !== id);

    try {
      await api(`/api/v1/sunset-queue/${id}`, { method: 'DELETE' });
      addToast('Sunset item cancelled', 'success');
    } catch {
      sunsetItems.value = prev;
      addToast('Failed to cancel sunset item', 'error');
    }
  }

  async function rescheduleItem(id: number, deletionDate: string) {
    try {
      const result = (await api(`/api/v1/sunset-queue/${id}`, {
        method: 'PATCH',
        body: { deletionDate },
      })) as { id: number; mediaName: string; deletionDate: string; daysRemaining: number };

      // Update local state
      const idx = sunsetItems.value.findIndex((item) => item.id === id);
      if (idx >= 0) {
        const existing = sunsetItems.value[idx];
        sunsetItems.value[idx] = Object.assign({}, existing, {
          deletionDate: result.deletionDate,
          daysRemaining: result.daysRemaining,
        });
      }
      addToast(`Rescheduled — leaving in ${result.daysRemaining} days`, 'success');
    } catch {
      addToast('Failed to reschedule sunset item', 'error');
    }
  }

  async function clearAll() {
    try {
      const result = (await api('/api/v1/sunset-queue/clear', { method: 'POST' })) as {
        cancelled: number;
      };
      sunsetItems.value = [];
      addToast(`Cleared ${result.cancelled} sunset item(s)`, 'success');
    } catch {
      addToast('Failed to clear sunset queue', 'error');
    }
  }

  // Auto-refresh on engine run completion
  watch(runCompletionCounter, () => {
    fetchSunsetItems();
  });

  // SSE subscriptions — register once globally
  if (import.meta.client && !_sseRegistered) {
    _sseRegistered = true;

    on('sunset_created', () => fetchSunsetItems());
    on('sunset_cancelled', () => fetchSunsetItems());
    on('sunset_expired', () => fetchSunsetItems());
    on('sunset_rescheduled', () => fetchSunsetItems());
    on('sunset_escalated', () => fetchSunsetItems());
    on('sunset_saved', () => fetchSunsetItems());
    on('sunset_saved_cleaned', () => fetchSunsetItems());
  }

  return {
    sunsetItems: readonly(sunsetItems),
    loading: readonly(loading),
    fetchSunsetItems,
    cancelItem,
    rescheduleItem,
    clearAll,
  };
}
