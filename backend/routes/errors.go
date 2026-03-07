package routes

import "github.com/labstack/echo/v4"

// apiError returns a JSON error response with a consistent envelope shape.
// All error responses across the API use {"error": "message"} for client consistency.
func apiError(c echo.Context, status int, message string) error {
	return c.JSON(status, map[string]string{"error": message})
}
