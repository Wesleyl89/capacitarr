package notifications

import (
	"log/slog"

	"capacitarr/internal/db"
)

// Event types used to match against NotificationConfig subscription booleans.
const (
	EventThresholdBreach  = "threshold_breach"
	EventDeletionExecuted = "deletion_executed"
	EventEngineError      = "engine_error"
	EventEngineComplete   = "engine_complete"
)

// Emoji constants shared by notification formatters (Discord, Slack).
const (
	emojiRed    = "🔴"
	emojiYellow = "🟡"
	emojiGreen  = "🟢"
	emojiInfo   = "ℹ️"
)

// NotificationEvent represents something that happened that channels may want to know about.
type NotificationEvent struct {
	Type    string            // One of the Event* constants
	Title   string            // Short title
	Message string            // Detailed message
	Fields  map[string]string // Key-value pairs for rich formatting (e.g. "Disk Group" → "/mnt/media")
}

// subscribes returns true if the given config is subscribed to the event type.
func subscribes(cfg db.NotificationConfig, eventType string) bool {
	switch eventType {
	case EventThresholdBreach:
		return cfg.OnThresholdBreach
	case EventDeletionExecuted:
		return cfg.OnDeletionExecuted
	case EventEngineError:
		return cfg.OnEngineError
	case EventEngineComplete:
		return cfg.OnEngineComplete
	default:
		return false
	}
}

// Dispatch sends a notification event to all enabled channels that subscribe to this event type.
// Each send runs in a goroutine so notifications never block the caller.
// Errors are logged but not returned — notifications are best-effort.
func Dispatch(event NotificationEvent) {
	var configs []db.NotificationConfig
	if err := db.DB.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		slog.Error("Failed to query notification configs", "component", "notifications", "error", err)
		return
	}

	for _, cfg := range configs {
		if !subscribes(cfg, event.Type) {
			continue
		}

		// Capture loop variable for goroutine
		c := cfg
		go func() {
			var err error
			switch c.Type {
			case "discord":
				err = SendDiscord(c.WebhookURL, event)
			case "slack":
				err = SendSlack(c.WebhookURL, event)
			case "inapp":
				err = SendInApp(event)
			default:
				slog.Warn("Unknown notification channel type", "component", "notifications", "type", c.Type)
				return
			}

			if err != nil {
				slog.Error("Failed to send notification",
					"component", "notifications",
					"channel", c.Name,
					"type", c.Type,
					"event", event.Type,
					"error", err,
				)
			} else {
				slog.Debug("Notification sent",
					"component", "notifications",
					"channel", c.Name,
					"type", c.Type,
					"event", event.Type,
				)
			}
		}()
	}
}
