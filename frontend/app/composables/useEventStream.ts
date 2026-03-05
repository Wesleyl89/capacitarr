/**
 * useEventStream — SSE composable for real-time event streaming.
 *
 * Connects to GET /api/v1/events and dispatches typed events to registered handlers.
 * Features:
 * - Auto-reconnect with exponential backoff
 * - Last-Event-ID replay on reconnection
 * - Typed event handlers
 */
export function useEventStream() {
  const connected = ref(false)
  const eventSource = ref<EventSource | null>(null)
  const handlers = new Map<string, Set<(data: unknown) => void>>()
  const lastEventId = ref<string>('')

  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectAttempts = 0
  const maxReconnectDelay = 30_000 // 30 seconds

  function connect() {
    if (eventSource.value) {
      return // Already connected
    }

    const url = '/api/v1/events'

    // EventSource doesn't allow custom headers, so we rely on cookies for auth.
    // The Last-Event-ID is sent automatically by the browser on reconnection.
    const es = new EventSource(url)

    es.onopen = () => {
      connected.value = true
      reconnectAttempts = 0
    }

    es.onerror = () => {
      connected.value = false
      es.close()
      eventSource.value = null
      scheduleReconnect()
    }

    // Listen for all event types by handling the generic 'message' event
    // and also specific named events
    es.onmessage = (event: MessageEvent) => {
      handleEvent('message', event)
    }

    eventSource.value = es
  }

  function disconnect() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }

    if (eventSource.value) {
      eventSource.value.close()
      eventSource.value = null
    }

    connected.value = false
    reconnectAttempts = 0
  }

  function scheduleReconnect() {
    if (reconnectTimer) return

    const delay = Math.min(
      1000 * Math.pow(2, reconnectAttempts),
      maxReconnectDelay,
    )
    reconnectAttempts++

    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      connect()
    }, delay)
  }

  function on(eventType: string, handler: (data: unknown) => void) {
    if (!handlers.has(eventType)) {
      handlers.set(eventType, new Set())
    }
    handlers.get(eventType)!.add(handler)

    // Register a named event listener on the EventSource
    if (eventSource.value) {
      registerEventListener(eventSource.value, eventType)
    }
  }

  function off(eventType: string, handler: (data: unknown) => void) {
    const set = handlers.get(eventType)
    if (set) {
      set.delete(handler)
      if (set.size === 0) {
        handlers.delete(eventType)
      }
    }
  }

  function handleEvent(eventType: string, event: MessageEvent) {
    if (event.lastEventId) {
      lastEventId.value = event.lastEventId
    }

    let data: unknown = event.data
    try {
      data = JSON.parse(event.data as string)
    }
    catch {
      // Not JSON, use raw string
    }

    const set = handlers.get(eventType)
    if (set) {
      for (const handler of set) {
        handler(data)
      }
    }
  }

  function registerEventListener(es: EventSource, eventType: string) {
    es.addEventListener(eventType, ((event: Event) => {
      handleEvent(eventType, event as MessageEvent)
    }) as EventListener)
  }

  // Re-register event listeners when reconnecting
  watch(eventSource, (es) => {
    if (es) {
      for (const eventType of handlers.keys()) {
        registerEventListener(es, eventType)
      }
    }
  })

  // Auto cleanup on unmount
  onUnmounted(() => {
    disconnect()
  })

  return {
    connected: readonly(connected),
    lastEventId: readonly(lastEventId),
    connect,
    disconnect,
    on,
    off,
  }
}
