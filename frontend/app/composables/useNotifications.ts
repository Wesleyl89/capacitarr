import type { InAppNotification } from '~/types/api';

// Module-level flag: SSE handlers registered once globally.
let _notifSseRegistered = false;

/**
 * Composable for in-app notification management.
 * Uses SSE events as the primary trigger for refreshing the unread count
 * (notification_sent, notification_delivery_failed, engine_complete, engine_error,
 * deletion_success). Falls back to a single fetch on start for initial hydration.
 */
export function useNotifications() {
  const api = useApi();
  const { on: sseOn } = useEventStream();
  const unreadCount = useState<number>('notif-unread', () => 0);
  const notifications = useState<InAppNotification[]>('notif-list', () => []);
  const loading = useState<boolean>('notif-loading', () => false);

  /** Fetch unread count from the API */
  async function fetchUnreadCount() {
    try {
      const res = (await api('/api/v1/notifications/unread-count')) as { count: number };
      unreadCount.value = res.count;
    } catch (e) {
      // Silently fail — badge just stays at last known value
      console.warn('[useNotifications] fetchUnreadCount failed:', e);
    }
  }

  /** Fetch recent in-app notifications (newest first, max 20) */
  async function fetchNotifications() {
    loading.value = true;
    try {
      notifications.value = (await api('/api/v1/notifications')) as InAppNotification[];
    } catch (e) {
      // Silently fail — list stays at last known state
      console.warn('[useNotifications] fetchNotifications failed:', e);
    } finally {
      loading.value = false;
    }
  }

  /** Mark a single notification as read */
  async function markAsRead(id: number) {
    try {
      await api(`/api/v1/notifications/${id}/read`, { method: 'PUT' });
      // Update local state
      const notif = notifications.value.find((n) => n.id === id);
      if (notif) notif.read = true;
      unreadCount.value = Math.max(0, unreadCount.value - 1);
    } catch (e) {
      // Silently fail — mark-as-read is non-critical
      console.warn('[useNotifications] markAsRead failed:', e);
    }
  }

  /** Mark all notifications as read */
  async function markAllAsRead() {
    try {
      await api('/api/v1/notifications/read-all', { method: 'PUT' });
      notifications.value.forEach((n) => {
        n.read = true;
      });
      unreadCount.value = 0;
    } catch (e) {
      // Silently fail — mark-all-as-read is non-critical
      console.warn('[useNotifications] markAllAsRead failed:', e);
    }
  }

  /** Delete all in-app notifications */
  async function clearAll() {
    try {
      await api('/api/v1/notifications', { method: 'DELETE' });
      notifications.value = [];
      unreadCount.value = 0;
    } catch (e) {
      // Silently fail — clearAll is non-critical
      console.warn('[useNotifications] clearAll failed:', e);
    }
  }

  // ---------------------------------------------------------------------------
  // SSE subscriptions — registered once globally to refresh unread count
  // when the backend creates in-app notifications via the event bus.
  // ---------------------------------------------------------------------------
  if (import.meta.client && !_notifSseRegistered) {
    _notifSseRegistered = true;

    const refreshCount = () => fetchUnreadCount();

    // Events that trigger in-app notification creation on the backend
    // (see notifications/subscriber.go — engine_complete, engine_error, deletion)
    sseOn('engine_complete', refreshCount);
    sseOn('engine_error', refreshCount);
    sseOn('deletion_success', refreshCount);
    sseOn('deletion_failed', refreshCount);
    sseOn('notification_sent', refreshCount);
    sseOn('notification_delivery_failed', refreshCount);
  }

  /**
   * Fetch initial unread count. Called once on mount by the Navbar.
   * Ongoing updates arrive via SSE event subscriptions above.
   */
  function start() {
    fetchUnreadCount();
  }

  return {
    unreadCount,
    notifications,
    loading,
    fetchUnreadCount,
    fetchNotifications,
    markAsRead,
    markAllAsRead,
    clearAll,
    start,
  };
}
