package routes

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/services"
)

// RegisterDeletionQueueRoutes registers deletion queue management endpoints.
func RegisterDeletionQueueRoutes(g *echo.Group, reg *services.Registry) {
	g.GET("/deletion-queue", handleListDeletionQueue(reg))
	g.DELETE("/deletion-queue", handleCancelDeletion(reg))
}

func handleListDeletionQueue(reg *services.Registry) echo.HandlerFunc {
	return func(c echo.Context) error {
		items := reg.Deletion.ListQueuedItems()
		return c.JSON(http.StatusOK, items)
	}
}

func handleCancelDeletion(reg *services.Registry) echo.HandlerFunc {
	return func(c echo.Context) error {
		mediaName := c.QueryParam("mediaName")
		mediaType := c.QueryParam("mediaType")

		if mediaName == "" || mediaType == "" {
			return apiError(c, http.StatusBadRequest, "mediaName and mediaType query parameters are required")
		}

		cancelled := reg.Deletion.CancelDeletion(mediaName, mediaType)
		if !cancelled {
			return apiError(c, http.StatusNotFound, "item not found in deletion queue")
		}

		return c.JSON(http.StatusOK, map[string]bool{"cancelled": true})
	}
}
