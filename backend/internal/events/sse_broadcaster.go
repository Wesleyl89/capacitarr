package events

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
)

// SSEBroadcaster subscribes to an EventBus and fans out events to connected
// SSE (Server-Sent Events) clients. It supports event ID-based replay via
// the Last-Event-ID header.
type SSEBroadcaster struct {
	mu      sync.RWMutex
	clients map[*sseClient]struct{}
	bus     *EventBus
	ch      chan Event
	done    chan struct{}

	// Ring buffer for replay
	ringMu  sync.RWMutex
	ring    [ringBufferSize]ringEntry
	ringIdx int
	nextID  atomic.Int64
}

type sseClient struct {
	events chan []byte // Pre-formatted SSE message
	done   chan struct{}
}

type ringEntry struct {
	id      int64
	payload []byte // Pre-formatted SSE message
}

const (
	ringBufferSize    = 100
	clientBufferSize  = 64
	keepaliveInterval = 30 * time.Second
)

// NewSSEBroadcaster creates a new SSE broadcaster wired to the given event bus.
func NewSSEBroadcaster(bus *EventBus) *SSEBroadcaster {
	return &SSEBroadcaster{
		clients: make(map[*sseClient]struct{}),
		bus:     bus,
		done:    make(chan struct{}),
	}
}

// Start subscribes to the event bus and begins broadcasting.
func (b *SSEBroadcaster) Start() {
	b.ch = b.bus.Subscribe()
	go b.run()
}

// Stop unsubscribes from the bus and disconnects all clients.
func (b *SSEBroadcaster) Stop() {
	b.bus.Unsubscribe(b.ch)
	<-b.done

	b.mu.Lock()
	defer b.mu.Unlock()
	for client := range b.clients {
		close(client.done)
		delete(b.clients, client)
	}
}

func (b *SSEBroadcaster) run() {
	defer close(b.done)
	for event := range b.ch {
		b.broadcast(event)
	}
}

func (b *SSEBroadcaster) broadcast(event Event) {
	id := b.nextID.Add(1)

	// Marshal the concrete event struct first, then inject "message" into the
	// flat JSON object. We can't use Go struct embedding with an interface
	// because json.Marshal wraps the interface value in a named "Event" key
	// instead of flattening the fields, e.g. {"Event":{...},"message":"..."}.
	eventJSON, err := json.Marshal(event)
	if err != nil {
		slog.Error("Failed to marshal SSE event", "component", "sse", "error", err)
		return
	}

	// Merge "message" into the existing JSON object: strip the closing '}',
	// append the message field. This avoids a full unmarshal/re-marshal cycle.
	humanMsg := event.EventMessage()
	escapedMsg, _ := json.Marshal(humanMsg) //nolint:errcheck // string marshal can't fail
	mergedJSON := make([]byte, 0, len(eventJSON)+len(escapedMsg)+14)
	mergedJSON = append(mergedJSON, eventJSON[:len(eventJSON)-1]...) // strip trailing '}'
	mergedJSON = append(mergedJSON, `,"message":`...)
	mergedJSON = append(mergedJSON, escapedMsg...)
	mergedJSON = append(mergedJSON, '}')

	// Build SSE message with proper formatting
	msg := fmt.Appendf(nil, "id: %d\nevent: %s\ndata: ", id, event.EventType())
	msg = append(msg, mergedJSON...)
	msg = append(msg, '\n', '\n')

	// Store in ring buffer for replay
	b.ringMu.Lock()
	b.ring[b.ringIdx%ringBufferSize] = ringEntry{id: id, payload: msg}
	b.ringIdx++
	b.ringMu.Unlock()

	// Fan out to all connected clients
	b.mu.RLock()
	for client := range b.clients {
		select {
		case client.events <- msg:
		default:
			// Client too slow, skip this event
			slog.Warn("SSE client buffer full, dropping event", "component", "sse", "eventType", event.EventType())
		}
	}
	b.mu.RUnlock()
}

// HandleSSE is the Echo handler for GET /api/v1/events.
// It establishes an SSE connection and streams events to the client.
func (b *SSEBroadcaster) HandleSSE(c echo.Context) error {
	// Set SSE headers
	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
	w.WriteHeader(http.StatusOK)
	w.Flush()

	client := &sseClient{
		events: make(chan []byte, clientBufferSize),
		done:   make(chan struct{}),
	}

	// Register client
	b.mu.Lock()
	b.clients[client] = struct{}{}
	b.mu.Unlock()

	// Replay missed events if Last-Event-ID is provided
	lastEventID := c.Request().Header.Get("Last-Event-ID")
	if lastEventID != "" {
		b.replay(client, lastEventID)
	}

	// Stream events to the client
	ctx := c.Request().Context()
	keepalive := time.NewTicker(keepaliveInterval)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			b.removeClient(client)
			return nil
		case <-client.done:
			return nil
		case msg := <-client.events:
			if _, err := w.Write(msg); err != nil {
				b.removeClient(client)
				return nil
			}
			w.Flush()
		case <-keepalive.C:
			// Send a comment to keep the connection alive
			if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
				b.removeClient(client)
				return nil
			}
			w.Flush()
		}
	}
}

func (b *SSEBroadcaster) removeClient(client *sseClient) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, client)
}

func (b *SSEBroadcaster) replay(client *sseClient, lastEventID string) {
	var lastID int64
	if _, err := fmt.Sscanf(lastEventID, "%d", &lastID); err != nil {
		return // Invalid Last-Event-ID, skip replay
	}

	b.ringMu.RLock()
	defer b.ringMu.RUnlock()

	// Iterate the ring buffer in chronological order, starting from the oldest
	// entry. When the buffer has wrapped, the oldest entry is at ringIdx (the
	// next slot to be overwritten), so we start there and wrap around.
	for i := 0; i < ringBufferSize; i++ {
		idx := (b.ringIdx + i) % ringBufferSize
		entry := b.ring[idx]
		if entry.id > lastID && entry.payload != nil {
			select {
			case client.events <- entry.payload:
			default:
				// Client buffer full during replay, stop
				return
			}
		}
	}
}

// ClientCount returns the number of connected SSE clients. Useful for monitoring.
func (b *SSEBroadcaster) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}
