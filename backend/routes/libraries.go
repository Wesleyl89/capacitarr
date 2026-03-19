package routes

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/db"
	"capacitarr/internal/services"
)

// RegisterLibraryRoutes registers CRUD endpoints for the Library entity.
func RegisterLibraryRoutes(g *echo.Group, reg *services.Registry) {
	g.GET("/libraries", listLibrariesHandler(reg))
	g.GET("/libraries/:id", getLibraryHandler(reg))
	g.POST("/libraries", createLibraryHandler(reg))
	g.PUT("/libraries/:id", updateLibraryHandler(reg))
	g.DELETE("/libraries/:id", deleteLibraryHandler(reg))
}

func listLibrariesHandler(reg *services.Registry) echo.HandlerFunc {
	return func(c echo.Context) error {
		libraries, err := reg.Library.List()
		if err != nil {
			return apiError(c, http.StatusInternalServerError, "failed to list libraries")
		}
		return c.JSON(http.StatusOK, libraries)
	}
}

func getLibraryHandler(reg *services.Registry) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "invalid library ID")
		}
		library, err := reg.Library.GetByID(uint(id))
		if err != nil {
			return apiError(c, http.StatusNotFound, "library not found")
		}
		return c.JSON(http.StatusOK, library)
	}
}

type createLibraryRequest struct {
	Name         string   `json:"name"`
	DiskGroupID  *uint    `json:"diskGroupId"`
	ThresholdPct *float64 `json:"thresholdPct"`
	TargetPct    *float64 `json:"targetPct"`
}

func createLibraryHandler(reg *services.Registry) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req createLibraryRequest
		if err := c.Bind(&req); err != nil {
			return apiError(c, http.StatusBadRequest, "invalid request body")
		}
		library := &db.Library{
			Name:         req.Name,
			DiskGroupID:  req.DiskGroupID,
			ThresholdPct: req.ThresholdPct,
			TargetPct:    req.TargetPct,
		}
		if err := reg.Library.Create(library); err != nil {
			return apiError(c, http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusCreated, library)
	}
}

func updateLibraryHandler(reg *services.Registry) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "invalid library ID")
		}
		existing, err := reg.Library.GetByID(uint(id))
		if err != nil {
			return apiError(c, http.StatusNotFound, "library not found")
		}
		var req createLibraryRequest
		if err := c.Bind(&req); err != nil {
			return apiError(c, http.StatusBadRequest, "invalid request body")
		}
		existing.Name = req.Name
		existing.DiskGroupID = req.DiskGroupID
		existing.ThresholdPct = req.ThresholdPct
		existing.TargetPct = req.TargetPct

		if err := reg.Library.Update(existing); err != nil {
			return apiError(c, http.StatusBadRequest, err.Error())
		}
		return c.JSON(http.StatusOK, existing)
	}
}

func deleteLibraryHandler(reg *services.Registry) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "invalid library ID")
		}
		if err := reg.Library.Delete(uint(id)); err != nil {
			return apiError(c, http.StatusNotFound, err.Error())
		}
		return c.NoContent(http.StatusNoContent)
	}
}
