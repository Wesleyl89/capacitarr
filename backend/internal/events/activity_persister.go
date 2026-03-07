package events

import (
	"encoding/json"
	"log/slog"
)

// ActivityWriter provides the ability to create activity event records.
// Implemented by SettingsService to keep DB access within the service layer.
type ActivityWriter interface {
	CreateActivity(eventType, message, metadata string) error
}

// ActivityPersister subscribes to an EventBus and writes every event
// as an ActivityEvent row via the ActivityWriter. It runs as a background goroutine.
type ActivityPersister struct {
	writer ActivityWriter
	bus    *EventBus
	ch     chan Event
	done   chan struct{}
}

// NewActivityPersister creates a new ActivityPersister wired to the given bus and writer.
func NewActivityPersister(writer ActivityWriter, bus *EventBus) *ActivityPersister {
	return &ActivityPersister{
		writer: writer,
		bus:    bus,
		done:   make(chan struct{}),
	}
}

// Start subscribes to the bus and begins persisting events in the background.
// Call Stop() to gracefully shut down.
func (p *ActivityPersister) Start() {
	p.ch = p.bus.Subscribe()
	go p.run()
}

// Stop unsubscribes from the bus and waits for the background goroutine to finish.
func (p *ActivityPersister) Stop() {
	p.bus.Unsubscribe(p.ch)
	<-p.done
}

func (p *ActivityPersister) run() {
	defer close(p.done)

	for event := range p.ch {
		p.persist(event)
	}
}

// persist writes a single event as an ActivityEvent row via the ActivityWriter.
func (p *ActivityPersister) persist(event Event) {
	metadata := ""
	if jsonBytes, err := json.Marshal(event); err == nil {
		metadata = string(jsonBytes)
	}

	if err := p.writer.CreateActivity(event.EventType(), event.EventMessage(), metadata); err != nil {
		slog.Error("Failed to persist activity event",
			"component", "events",
			"eventType", event.EventType(),
			"message", event.EventMessage(),
			"error", err,
		)
	}
}
