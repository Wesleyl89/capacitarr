package notifications

// SeverityForEvent maps event types to in-app notification severity levels.
func SeverityForEvent(eventType string) string {
	switch eventType {
	case EventThresholdBreach:
		return "warning"
	case EventDeletionExecuted:
		return "info"
	case EventEngineError:
		return "error"
	case EventEngineComplete:
		return "success"
	default:
		return "info"
	}
}
