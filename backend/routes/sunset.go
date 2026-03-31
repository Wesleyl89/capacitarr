package routes

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"capacitarr/internal/services"

	"github.com/labstack/echo/v4"
)

// RegisterSunsetRoutes adds sunset queue management endpoints.
func RegisterSunsetRoutes(g *echo.Group, reg *services.Registry) {
	sunset := g.Group("/sunset-queue")

	// GET /api/v1/sunset-queue — list all sunset items with computed daysRemaining
	sunset.GET("", func(c echo.Context) error {
		items, err := reg.Sunset.ListAll()
		if err != nil {
			slog.Error("Failed to list sunset queue", "component", "routes", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to list sunset queue")
		}

		type sunsetResponse struct {
			ID                  uint    `json:"id"`
			MediaName           string  `json:"mediaName"`
			MediaType           string  `json:"mediaType"`
			TmdbID              *int    `json:"tmdbId,omitempty"`
			IntegrationID       uint    `json:"integrationId"`
			SizeBytes           int64   `json:"sizeBytes"`
			Score               float64 `json:"score"`
			ScoreDetails        string  `json:"scoreDetails,omitempty"`
			PosterURL           string  `json:"posterUrl,omitempty"`
			DiskGroupID         uint    `json:"diskGroupId"`
			CollectionGroup     string  `json:"collectionGroup,omitempty"`
			Trigger             string  `json:"trigger"`
			DeletionDate        string  `json:"deletionDate"`
			DaysRemaining       int     `json:"daysRemaining"`
			LabelApplied        bool    `json:"labelApplied"`
			PosterOverlayActive bool    `json:"posterOverlayActive"`
			Status              string  `json:"status"`
			SavedAt             string  `json:"savedAt,omitempty"`
			SavedScore          float64 `json:"savedScore,omitempty"`
			SavedReason         string  `json:"savedReason,omitempty"`
			ExpiredAt           string  `json:"expiredAt,omitempty"`
			CreatedAt           string  `json:"createdAt"`
		}

		result := make([]sunsetResponse, len(items))
		for i, item := range items {
			resp := sunsetResponse{
				ID:                  item.ID,
				MediaName:           item.MediaName,
				MediaType:           item.MediaType,
				TmdbID:              item.TmdbID,
				IntegrationID:       item.IntegrationID,
				SizeBytes:           item.SizeBytes,
				Score:               item.Score,
				ScoreDetails:        item.ScoreDetails,
				PosterURL:           item.PosterURL,
				DiskGroupID:         item.DiskGroupID,
				CollectionGroup:     item.CollectionGroup,
				Trigger:             item.Trigger,
				DeletionDate:        item.DeletionDate.Format("2006-01-02"),
				DaysRemaining:       reg.Sunset.DaysRemaining(item),
				LabelApplied:        item.LabelApplied,
				PosterOverlayActive: item.PosterOverlayActive,
				Status:              item.Status,
				SavedScore:          item.SavedScore,
				SavedReason:         item.SavedReason,
				CreatedAt:           item.CreatedAt.Format(time.RFC3339),
			}
			if item.SavedAt != nil {
				resp.SavedAt = item.SavedAt.Format(time.RFC3339)
			}
			if item.ExpiredAt != nil {
				resp.ExpiredAt = item.ExpiredAt.Format(time.RFC3339)
			}
			result[i] = resp
		}
		return c.JSON(http.StatusOK, result)
	})

	// DELETE /api/v1/sunset-queue/:id — cancel a sunset item
	sunset.DELETE("/:id", func(c echo.Context) error {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid ID")
		}

		// Build the integration registry so label removal works from the UI.
		registry, registryErr := reg.Integration.BuildIntegrationRegistry()
		if registryErr != nil {
			slog.Error("Failed to build integration registry for sunset cancel — label removal may be skipped",
				"component", "routes", "error", registryErr)
		}

		if err := reg.Sunset.Cancel(uint(id), services.SunsetDeps{
			Registry:      registry,
			Deletion:      reg.Deletion,
			Engine:        reg.Engine,
			Settings:      reg.Settings,
			PosterOverlay: reg.PosterOverlay,
			Mapping:       reg.Mapping,
		}); err != nil {
			slog.Error("Failed to cancel sunset item", "component", "routes", "id", id, "error", err)
			return apiError(c, http.StatusNotFound, "Sunset item not found")
		}

		return c.JSON(http.StatusOK, map[string]string{"status": "cancelled"})
	})

	// PATCH /api/v1/sunset-queue/:id — reschedule (change deletion date)
	sunset.PATCH("/:id", func(c echo.Context) error {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid ID")
		}

		var payload struct {
			DeletionDate string `json:"deletionDate"` // YYYY-MM-DD
		}
		if err := c.Bind(&payload); err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid payload")
		}

		newDate, err := time.Parse("2006-01-02", payload.DeletionDate)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid date format — use YYYY-MM-DD")
		}

		if newDate.Before(time.Now().UTC().Truncate(24 * time.Hour)) {
			return apiError(c, http.StatusBadRequest, "Deletion date cannot be in the past")
		}

		item, err := reg.Sunset.Reschedule(uint(id), newDate)
		if err != nil {
			slog.Error("Failed to reschedule sunset item", "component", "routes", "id", id, "error", err)
			return apiError(c, http.StatusNotFound, "Sunset item not found")
		}

		return c.JSON(http.StatusOK, map[string]any{
			"id":            item.ID,
			"mediaName":     item.MediaName,
			"deletionDate":  item.DeletionDate.Format("2006-01-02"),
			"daysRemaining": reg.Sunset.DaysRemaining(*item),
		})
	})

	// POST /api/v1/sunset-queue/clear — cancel all sunset items
	sunset.POST("/clear", func(c echo.Context) error {
		registry, registryErr := reg.Integration.BuildIntegrationRegistry()
		if registryErr != nil {
			slog.Error("Failed to build integration registry for sunset clear — label removal may be skipped",
				"component", "routes", "error", registryErr)
		}

		count, err := reg.Sunset.CancelAll(services.SunsetDeps{
			Registry:      registry,
			Deletion:      reg.Deletion,
			Engine:        reg.Engine,
			Settings:      reg.Settings,
			PosterOverlay: reg.PosterOverlay,
			Mapping:       reg.Mapping,
		})
		if err != nil {
			slog.Error("Failed to clear sunset queue", "component", "routes", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to clear sunset queue")
		}

		return c.JSON(http.StatusOK, map[string]any{
			"status":    "cleared",
			"cancelled": count,
		})
	})

	// POST /api/v1/sunset-queue/refresh-labels — re-apply labels to unlabeled queue items
	sunset.POST("/refresh-labels", func(c echo.Context) error {
		registry, registryErr := reg.Integration.BuildIntegrationRegistry()
		if registryErr != nil {
			slog.Error("Failed to build integration registry for label refresh",
				"component", "routes", "error", registryErr)
		}

		applied, err := reg.Sunset.RefreshLabels(services.SunsetDeps{
			Registry: registry,
			Settings: reg.Settings,
		})
		if err != nil {
			slog.Error("Failed to refresh labels", "component", "routes", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to refresh labels")
		}

		return c.JSON(http.StatusOK, map[string]any{
			"status":  "refreshed",
			"applied": applied,
		})
	})

	// POST /api/v1/sunset-queue/refresh-posters — force re-generate and re-upload all overlays
	sunset.POST("/refresh-posters", func(c echo.Context) error {
		if reg.PosterOverlay == nil {
			return apiError(c, http.StatusServiceUnavailable, "Poster overlay service not available")
		}

		registry, registryErr := reg.Integration.BuildIntegrationRegistry()
		if registryErr != nil {
			slog.Error("Failed to build integration registry for poster refresh — refresh may be incomplete",
				"component", "routes", "error", registryErr)
		}

		updated, err := reg.PosterOverlay.UpdateAll(reg.Sunset, services.PosterDeps{
			Registry: registry,
			Mapping:  reg.Mapping,
		})
		if err != nil {
			slog.Error("Failed to refresh posters", "component", "routes", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to refresh posters")
		}

		return c.JSON(http.StatusOK, map[string]any{
			"status":  "refreshed",
			"updated": updated,
		})
	})

	// POST /api/v1/sunset-queue/restore-posters — restore all original posters (emergency)
	sunset.POST("/restore-posters", func(c echo.Context) error {
		if reg.PosterOverlay == nil {
			return apiError(c, http.StatusServiceUnavailable, "Poster overlay service not available")
		}

		registry, registryErr := reg.Integration.BuildIntegrationRegistry()
		if registryErr != nil {
			slog.Error("Failed to build integration registry for poster restore — restore may be incomplete",
				"component", "routes", "error", registryErr)
		}

		restored, err := reg.PosterOverlay.RestoreAll(reg.Sunset, services.PosterDeps{
			Registry: registry,
			Mapping:  reg.Mapping,
		})
		if err != nil {
			slog.Error("Failed to restore posters", "component", "routes", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to restore posters")
		}

		return c.JSON(http.StatusOK, map[string]any{
			"status":   "restored",
			"restored": restored,
		})
	})
}
