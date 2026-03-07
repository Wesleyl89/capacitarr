package routes

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/services"
)

// RegisterDiskGroupRoutes registers disk group management endpoints on the protected group.
func RegisterDiskGroupRoutes(g *echo.Group, reg *services.Registry) {
	g.GET("/disk-groups", func(c echo.Context) error {
		groups, err := reg.Settings.ListDiskGroups()
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

		group, err := reg.Settings.GetDiskGroup(uint(idNum))
		if err != nil {
			return apiError(c, http.StatusNotFound, "Disk group not found")
		}

		var req struct {
			ThresholdPct float64 `json:"thresholdPct"`
			TargetPct    float64 `json:"targetPct"`
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

		updated, err := reg.Settings.UpdateThresholds(group.ID, req.ThresholdPct, req.TargetPct)
		if err != nil {
			return apiError(c, http.StatusInternalServerError, "Failed to update disk group")
		}

		return c.JSON(http.StatusOK, updated)
	})
}
