// Package events provides a typed, in-process event bus with fan-out delivery
// to multiple subscribers via buffered channels.
package events

import (
	"log/slog"
	"sync"
)

// Event is the interface all typed events implement.
type Event interface {
	// EventType returns a machine-readable event type string (e.g. "engine_start").
	EventType() string
	// EventMessage returns a human-readable description of the event.
	EventMessage() string
}

// subscriberBufferSize is the capacity of each subscriber's buffered channel.
// Events are dropped (with a warning log) if a subscriber falls this far behind.
const subscriberBufferSize = 256

// EventBus provides typed publish/subscribe with fan-out delivery.
// Each subscriber receives every published event on its own buffered channel.
// The bus is safe for concurrent use.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
	closed      bool
}

// NewEventBus creates a new EventBus ready for use.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[chan Event]struct{}),
	}
}

// Publish sends an event to all current subscribers.
// If a subscriber's buffer is full, the event is dropped for that subscriber
// and a warning is logged. Publish never blocks.
func (b *EventBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			slog.Warn("Event bus subscriber buffer full, dropping event",
				"component", "events",
				"eventType", event.EventType(),
				"message", event.EventMessage(),
			)
		}
	}
}

// Subscribe creates a new subscriber channel that receives all published events.
// The returned channel is buffered with subscriberBufferSize capacity.
// The caller must eventually call Unsubscribe to release resources.
func (b *EventBus) Subscribe() chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, subscriberBufferSize)
	if !b.closed {
		b.subscribers[ch] = struct{}{}
	} else {
		close(ch)
	}
	return ch
}

// Unsubscribe removes a subscriber channel and closes it.
// It is safe to call Unsubscribe multiple times for the same channel.
func (b *EventBus) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.subscribers[ch]; ok {
		delete(b.subscribers, ch)
		close(ch)
	}
}

// Close shuts down the event bus: removes all subscribers and closes their channels.
// After Close, Publish is a no-op and Subscribe returns a closed channel.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true
	for ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, ch)
	}
}

// SubscriberCount returns the number of active subscribers. Useful for testing.
func (b *EventBus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
