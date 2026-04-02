package services

import (
	"log/slog"
	"sync"
	"time"

	"capacitarr/internal/events"
	"capacitarr/internal/integrations"
)

// Recovery backoff constants. The probe interval starts at recoveryBaseDelay
// and doubles on each consecutive failure, capped at recoveryMaxDelay.
const (
	recoveryBaseDelay  = 30 * time.Second
	recoveryMaxDelay   = 5 * time.Minute
	recoveryTickPeriod = 15 * time.Second
)

// RecoveryTracker is the interface consumed by IntegrationService to notify
// the recovery monitor of state changes. Defined as an interface to avoid
// import cycles and simplify testing.
type RecoveryTracker interface {
	TrackFailure(id uint, intType, name, url, apiKey, errMsg string)
	TrackRecovery(id uint)
}

// recoveryState holds in-memory tracking for a single failing integration.
type recoveryState struct {
	IntegrationID       uint
	IntegrationType     string
	Name                string
	URL                 string
	APIKey              string
	LastError           string
	ConsecutiveFailures int
	NextRetry           time.Time
	LastAttempt         time.Time
}

// nextBackoff calculates the next retry delay using exponential backoff.
// delay = baseDelay * 2^(failures-1), capped at maxDelay.
func (s *recoveryState) nextBackoff() time.Duration {
	if s.ConsecutiveFailures <= 0 {
		return recoveryBaseDelay
	}
	shift := s.ConsecutiveFailures - 1
	if shift > 10 {
		shift = 10 // prevent overflow
	}
	delay := recoveryBaseDelay * (1 << shift)
	if delay > recoveryMaxDelay {
		delay = recoveryMaxDelay
	}
	return delay
}

// IntegrationHealthEntry is the API-facing snapshot of a tracked integration's
// recovery state. Returned by RecoveryService.HealthStatus().
type IntegrationHealthEntry struct {
	IntegrationID       uint      `json:"integrationId"`
	IntegrationType     string    `json:"integrationType"`
	Name                string    `json:"name"`
	ConsecutiveFailures int       `json:"consecutiveFailures"`
	LastError           string    `json:"lastError"`
	NextRetryAt         time.Time `json:"nextRetryAt"`
	Recovering          bool      `json:"recovering"`
}

// RecoveryService monitors failing integrations and probes them with
// exponential backoff to detect recovery between poll cycles.
type RecoveryService struct {
	integrationSvc *IntegrationService
	bus            *events.EventBus

	mu       sync.Mutex
	tracked  map[uint]*recoveryState // integrationID → state
	done     chan struct{}
	stopOnce sync.Once
}

// NewRecoveryService creates a RecoveryService. Call Start() to begin probing.
func NewRecoveryService(integrationSvc *IntegrationService, bus *events.EventBus) *RecoveryService {
	return &RecoveryService{
		integrationSvc: integrationSvc,
		bus:            bus,
		tracked:        make(map[uint]*recoveryState),
		done:           make(chan struct{}),
	}
}

// Start seeds the recovery map from DB state (integrations with non-empty
// LastError) and begins the background probing goroutine.
func (r *RecoveryService) Start() {
	r.seed()

	go func() {
		ticker := time.NewTicker(recoveryTickPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r.tick()
			case <-r.done:
				return
			}
		}
	}()

	slog.Info("Recovery service started", "component", "recovery")
}

// Stop signals the background goroutine to exit.
func (r *RecoveryService) Stop() {
	r.stopOnce.Do(func() {
		close(r.done)
		slog.Info("Recovery service stopped", "component", "recovery")
	})
}

// seed loads all enabled integrations with non-empty LastError from the DB
// and adds them to the recovery tracker. Called once on startup.
func (r *RecoveryService) seed() {
	configs, err := r.integrationSvc.ListEnabled()
	if err != nil {
		slog.Error("Recovery seed: failed to list integrations",
			"component", "recovery", "error", err)
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	seeded := 0
	for _, cfg := range configs {
		if cfg.LastError == "" {
			continue
		}
		r.tracked[cfg.ID] = &recoveryState{
			IntegrationID:       cfg.ID,
			IntegrationType:     cfg.Type,
			Name:                cfg.Name,
			URL:                 cfg.URL,
			APIKey:              cfg.APIKey,
			LastError:           cfg.LastError,
			ConsecutiveFailures: cfg.ConsecutiveFailures,
			NextRetry:           time.Now().Add(recoveryBaseDelay),
		}
		seeded++
	}

	if seeded > 0 {
		slog.Info("Recovery seed: tracking failing integrations from DB",
			"component", "recovery", "count", seeded)
	}
}

// TrackFailure registers or updates a failing integration in the recovery map.
// Called by IntegrationService.UpdateSyncStatus when an error is recorded.
func (r *RecoveryService) TrackFailure(id uint, intType, name, url, apiKey, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, exists := r.tracked[id]
	if !exists {
		state = &recoveryState{
			IntegrationID:   id,
			IntegrationType: intType,
			Name:            name,
			URL:             url,
			APIKey:          apiKey,
		}
		r.tracked[id] = state
	}

	state.ConsecutiveFailures++
	state.LastError = errMsg
	state.URL = url
	state.APIKey = apiKey
	state.Name = name
	state.IntegrationType = intType
	state.NextRetry = time.Now().Add(state.nextBackoff())

	slog.Debug("Recovery: tracking failure",
		"component", "recovery",
		"integrationID", id,
		"name", name,
		"failures", state.ConsecutiveFailures,
		"nextRetry", state.NextRetry.Format(time.RFC3339))
}

// TrackRecovery removes an integration from the recovery map. Called when
// a connection test succeeds (from poller or manual test).
func (r *RecoveryService) TrackRecovery(id uint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tracked[id]; exists {
		delete(r.tracked, id)
		slog.Debug("Recovery: integration recovered, removed from tracker",
			"component", "recovery", "integrationID", id)
	}
}

// HealthStatus returns a snapshot of all tracked failing integrations.
func (r *RecoveryService) HealthStatus() []IntegrationHealthEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries := make([]IntegrationHealthEntry, 0, len(r.tracked))
	for _, state := range r.tracked {
		entries = append(entries, IntegrationHealthEntry{
			IntegrationID:       state.IntegrationID,
			IntegrationType:     state.IntegrationType,
			Name:                state.Name,
			ConsecutiveFailures: state.ConsecutiveFailures,
			LastError:           state.LastError,
			NextRetryAt:         state.NextRetry,
			Recovering:          true,
		})
	}
	return entries
}

// TrackedCount returns the number of integrations currently being monitored.
func (r *RecoveryService) TrackedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.tracked)
}

// tick is called periodically by the background goroutine. It checks which
// integrations are due for a recovery probe and tests them.
func (r *RecoveryService) tick() {
	now := time.Now()

	// Collect integrations due for probing under the lock
	r.mu.Lock()
	var due []*recoveryState
	for _, state := range r.tracked {
		if now.After(state.NextRetry) || now.Equal(state.NextRetry) {
			// Copy state for use outside the lock
			cp := *state
			due = append(due, &cp)
		}
	}
	r.mu.Unlock()

	if len(due) == 0 {
		return
	}

	// Probe each due integration outside the lock (network I/O)
	for _, state := range due {
		r.probeIntegration(state)
	}
}

// probeIntegration tests connectivity to a single integration and updates
// state based on the result.
func (r *RecoveryService) probeIntegration(state *recoveryState) {
	integrations.RegisterAllFactories()

	rawClient := integrations.CreateClient(state.IntegrationType, state.URL, state.APIKey)
	if rawClient == nil {
		slog.Warn("Recovery probe: unknown integration type",
			"component", "recovery",
			"integrationID", state.IntegrationID,
			"type", state.IntegrationType)
		return
	}

	conn, ok := rawClient.(integrations.Connectable)
	if !ok {
		return
	}

	attempt := state.ConsecutiveFailures + 1
	err := conn.TestConnection()

	if err == nil {
		// Recovery detected
		slog.Info("Recovery probe: integration recovered",
			"component", "recovery",
			"integrationID", state.IntegrationID,
			"name", state.Name,
			"type", state.IntegrationType,
			"afterAttempts", attempt)

		// Update DB: clear error, reset consecutive failures
		r.integrationSvc.PublishRecoveryIfNeeded(state.IntegrationID)
		now := time.Now()
		_ = r.integrationSvc.UpdateSyncStatusDirect(state.IntegrationID, &now, "", 0)

		// Remove from tracker
		r.TrackRecovery(state.IntegrationID)

		// Publish recovery attempt event (success)
		r.bus.Publish(events.IntegrationRecoveryAttemptEvent{
			IntegrationID:   state.IntegrationID,
			IntegrationType: state.IntegrationType,
			Name:            state.Name,
			Attempt:         attempt,
			Success:         true,
		})
		return
	}

	// Still failing — update state
	r.mu.Lock()
	if tracked, exists := r.tracked[state.IntegrationID]; exists {
		tracked.ConsecutiveFailures++
		tracked.LastError = err.Error()
		tracked.LastAttempt = time.Now()
		tracked.NextRetry = time.Now().Add(tracked.nextBackoff())

		slog.Debug("Recovery probe: still failing",
			"component", "recovery",
			"integrationID", state.IntegrationID,
			"name", state.Name,
			"failures", tracked.ConsecutiveFailures,
			"error", err.Error(),
			"nextRetry", tracked.NextRetry.Format(time.RFC3339))
	}
	r.mu.Unlock()

	// Publish recovery attempt event (failure)
	nextDelay := state.nextBackoff()
	r.bus.Publish(events.IntegrationRecoveryAttemptEvent{
		IntegrationID:    state.IntegrationID,
		IntegrationType:  state.IntegrationType,
		Name:             state.Name,
		Attempt:          attempt,
		Success:          false,
		Error:            err.Error(),
		NextRetrySeconds: int(nextDelay.Seconds()),
	})
}
