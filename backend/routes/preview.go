package routes

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/services"
)

// RegisterPreviewRoutes sets up the score preview endpoint.
func RegisterPreviewRoutes(protected *echo.Group, reg *services.Registry) {
	protected.GET("/preview", func(c echo.Context) error {
		result, err := reg.Engine.GetPreview()
		if err != nil {
			return apiError(c, http.StatusInternalServerError, "Failed to generate preview")
		}
		return c.JSON(http.StatusOK, result)
	})
}
