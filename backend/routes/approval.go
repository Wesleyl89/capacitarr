package routes

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/db"
	"capacitarr/internal/services"
)

// RegisterApprovalRoutes sets up the API endpoints for the approval queue.
func RegisterApprovalRoutes(g *echo.Group, reg *services.Registry) {
	// List approval queue items
	g.GET("/approval-queue", func(c echo.Context) error {
		limit := 200
		if l := c.QueryParam("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		if limit > 2000 {
			limit = 2000
		}

		status := c.QueryParam("status")
		items, err := reg.Approval.ListQueue(status, limit)
		if err != nil {
			return apiError(c, http.StatusInternalServerError, "Failed to fetch approval queue")
		}

		return c.JSON(http.StatusOK, items)
	})

	// Approve a queued item: queue it for deletion
	g.POST("/approval-queue/:id/approve", func(c echo.Context) error {
		id := c.Param("id")
		entryID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid ID")
		}

		// Safety check: block approvals when deletions are disabled
		prefs, err := reg.Settings.GetPreferences()
		if err != nil {
			slog.Error("Failed to load preferences for approval check", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to load preferences")
		}
		if !prefs.DeletionsEnabled {
			return apiError(c, http.StatusConflict, "Deletions are currently disabled in settings. Enable deletions before approving items.")
		}

		// Execute the full approval workflow via service
		approved, err := reg.Approval.ExecuteApproval(uint(entryID), services.ExecuteApprovalDeps{
			Integration: reg.Integration,
			Deletion:    reg.Deletion,
			Engine:      reg.Engine,
		})
		if err != nil {
			if errors.Is(err, services.ErrApprovalNotPending) {
				return apiError(c, http.StatusBadRequest, err.Error())
			}
			if errors.Is(err, services.ErrApprovalNotFound) {
				return apiError(c, http.StatusNotFound, "Approval queue entry not found")
			}
			slog.Error("Approval execution failed", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to approve entry")
		}

		return c.JSON(http.StatusOK, approved)
	})

	// Reject a queued item: snooze it
	g.POST("/approval-queue/:id/reject", func(c echo.Context) error {
		id := c.Param("id")
		entryID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid ID")
		}

		// Load preferences to get configured snooze duration
		prefs, err := reg.Settings.GetPreferences()
		if err != nil {
			slog.Error("Failed to load preferences for snooze duration", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to load preferences")
		}

		rejected, err := reg.Approval.Reject(uint(entryID), prefs.SnoozeDurationHours)
		if err != nil {
			if errors.Is(err, services.ErrApprovalNotPending) {
				return apiError(c, http.StatusBadRequest, err.Error())
			}
			if errors.Is(err, services.ErrApprovalNotFound) {
				return apiError(c, http.StatusNotFound, "Approval queue entry not found")
			}
			return apiError(c, http.StatusInternalServerError, "Failed to reject entry")
		}

		return c.JSON(http.StatusOK, rejected)
	})

	// Force-delete: queue items for deletion regardless of disk threshold
	g.POST("/force-delete", func(c echo.Context) error {
		// Safety check: block when deletions are disabled
		prefs, err := reg.Settings.GetPreferences()
		if err != nil {
			slog.Error("Failed to load preferences for force-delete check", "error", err)
			return apiError(c, http.StatusInternalServerError, "Failed to load preferences")
		}
		if !prefs.DeletionsEnabled {
			return apiError(c, http.StatusConflict, "Deletions are currently disabled in settings")
		}
		if prefs.ExecutionMode == "dry-run" {
			return apiError(c, http.StatusConflict, "Force delete is not available in dry-run mode. Switch to approval or auto mode.")
		}

		// Parse request body — array of items to force-delete
		var items []struct {
			MediaName     string `json:"mediaName"`
			MediaType     string `json:"mediaType"`
			IntegrationID uint   `json:"integrationId"`
			ExternalID    string `json:"externalId"`
			SizeBytes     int64  `json:"sizeBytes"`
			Reason        string `json:"reason"`
			ScoreDetails  string `json:"scoreDetails"`
			PosterURL     string `json:"posterUrl"`
		}
		if err := c.Bind(&items); err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid request body")
		}
		if len(items) == 0 {
			return apiError(c, http.StatusBadRequest, "No items provided")
		}

		var created int
		for _, item := range items {
			if _, err := reg.Approval.CreateForceDelete(db.ApprovalQueueItem{
				MediaName:     item.MediaName,
				MediaType:     item.MediaType,
				IntegrationID: item.IntegrationID,
				ExternalID:    item.ExternalID,
				SizeBytes:     item.SizeBytes,
				Reason:        item.Reason,
				ScoreDetails:  item.ScoreDetails,
				PosterURL:     item.PosterURL,
			}); err != nil {
				slog.Error("Failed to create force-delete entry", "media", item.MediaName, "error", err)
				continue
			}
			created++
		}

		return c.JSON(http.StatusOK, map[string]any{
			"queued": created,
			"total":  len(items),
		})
	})

	// Unsnooze a rejected item: clear snooze and reset to pending
	g.POST("/approval-queue/:id/unsnooze", func(c echo.Context) error {
		id := c.Param("id")
		entryID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid ID")
		}

		unsnoozed, err := reg.Approval.Unsnooze(uint(entryID))
		if err != nil {
			if errors.Is(err, services.ErrApprovalNotPending) {
				return apiError(c, http.StatusBadRequest, err.Error())
			}
			if errors.Is(err, services.ErrApprovalNotFound) {
				return apiError(c, http.StatusNotFound, "Approval queue entry not found")
			}
			return apiError(c, http.StatusInternalServerError, "Failed to unsnooze entry")
		}

		return c.JSON(http.StatusOK, unsnoozed)
	})

	// Dismiss a single queued item (pending or rejected only)
	g.DELETE("/approval-queue/:id", func(c echo.Context) error {
		id := c.Param("id")
		entryID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid ID")
		}

		if err := reg.Approval.Dismiss(uint(entryID)); err != nil {
			if errors.Is(err, services.ErrApprovalNotDismissable) {
				return apiError(c, http.StatusBadRequest, err.Error())
			}
			if errors.Is(err, services.ErrApprovalNotFound) {
				return apiError(c, http.StatusNotFound, "Approval queue entry not found")
			}
			return apiError(c, http.StatusInternalServerError, "Failed to dismiss entry")
		}

		return c.JSON(http.StatusOK, map[string]string{"status": "dismissed"})
	})

	// Clear the entire approval queue (pending + rejected items)
	g.POST("/approval-queue/clear", func(c echo.Context) error {
		count, err := reg.Approval.ClearQueue()
		if err != nil {
			return apiError(c, http.StatusInternalServerError, "Failed to clear approval queue")
		}

		return c.JSON(http.StatusOK, map[string]any{"cleared": count})
	})
}
