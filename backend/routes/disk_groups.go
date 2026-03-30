package routes

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/db"
	"capacitarr/internal/services"
)

// RegisterDiskGroupRoutes registers disk group management endpoints on the protected group.
func RegisterDiskGroupRoutes(g *echo.Group, reg *services.Registry) {
	g.GET("/disk-groups", func(c echo.Context) error {
		groups, err := reg.DiskGroup.ListWithIntegrations()
		if err != nil {
			return apiError(c, http.StatusInternalServerError, "Failed to fetch disk groups")
		}
		return c.JSON(http.StatusOK, groups)
	})

	g.PUT("/disk-groups/:id", func(c echo.Context) error {
		id := c.Param("id")
		idNum, convErr := strconv.ParseUint(id, 10, 64)
		if convErr != nil {
			return apiError(c, http.StatusBadRequest, "Invalid ID")
		}

		group, err := reg.DiskGroup.GetByID(uint(idNum))
		if err != nil {
			return apiError(c, http.StatusNotFound, "Disk group not found")
		}

		var req struct {
			ThresholdPct       float64  `json:"thresholdPct"`
			TargetPct          float64  `json:"targetPct"`
			TotalBytesOverride *int64   `json:"totalBytesOverride"`
			Mode               string   `json:"mode"`
			SunsetPct          *float64 `json:"sunsetPct"`
		}
		if err := c.Bind(&req); err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid request body")
		}

		// Validate thresholds
		if req.ThresholdPct < 1 || req.ThresholdPct > 99 || req.TargetPct < 1 || req.TargetPct > 99 {
			return apiError(c, http.StatusBadRequest, "Threshold and target must be between 1 and 99")
		}
		if req.ThresholdPct <= req.TargetPct {
			return apiError(c, http.StatusBadRequest, "Threshold must be greater than target")
		}

		// Validate override: negative values are not allowed
		if req.TotalBytesOverride != nil && *req.TotalBytesOverride < 0 {
			return apiError(c, http.StatusBadRequest, "Total bytes override must not be negative")
		}

		// Validate mode if provided (empty string preserves existing mode)
		if req.Mode != "" {
			if !db.ValidExecutionModes[req.Mode] {
				return apiError(c, http.StatusBadRequest, "Mode must be one of: "+db.FormatValidKeys(db.ValidExecutionModes))
			}
		}

		// Sunset threshold validation (sunsetPct < targetPct < thresholdPct, nil
		// rejection for sunset mode) is handled in the service layer by
		// ValidateSunsetConfig, called inside UpdateThresholds.

		updated, err := reg.DiskGroup.UpdateThresholds(group.ID, req.ThresholdPct, req.TargetPct, req.TotalBytesOverride, req.Mode, req.SunsetPct)
		if err != nil {
			// Surface validation errors (from ValidateSunsetConfig) as 400, not 500.
			return apiError(c, http.StatusBadRequest, err.Error())
		}

		return c.JSON(http.StatusOK, updated)
	})
}
