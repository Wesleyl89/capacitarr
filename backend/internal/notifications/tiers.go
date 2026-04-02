package notifications

// NotificationTier represents a notification verbosity level.
// Lower values are more severe (sent to more channels).
type NotificationTier int

// Notification tier constants define the verbosity levels for notification channels.
const (
	TierOff       NotificationTier = 0
	TierCritical  NotificationTier = 1
	TierImportant NotificationTier = 2
	TierNormal    NotificationTier = 3
	TierVerbose   NotificationTier = 4
)

// EventTier maps each notification event kind to its default tier.
// Mode-specific events (sunset escalation, sunset misconfigured) are
// mapped to their generic equivalents by the dispatch service — modes
// are a rendering concern, not a notification routing concern.
var EventTier = map[string]NotificationTier{
	// Critical — something is wrong or needs immediate attention
	"error":              TierCritical,
	"threshold_breached": TierCritical,
	"integration_down":   TierCritical,

	// Important — user action may be relevant
	"mode_changed":      TierImportant,
	"approval_activity": TierImportant,

	// Normal — informational
	"cycle_digest":     TierNormal,
	"update_available": TierNormal,
	"server_started":   TierNormal,

	// Verbose — everything
	"dry_run_digest":       TierVerbose,
	"integration_recovery": TierVerbose,
}

// ParseLevel converts a database string to a NotificationTier.
// Returns TierNormal for unrecognized values.
func ParseLevel(s string) NotificationTier {
	switch s {
	case "off":
		return TierOff
	case "critical":
		return TierCritical
	case "important":
		return TierImportant
	case "normal":
		return TierNormal
	case "verbose":
		return TierVerbose
	default:
		return TierNormal
	}
}

// ShouldNotify determines whether a notification should be sent to a channel.
// If the per-event override is set, it takes precedence. Otherwise, the event
// is sent if its tier is at or below (more severe than) the channel's level.
func ShouldNotify(channelLevel NotificationTier, eventKind string, override *bool) bool {
	if override != nil {
		return *override
	}
	if channelLevel == TierOff {
		return false
	}
	eventTier, ok := EventTier[eventKind]
	if !ok {
		// Unknown event kinds default to Normal tier
		eventTier = TierNormal
	}
	return eventTier <= channelLevel
}

// TierDescription returns a human-readable summary of what a tier includes.
// Used by the API to populate the frontend dropdown descriptions.
func TierDescription(tier NotificationTier) string {
	switch tier {
	case TierCritical:
		return "Errors, threshold breaches, and integration failures"
	case TierImportant:
		return "Critical events plus mode changes and review activity"
	case TierNormal:
		return "Cycle digests, update notices, and all important events"
	case TierVerbose:
		return "Everything including simulation digests and integration recovery"
	case TierOff:
		return "No notifications"
	default:
		return "No notifications"
	}
}
