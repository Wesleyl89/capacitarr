package notifications

import (
	"fmt"
	"log/slog"

	"capacitarr/internal/db"
	"capacitarr/internal/events"
)

// NotificationProvider abstracts the notification channel service to avoid
// import cycles between the notifications and services packages.
type NotificationProvider interface {
	ListEnabled() ([]db.NotificationConfig, error)
	CreateInApp(title, message, severity, eventType string) error
}

// EventBusSubscriber subscribes to the event bus and dispatches notifications
// to configured channels. It replaces the inline Dispatch() calls scattered
// throughout the codebase.
type EventBusSubscriber struct {
	provider NotificationProvider
	bus      *events.EventBus
	ch       chan events.Event
	done     chan struct{}
}

// NewEventBusSubscriber creates a new notification subscriber.
func NewEventBusSubscriber(provider NotificationProvider, bus *events.EventBus) *EventBusSubscriber {
	return &EventBusSubscriber{
		provider: provider,
		bus:      bus,
		done:     make(chan struct{}),
	}
}

// Start subscribes to the event bus and begins dispatching notifications.
func (s *EventBusSubscriber) Start() {
	s.ch = s.bus.Subscribe()
	go s.run()
}

// Stop unsubscribes from the bus and waits for the background goroutine.
func (s *EventBusSubscriber) Stop() {
	s.bus.Unsubscribe(s.ch)
	<-s.done
}

func (s *EventBusSubscriber) run() {
	defer close(s.done)
	for event := range s.ch {
		s.handle(event)
	}
}

// handle maps typed events to notification events and dispatches them.
func (s *EventBusSubscriber) handle(event events.Event) {
	notifEvent, notifType := mapToNotification(event)
	if notifType == "" {
		return // This event type doesn't trigger notifications
	}

	configs, err := s.provider.ListEnabled()
	if err != nil {
		slog.Error("Failed to query notification configs", "component", "notifications", "error", err)
		return
	}

	for _, cfg := range configs {
		if !subscribes(cfg, notifType) {
			continue
		}

		c := cfg
		ne := notifEvent
		go func() {
			var sendErr error
			switch c.Type {
			case "discord":
				sendErr = SendDiscord(c.WebhookURL, ne)
			case "slack":
				sendErr = SendSlack(c.WebhookURL, ne)
			case "inapp":
				severity := SeverityForEvent(ne.Type)
				sendErr = s.provider.CreateInApp(ne.Title, ne.Message, severity, ne.Type)
			default:
				slog.Warn("Unknown notification channel type", "component", "notifications", "type", c.Type)
				return
			}

			if sendErr != nil {
				slog.Error("Failed to send notification",
					"component", "notifications",
					"channel", c.Name,
					"type", c.Type,
					"event", ne.Type,
					"error", sendErr,
				)
				s.bus.Publish(events.NotificationDeliveryFailedEvent{
					ChannelID:   c.ID,
					ChannelType: c.Type,
					Name:        c.Name,
					Error:       sendErr.Error(),
				})
			} else {
				slog.Debug("Notification sent via event bus subscriber",
					"component", "notifications",
					"channel", c.Name,
					"type", c.Type,
					"event", ne.Type,
				)
				s.bus.Publish(events.NotificationSentEvent{
					ChannelID:   c.ID,
					ChannelType: c.Type,
					Name:        c.Name,
					TriggerType: ne.Type,
				})
			}
		}()
	}
}

// mapToNotification converts a typed event bus event to a NotificationEvent.
// Returns empty notifType if the event doesn't trigger notifications.
func mapToNotification(event events.Event) (NotificationEvent, string) {
	switch e := event.(type) {
	// Threshold events
	case events.ThresholdChangedEvent:
		return NotificationEvent{
			Type:    EventThresholdBreach,
			Title:   "Threshold Changed",
			Message: e.EventMessage(),
			Fields: map[string]string{
				"Mount":     e.MountPath,
				"Threshold": fmt.Sprintf("%.0f%%", e.ThresholdPct),
				"Target":    fmt.Sprintf("%.0f%%", e.TargetPct),
			},
		}, EventThresholdBreach

	// Engine events
	case events.EngineCompleteEvent:
		return NotificationEvent{
			Type:    EventEngineComplete,
			Title:   "Engine Run Complete",
			Message: e.EventMessage(),
			Fields: map[string]string{
				"Evaluated": fmt.Sprintf("%d", e.Evaluated),
				"Flagged":   fmt.Sprintf("%d", e.Flagged),
				"Duration":  fmt.Sprintf("%dms", e.DurationMs),
			},
		}, EventEngineComplete

	case events.EngineErrorEvent:
		return NotificationEvent{
			Type:    EventEngineError,
			Title:   "Engine Error",
			Message: e.EventMessage(),
			Fields: map[string]string{
				"Error": e.Error,
			},
		}, EventEngineError

	// Deletion events
	case events.DeletionSuccessEvent:
		return NotificationEvent{
			Type:    EventDeletionExecuted,
			Title:   "Deletion Executed",
			Message: e.EventMessage(),
			Fields: map[string]string{
				"Media":  e.MediaName,
				"Action": "Deleted",
				"Size":   fmt.Sprintf("%d bytes", e.SizeBytes),
			},
		}, EventDeletionExecuted

	case events.DeletionDryRunEvent:
		return NotificationEvent{
			Type:    EventDeletionExecuted,
			Title:   "Deletion Executed (Dry-Run)",
			Message: e.EventMessage(),
			Fields: map[string]string{
				"Media":  e.MediaName,
				"Action": "Dry-Run",
				"Size":   fmt.Sprintf("%d bytes", e.SizeBytes),
			},
		}, EventDeletionExecuted

	case events.DeletionFailedEvent:
		return NotificationEvent{
			Type:    EventEngineError,
			Title:   "Deletion Failed",
			Message: e.EventMessage(),
			Fields: map[string]string{
				"Media": e.MediaName,
				"Error": e.Error,
			},
		}, EventEngineError

	default:
		return NotificationEvent{}, ""
	}
}
