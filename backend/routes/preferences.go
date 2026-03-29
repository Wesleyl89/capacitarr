package routes

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/db"
	"capacitarr/internal/services"
)

// RegisterPreferenceRoutes sets up the endpoints for managing the PreferenceSet singleton.
// Note: Scoring factor weights have been moved to their own table and API — see factorweights.go.
func RegisterPreferenceRoutes(protected *echo.Group, reg *services.Registry) {
	protected.GET("/preferences", func(c echo.Context) error {
		pref, err := reg.Settings.GetPreferences()
		if err != nil {
			slog.Error("Failed to fetch preferences", "component", "api", "operation", "fetch_preferences", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to fetch preferences")
		}
		return c.JSON(http.StatusOK, pref)
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
}
