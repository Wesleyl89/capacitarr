package routes

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/db"
	"capacitarr/internal/engine"
	"capacitarr/internal/integrations"
	"capacitarr/internal/services"
)

// RegisterApprovalRoutes sets up the API endpoints for the approval queue.
func RegisterApprovalRoutes(g *echo.Group, reg *services.Registry) {
	database := reg.DB

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

		items := make([]db.ApprovalQueueItem, 0)
		query := database.Model(&db.ApprovalQueueItem{})

		if status := c.QueryParam("status"); status != "" {
			query = query.Where("status = ?", status)
		}

		if err := query.Order("created_at desc").Limit(limit).Find(&items).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch approval queue"})
		}

		return c.JSON(http.StatusOK, items)
	})

	// Approve a queued item: queue it for deletion
	g.POST("/approval-queue/:id/approve", func(c echo.Context) error {
		id := c.Param("id")
		entryID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		}

		// Safety check: block approvals when deletions are disabled
		var prefs db.PreferenceSet
		if err := database.FirstOrCreate(&prefs, db.PreferenceSet{ID: 1}).Error; err != nil {
			slog.Error("Failed to load preferences for approval check", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to load preferences",
			})
		}
		if !prefs.DeletionsEnabled {
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "Deletions are currently disabled in settings. Enable deletions before approving items.",
			})
		}

		// Mark as approved via service (single fetch + status validation)
		approved, err := reg.Approval.Approve(uint(entryID))
		if err != nil {
			if err.Error() == "entry is not pending (current status: approved)" ||
				err.Error() == "entry is not pending (current status: rejected)" {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			if err.Error() == "approval queue entry not found: record not found" {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "Approval queue entry not found"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to approve entry"})
		}

		// Look up the integration to construct a client for deletion
		var integration db.IntegrationConfig
		if err := database.First(&integration, approved.IntegrationID).Error; err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Integration not found",
			})
		}

		client := integrations.NewClient(integration.Type, integration.URL, integration.APIKey)
		if client == nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Unsupported integration type",
			})
		}

		// Reconstruct the MediaItem from stored approval data
		item := integrations.MediaItem{
			ExternalID:    approved.ExternalID,
			IntegrationID: approved.IntegrationID,
			Type:          integrations.MediaType(approved.MediaType),
			Title:         approved.MediaName,
			SizeBytes:     approved.SizeBytes,
		}

		// Parse stored score details back into factors
		var factors []engine.ScoreFactor
		if approved.ScoreDetails != "" {
			if err := json.Unmarshal([]byte(approved.ScoreDetails), &factors); err != nil {
				slog.Warn("Failed to parse score details for approval", "id", approved.ID, "error", err)
			}
		}

		// Queue for background deletion via DeletionService
		if err := reg.Deletion.QueueDeletion(services.DeleteJob{
			Client:  client,
			Item:    item,
			Reason:  approved.Reason,
			Score:   0,
			Factors: factors,
		}); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"error": "Deletion queue is full, try again later",
			})
		}

		return c.JSON(http.StatusOK, approved)
	})

	// Reject a queued item: snooze it
	g.POST("/approval-queue/:id/reject", func(c echo.Context) error {
		id := c.Param("id")
		entryID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		}

		// Load preferences to get configured snooze duration
		var prefs db.PreferenceSet
		if err := database.FirstOrCreate(&prefs, db.PreferenceSet{ID: 1}).Error; err != nil {
			slog.Error("Failed to load preferences for snooze duration", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to load preferences",
			})
		}

		rejected, err := reg.Approval.Reject(uint(entryID), prefs.SnoozeDurationHours)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, rejected)
	})

	// Unsnooze a rejected item: clear snooze and reset to pending
	g.POST("/approval-queue/:id/unsnooze", func(c echo.Context) error {
		id := c.Param("id")
		entryID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		}

		unsnoozed, err := reg.Approval.Unsnooze(uint(entryID))
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, unsnoozed)
	})
}
