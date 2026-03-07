package routes

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/services"
)

// RegisterVersionRoutes sets up the version check endpoints on the protected group.
func RegisterVersionRoutes(g *echo.Group, reg *services.Registry) {
	g.GET("/version/check", func(c echo.Context) error {
		result, err := reg.Version.CheckForUpdate()
		if err != nil {
			return apiError(c, http.StatusInternalServerError, "Failed to check for updates")
		}
		return c.JSON(http.StatusOK, result)
	})

	g.POST("/version/check", func(c echo.Context) error {
		result, err := reg.Version.ForceCheck()
		if err != nil {
			return apiError(c, http.StatusInternalServerError, "Failed to check for updates")
		}
		return c.JSON(http.StatusOK, result)
	})
}
