package events

import "fmt"

// =============================================================================
// Engine Events
// =============================================================================

// EngineStartEvent is published when an engine evaluation cycle begins.
type EngineStartEvent struct {
	ExecutionMode string `json:"executionMode"`
}

func (e EngineStartEvent) EventType() string    { return "engine_start" }
func (e EngineStartEvent) EventMessage() string { return "Engine run started in " + e.ExecutionMode + " mode" }

// EngineCompleteEvent is published when an engine evaluation cycle finishes.
type EngineCompleteEvent struct {
	Evaluated     int    `json:"evaluated"`
	Flagged       int    `json:"flagged"`
	Deleted       int    `json:"deleted"`
	FreedBytes    int64  `json:"freedBytes"`
	DurationMs    int64  `json:"durationMs"`
	ExecutionMode string `json:"executionMode"`
}

func (e EngineCompleteEvent) EventType() string { return "engine_complete" }
func (e EngineCompleteEvent) EventMessage() string {
	return fmt.Sprintf("Engine run completed: evaluated %d, flagged %d", e.Evaluated, e.Flagged)
}

// EngineErrorEvent is published when an engine cycle fails.
type EngineErrorEvent struct {
	Error string `json:"error"`
}

func (e EngineErrorEvent) EventType() string    { return "engine_error" }
func (e EngineErrorEvent) EventMessage() string { return "Engine error: " + e.Error }

// EngineModeChangedEvent is published when the execution mode is changed.
type EngineModeChangedEvent struct {
	OldMode string `json:"oldMode"`
	NewMode string `json:"newMode"`
}

func (e EngineModeChangedEvent) EventType() string { return "engine_mode_changed" }
func (e EngineModeChangedEvent) EventMessage() string {
	return fmt.Sprintf("Execution mode changed from %s to %s", e.OldMode, e.NewMode)
}

// ManualRunTriggeredEvent is published when a user manually triggers an engine run.
type ManualRunTriggeredEvent struct{}

func (e ManualRunTriggeredEvent) EventType() string    { return "manual_run_triggered" }
func (e ManualRunTriggeredEvent) EventMessage() string { return "Manual engine run triggered" }

// =============================================================================
// Settings Events
// =============================================================================

// SettingsChangedEvent is published when preferences are saved.
type SettingsChangedEvent struct {
	Changes map[string]interface{} `json:"changes,omitempty"` // Fields that changed
}

func (e SettingsChangedEvent) EventType() string    { return "settings_changed" }
func (e SettingsChangedEvent) EventMessage() string { return "Settings updated" }

// ThresholdChangedEvent is published when disk group thresholds are updated.
type ThresholdChangedEvent struct {
	MountPath    string  `json:"mountPath"`
	ThresholdPct float64 `json:"thresholdPct"`
	TargetPct    float64 `json:"targetPct"`
}

func (e ThresholdChangedEvent) EventType() string { return "threshold_changed" }
func (e ThresholdChangedEvent) EventMessage() string {
	return fmt.Sprintf("Thresholds updated for %s: trigger at %.0f%%, target %.0f%%",
		e.MountPath, e.ThresholdPct, e.TargetPct)
}

// =============================================================================
// Auth Events
// =============================================================================

// LoginEvent is published on successful authentication.
type LoginEvent struct {
	Username string `json:"username"`
}

func (e LoginEvent) EventType() string    { return "login" }
func (e LoginEvent) EventMessage() string { return "User logged in: " + e.Username }

// PasswordChangedEvent is published when a user changes their password.
type PasswordChangedEvent struct {
	Username string `json:"username"`
}

func (e PasswordChangedEvent) EventType() string    { return "password_changed" }
func (e PasswordChangedEvent) EventMessage() string { return "Password changed for " + e.Username }

// UsernameChangedEvent is published when a user changes their username.
type UsernameChangedEvent struct {
	OldUsername string `json:"oldUsername"`
	NewUsername string `json:"newUsername"`
}

func (e UsernameChangedEvent) EventType() string { return "username_changed" }
func (e UsernameChangedEvent) EventMessage() string {
	return fmt.Sprintf("Username changed from %s to %s", e.OldUsername, e.NewUsername)
}

// APIKeyGeneratedEvent is published when an API key is generated.
type APIKeyGeneratedEvent struct {
	Username string `json:"username"`
	Hint     string `json:"hint"` // Last 4 chars
}

func (e APIKeyGeneratedEvent) EventType() string { return "api_key_generated" }
func (e APIKeyGeneratedEvent) EventMessage() string {
	return fmt.Sprintf("API key generated for %s (ending in %s)", e.Username, e.Hint)
}

// =============================================================================
// Integration Events
// =============================================================================

// IntegrationAddedEvent is published when a new integration is created.
type IntegrationAddedEvent struct {
	IntegrationID   uint   `json:"integrationId"`
	IntegrationType string `json:"integrationType"`
	Name            string `json:"name"`
}

func (e IntegrationAddedEvent) EventType() string { return "integration_added" }
func (e IntegrationAddedEvent) EventMessage() string {
	return fmt.Sprintf("Integration added: %s (%s)", e.Name, e.IntegrationType)
}

// IntegrationUpdatedEvent is published when an integration is modified.
type IntegrationUpdatedEvent struct {
	IntegrationID   uint   `json:"integrationId"`
	IntegrationType string `json:"integrationType"`
	Name            string `json:"name"`
}

func (e IntegrationUpdatedEvent) EventType() string { return "integration_updated" }
func (e IntegrationUpdatedEvent) EventMessage() string {
	return fmt.Sprintf("Integration updated: %s (%s)", e.Name, e.IntegrationType)
}

// IntegrationRemovedEvent is published when an integration is deleted.
type IntegrationRemovedEvent struct {
	IntegrationID   uint   `json:"integrationId"`
	IntegrationType string `json:"integrationType"`
	Name            string `json:"name"`
}

func (e IntegrationRemovedEvent) EventType() string { return "integration_removed" }
func (e IntegrationRemovedEvent) EventMessage() string {
	return fmt.Sprintf("Integration removed: %s (%s)", e.Name, e.IntegrationType)
}

// IntegrationTestEvent is published on a successful integration connection test.
type IntegrationTestEvent struct {
	IntegrationType string `json:"integrationType"`
	Name            string `json:"name"`
	URL             string `json:"url"`
}

func (e IntegrationTestEvent) EventType() string { return "integration_test" }
func (e IntegrationTestEvent) EventMessage() string {
	return fmt.Sprintf("Connection test succeeded: %s (%s)", e.Name, e.IntegrationType)
}

// IntegrationTestFailedEvent is published on a failed integration connection test.
type IntegrationTestFailedEvent struct {
	IntegrationType string `json:"integrationType"`
	Name            string `json:"name"`
	URL             string `json:"url"`
	Error           string `json:"error"`
}

func (e IntegrationTestFailedEvent) EventType() string { return "integration_test_failed" }
func (e IntegrationTestFailedEvent) EventMessage() string {
	return fmt.Sprintf("Connection test failed: %s (%s) — %s", e.Name, e.IntegrationType, e.Error)
}

// =============================================================================
// Approval Events
// =============================================================================

// ApprovalApprovedEvent is published when a queued item is approved for deletion.
type ApprovalApprovedEvent struct {
	EntryID   uint   `json:"entryId"`
	MediaName string `json:"mediaName"`
	MediaType string `json:"mediaType"`
	SizeBytes int64  `json:"sizeBytes"`
}

func (e ApprovalApprovedEvent) EventType() string { return "approval_approved" }
func (e ApprovalApprovedEvent) EventMessage() string {
	return fmt.Sprintf("Approved for deletion: %s", e.MediaName)
}

// ApprovalRejectedEvent is published when a queued item is rejected (snoozed).
type ApprovalRejectedEvent struct {
	EntryID        uint   `json:"entryId"`
	MediaName      string `json:"mediaName"`
	MediaType      string `json:"mediaType"`
	SnoozeDuration string `json:"snoozeDuration"` // e.g. "24h"
}

func (e ApprovalRejectedEvent) EventType() string { return "approval_rejected" }
func (e ApprovalRejectedEvent) EventMessage() string {
	return fmt.Sprintf("Rejected (snoozed): %s", e.MediaName)
}

// ApprovalUnsnoozedEvent is published when a snoozed item is manually unsnoozed.
type ApprovalUnsnoozedEvent struct {
	EntryID   uint   `json:"entryId"`
	MediaName string `json:"mediaName"`
	MediaType string `json:"mediaType"`
}

func (e ApprovalUnsnoozedEvent) EventType() string { return "approval_unsnoozed" }
func (e ApprovalUnsnoozedEvent) EventMessage() string {
	return fmt.Sprintf("Unsnoozed: %s", e.MediaName)
}

// ApprovalBulkUnsnoozedEvent is published when all snoozed items are cleared
// because disk usage dropped below threshold.
type ApprovalBulkUnsnoozedEvent struct {
	Count int `json:"count"`
}

func (e ApprovalBulkUnsnoozedEvent) EventType() string { return "approval_bulk_unsnoozed" }
func (e ApprovalBulkUnsnoozedEvent) EventMessage() string {
	return fmt.Sprintf("Bulk unsnoozed %d items (disk below threshold)", e.Count)
}

// ApprovalOrphansRecoveredEvent is published when orphaned approval items
// are requeued after a restart or integration reconnection.
type ApprovalOrphansRecoveredEvent struct {
	Count int `json:"count"`
}

func (e ApprovalOrphansRecoveredEvent) EventType() string { return "approval_orphans_recovered" }
func (e ApprovalOrphansRecoveredEvent) EventMessage() string {
	return fmt.Sprintf("Recovered %d orphaned approval items", e.Count)
}

// =============================================================================
// Deletion Events
// =============================================================================

// DeletionSuccessEvent is published when a media item is successfully deleted.
type DeletionSuccessEvent struct {
	MediaName     string `json:"mediaName"`
	MediaType     string `json:"mediaType"`
	SizeBytes     int64  `json:"sizeBytes"`
	IntegrationID uint   `json:"integrationId"`
}

func (e DeletionSuccessEvent) EventType() string { return "deletion_success" }
func (e DeletionSuccessEvent) EventMessage() string {
	sizeMB := float64(e.SizeBytes) / (1024 * 1024 * 1024)
	return fmt.Sprintf("Deleted: %s (%.2f GB freed)", e.MediaName, sizeMB)
}

// DeletionFailedEvent is published when a deletion attempt fails.
type DeletionFailedEvent struct {
	MediaName     string `json:"mediaName"`
	MediaType     string `json:"mediaType"`
	IntegrationID uint   `json:"integrationId"`
	Error         string `json:"error"`
}

func (e DeletionFailedEvent) EventType() string { return "deletion_failed" }
func (e DeletionFailedEvent) EventMessage() string {
	return fmt.Sprintf("Deletion failed: %s — %s", e.MediaName, e.Error)
}

// DeletionDryRunEvent is published when a dry-run deletion is recorded.
type DeletionDryRunEvent struct {
	MediaName string `json:"mediaName"`
	MediaType string `json:"mediaType"`
	SizeBytes int64  `json:"sizeBytes"`
}

func (e DeletionDryRunEvent) EventType() string { return "deletion_dry_run" }
func (e DeletionDryRunEvent) EventMessage() string {
	return fmt.Sprintf("Dry-run flagged: %s", e.MediaName)
}

// =============================================================================
// Rule Events
// =============================================================================

// RuleCreatedEvent is published when a custom rule is created.
type RuleCreatedEvent struct {
	RuleID uint   `json:"ruleId"`
	Field  string `json:"field"`
	Effect string `json:"effect"`
}

func (e RuleCreatedEvent) EventType() string { return "rule_created" }
func (e RuleCreatedEvent) EventMessage() string {
	return fmt.Sprintf("Custom rule created: %s → %s", e.Field, e.Effect)
}

// RuleUpdatedEvent is published when a custom rule is modified.
type RuleUpdatedEvent struct {
	RuleID uint   `json:"ruleId"`
	Field  string `json:"field"`
	Effect string `json:"effect"`
}

func (e RuleUpdatedEvent) EventType() string { return "rule_updated" }
func (e RuleUpdatedEvent) EventMessage() string {
	return fmt.Sprintf("Custom rule updated: %s → %s", e.Field, e.Effect)
}

// RuleDeletedEvent is published when a custom rule is deleted.
type RuleDeletedEvent struct {
	RuleID uint   `json:"ruleId"`
	Field  string `json:"field"`
}

func (e RuleDeletedEvent) EventType() string { return "rule_deleted" }
func (e RuleDeletedEvent) EventMessage() string {
	return fmt.Sprintf("Custom rule deleted: %s (ID %d)", e.Field, e.RuleID)
}

// =============================================================================
// Notification Events
// =============================================================================

// NotificationChannelAddedEvent is published when a notification channel is created.
type NotificationChannelAddedEvent struct {
	ChannelID   uint   `json:"channelId"`
	ChannelType string `json:"channelType"`
	Name        string `json:"name"`
}

func (e NotificationChannelAddedEvent) EventType() string { return "notification_channel_added" }
func (e NotificationChannelAddedEvent) EventMessage() string {
	return fmt.Sprintf("Notification channel added: %s (%s)", e.Name, e.ChannelType)
}

// NotificationChannelUpdatedEvent is published when a notification channel is modified.
type NotificationChannelUpdatedEvent struct {
	ChannelID   uint   `json:"channelId"`
	ChannelType string `json:"channelType"`
	Name        string `json:"name"`
}

func (e NotificationChannelUpdatedEvent) EventType() string { return "notification_channel_updated" }
func (e NotificationChannelUpdatedEvent) EventMessage() string {
	return fmt.Sprintf("Notification channel updated: %s (%s)", e.Name, e.ChannelType)
}

// NotificationChannelRemovedEvent is published when a notification channel is deleted.
type NotificationChannelRemovedEvent struct {
	ChannelID   uint   `json:"channelId"`
	ChannelType string `json:"channelType"`
	Name        string `json:"name"`
}

func (e NotificationChannelRemovedEvent) EventType() string { return "notification_channel_removed" }
func (e NotificationChannelRemovedEvent) EventMessage() string {
	return fmt.Sprintf("Notification channel removed: %s (%s)", e.Name, e.ChannelType)
}

// NotificationSentEvent is published when a notification is successfully delivered.
type NotificationSentEvent struct {
	ChannelID   uint   `json:"channelId"`
	ChannelType string `json:"channelType"`
	Name        string `json:"name"`
	TriggerType string `json:"triggerType"` // The event type that triggered the notification
}

func (e NotificationSentEvent) EventType() string { return "notification_sent" }
func (e NotificationSentEvent) EventMessage() string {
	return fmt.Sprintf("Notification sent via %s (%s)", e.Name, e.ChannelType)
}

// NotificationDeliveryFailedEvent is published when a notification delivery fails.
type NotificationDeliveryFailedEvent struct {
	ChannelID   uint   `json:"channelId"`
	ChannelType string `json:"channelType"`
	Name        string `json:"name"`
	Error       string `json:"error"`
}

func (e NotificationDeliveryFailedEvent) EventType() string { return "notification_delivery_failed" }
func (e NotificationDeliveryFailedEvent) EventMessage() string {
	return fmt.Sprintf("Notification delivery failed: %s (%s) — %s", e.Name, e.ChannelType, e.Error)
}

// =============================================================================
// Data Events
// =============================================================================

// DataResetEvent is published when all scraped data is cleared.
type DataResetEvent struct {
	Summary map[string]int64 `json:"summary"` // e.g. {"audit_log": 42, "approval_queue": 5}
}

func (e DataResetEvent) EventType() string    { return "data_reset" }
func (e DataResetEvent) EventMessage() string { return "All scraped data has been reset" }

// =============================================================================
// System Events
// =============================================================================

// ServerStartedEvent is published when the application starts.
type ServerStartedEvent struct {
	Version string `json:"version"`
}

func (e ServerStartedEvent) EventType() string { return "server_started" }
func (e ServerStartedEvent) EventMessage() string {
	if e.Version != "" {
		return fmt.Sprintf("Server started (version %s)", e.Version)
	}
	return "Server started"
}
