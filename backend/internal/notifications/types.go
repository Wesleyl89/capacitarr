package notifications

import (
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
