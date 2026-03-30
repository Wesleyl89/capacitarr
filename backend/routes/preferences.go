package routes

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/db"
	"capacitarr/internal/logger"
	"capacitarr/internal/services"
)

// preferencesResponse wraps PreferenceSet with runtime metadata that the
// frontend needs but that isn't stored in the database.
type preferencesResponse struct {
	db.PreferenceSet
	LogLevelOverridden bool `json:"logLevelOverridden"` // true when DEBUG=true pins the log level to debug
}

// RegisterPreferenceRoutes sets up the endpoints for managing the PreferenceSet singleton.
// Note: Scoring factor weights have been moved to their own table and API — see factorweights.go.
func RegisterPreferenceRoutes(protected *echo.Group, reg *services.Registry) {
	protected.GET("/preferences", func(c echo.Context) error {
		pref, err := reg.Settings.GetPreferences()
		if err != nil {
			slog.Error("Failed to fetch preferences", "component", "api", "operation", "fetch_preferences", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to fetch preferences")
		}
		return c.JSON(http.StatusOK, preferencesResponse{
			PreferenceSet:      pref,
			LogLevelOverridden: logger.DebugOverride(),
		})
	})

	protected.PUT("/preferences", func(c echo.Context) error {
		var payload db.PreferenceSet
		if err := c.Bind(&payload); err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid request payload")
		}
		// Force ID to 1 to ensure a single singleton record
		payload.ID = 1

		// Validate tiebreaker method
		if payload.TiebreakerMethod == "" {
			payload.TiebreakerMethod = db.TiebreakerSizeDesc
		}
		if !db.ValidTiebreakerMethods[payload.TiebreakerMethod] {
			return apiError(c, http.StatusBadRequest, "Tiebreaker method must be one of: "+db.FormatValidKeys(db.ValidTiebreakerMethods))
		}

		// Validate default disk group mode
		if !db.ValidExecutionModes[payload.DefaultDiskGroupMode] {
			return apiError(c, http.StatusBadRequest, "Default disk group mode must be one of: "+db.FormatValidKeys(db.ValidExecutionModes))
		}

		// Validate log level
		if !db.ValidLogLevels[payload.LogLevel] {
			return apiError(c, http.StatusBadRequest, "Log level must be one of: "+db.FormatValidKeys(db.ValidLogLevels))
		}

		// Validate poll interval (minimum 60s; default to 300s if omitted/zero)
		if payload.PollIntervalSeconds == 0 {
			payload.PollIntervalSeconds = 300
		} else if payload.PollIntervalSeconds < 60 {
			return apiError(c, http.StatusBadRequest, "Poll interval must be at least 60 seconds")
		}

		// Validate deletion queue delay (10-300 seconds; default to 30s if omitted/zero)
		if payload.DeletionQueueDelaySeconds == 0 {
			payload.DeletionQueueDelaySeconds = 30
		} else if payload.DeletionQueueDelaySeconds < 10 || payload.DeletionQueueDelaySeconds > 300 {
			return apiError(c, http.StatusBadRequest, "Deletion queue delay must be between 10 and 300 seconds")
		}

		// Delegate to SettingsService (handles DB save, log level change, event publishing)
		saved, err := reg.Settings.UpdatePreferences(payload)
		if err != nil {
			slog.Error("Failed to update preferences", "component", "api", "operation", "update_preferences", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to update preferences")
		}

		return c.JSON(http.StatusOK, saved)
	})

	// ─── PATCH Field Groups ─────────────────────────────────────────────

	protected.PATCH("/preferences/engine", func(c echo.Context) error {
		var patch services.EnginePreferencePatch
		if err := c.Bind(&patch); err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid request payload")
		}

		// Validate provided fields
		if patch.DefaultDiskGroupMode != nil && !db.ValidExecutionModes[*patch.DefaultDiskGroupMode] {
			return apiError(c, http.StatusBadRequest, "Default disk group mode must be one of: "+db.FormatValidKeys(db.ValidExecutionModes))
		}
		if patch.TiebreakerMethod != nil {
			if *patch.TiebreakerMethod == "" {
				v := db.TiebreakerSizeDesc
				patch.TiebreakerMethod = &v
			}
			if !db.ValidTiebreakerMethods[*patch.TiebreakerMethod] {
				return apiError(c, http.StatusBadRequest, "Tiebreaker method must be one of: "+db.FormatValidKeys(db.ValidTiebreakerMethods))
			}
		}
		if patch.SnoozeDurationHours != nil && *patch.SnoozeDurationHours < 1 {
			return apiError(c, http.StatusBadRequest, "Snooze duration must be at least 1 hour")
		}

		saved, err := reg.Settings.PatchEnginePreferences(patch)
		if err != nil {
			slog.Error("Failed to patch engine preferences", "component", "api", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to update engine preferences")
		}
		return c.JSON(http.StatusOK, saved)
	})

	protected.PATCH("/preferences/sunset", func(c echo.Context) error {
		var patch services.SunsetPreferencePatch
		if err := c.Bind(&patch); err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid request payload")
		}

		// Validate provided fields
		if patch.SunsetDays != nil && (*patch.SunsetDays < 1 || *patch.SunsetDays > 365) {
			return apiError(c, http.StatusBadRequest, "Sunset days must be between 1 and 365")
		}
		if patch.SunsetLabel != nil && *patch.SunsetLabel == "" {
			return apiError(c, http.StatusBadRequest, "Sunset label must not be empty")
		}

		saved, err := reg.Settings.PatchSunsetPreferences(patch)
		if err != nil {
			slog.Error("Failed to patch sunset preferences", "component", "api", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to update sunset preferences")
		}
		return c.JSON(http.StatusOK, saved)
	})

	protected.PATCH("/preferences/content", func(c echo.Context) error {
		var patch services.ContentPreferencePatch
		if err := c.Bind(&patch); err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid request payload")
		}

		// Validate provided fields
		if patch.DeadContentMinDays != nil && *patch.DeadContentMinDays < 1 {
			return apiError(c, http.StatusBadRequest, "Dead content minimum days must be at least 1")
		}
		if patch.StaleContentDays != nil && *patch.StaleContentDays < 1 {
			return apiError(c, http.StatusBadRequest, "Stale content days must be at least 1")
		}

		saved, err := reg.Settings.PatchContentPreferences(patch)
		if err != nil {
			slog.Error("Failed to patch content preferences", "component", "api", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to update content preferences")
		}
		return c.JSON(http.StatusOK, saved)
	})

	protected.PATCH("/preferences/advanced", func(c echo.Context) error {
		var patch services.AdvancedPreferencePatch
		if err := c.Bind(&patch); err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid request payload")
		}

		// Validate provided fields
		if patch.LogLevel != nil && !db.ValidLogLevels[*patch.LogLevel] {
			return apiError(c, http.StatusBadRequest, "Log level must be one of: "+db.FormatValidKeys(db.ValidLogLevels))
		}
		if patch.PollIntervalSeconds != nil {
			if *patch.PollIntervalSeconds == 0 {
				v := 300
				patch.PollIntervalSeconds = &v
			} else if *patch.PollIntervalSeconds < 60 {
				return apiError(c, http.StatusBadRequest, "Poll interval must be at least 60 seconds")
			}
		}
		if patch.DeletionQueueDelaySeconds != nil {
			if *patch.DeletionQueueDelaySeconds == 0 {
				v := 30
				patch.DeletionQueueDelaySeconds = &v
			} else if *patch.DeletionQueueDelaySeconds < 10 || *patch.DeletionQueueDelaySeconds > 300 {
				return apiError(c, http.StatusBadRequest, "Deletion queue delay must be between 10 and 300 seconds")
			}
		}
		if patch.AuditLogRetentionDays != nil && *patch.AuditLogRetentionDays < 0 {
			return apiError(c, http.StatusBadRequest, "Audit log retention days must be 0 or greater")
		}

		saved, err := reg.Settings.PatchAdvancedPreferences(patch)
		if err != nil {
			slog.Error("Failed to patch advanced preferences", "component", "api", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to update advanced preferences")
		}
		return c.JSON(http.StatusOK, saved)
	})
}
