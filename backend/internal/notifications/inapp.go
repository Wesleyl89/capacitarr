package notifications

import "fmt"

// InAppSender implements Sender for in-app (database) notification delivery.
// It uses the InAppCreator interface to persist notification records.
type InAppSender struct {
	creator InAppCreator
}

// NewInAppSender creates a new InAppSender with the given persistence layer.
func NewInAppSender(creator InAppCreator) *InAppSender {
	return &InAppSender{creator: creator}
}

// SendDigest delivers a cycle digest notification as an in-app record.
// The webhookURL parameter is ignored for in-app notifications.
func (s *InAppSender) SendDigest(_ string, digest CycleDigest) error {
	title := digestTitle(digest)
	message := digestDescription(digest)

	// Flatten markdown bold for in-app display
	severity := digestSeverity(digest)
	eventType := "cycle_digest"

	return s.creator.CreateInApp(title, message, severity, eventType)
}

// SendAlert delivers an immediate alert notification as an in-app record.
// The webhookURL parameter is ignored for in-app notifications.
func (s *InAppSender) SendAlert(_ string, alert Alert) error {
	severity := alertSeverity(alert.Type)
	eventType := fmt.Sprintf("alert_%s", alert.Type)

	return s.creator.CreateInApp(alert.Title, alert.Message, severity, eventType)
}
