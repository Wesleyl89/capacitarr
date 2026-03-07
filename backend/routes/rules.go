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

// RegisterRuleRoutes sets up the endpoints for managing custom rules, preferences,
// and score preview.
func RegisterRuleRoutes(protected *echo.Group, reg *services.Registry) {
	// Delegate preference and preview routes to their own files
	RegisterPreferenceRoutes(protected, reg)
	RegisterPreviewRoutes(protected, reg)

	// Delegate rule-field and rule-value routes to rulefields.go
	registerRuleFieldRoutes(protected, reg)

	// Delegate import/export routes to rules_portability.go
	RegisterRulePortabilityRoutes(protected, reg)

	// ---------------------------------------------------------
	// CUSTOM RULES (protection/targeting)
	// ---------------------------------------------------------
	protected.GET("/custom-rules", func(c echo.Context) error {
		rules, err := reg.Rules.List()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch custom rules"})
		}
		return c.JSON(http.StatusOK, rules)
	})

	protected.PUT("/custom-rules/reorder", func(c echo.Context) error {
		var payload struct {
			Order []uint `json:"order"`
		}
		if err := c.Bind(&payload); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		}
		if len(payload.Order) == 0 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Order array must not be empty"})
		}

		if err := reg.Rules.Reorder(payload.Order); err != nil {
			slog.Error("Failed to reorder rules", "component", "api", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to reorder rules"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	protected.PUT("/custom-rules/:id", func(c echo.Context) error {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		}

		var updated db.CustomRule
		if err := c.Bind(&updated); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		}

		rule, err := reg.Rules.Update(uint(id), updated)
		if err != nil {
			if errors.Is(err, errors.New("rule not found")) || err.Error() == "rule not found: record not found" {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "Rule not found"})
			}
			slog.Error("Failed to update custom rule", "component", "api", "operation", "update_rule", "id", id, "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update rule"})
		}
		return c.JSON(http.StatusOK, rule)
	})

	protected.POST("/custom-rules", func(c echo.Context) error {
		var newRule db.CustomRule
		if err := c.Bind(&newRule); err != nil {
			slog.Debug("Failed to bind rule payload", "component", "api", "operation", "create_rule", "error", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload: " + err.Error()})
		}

		rule, err := reg.Rules.Create(newRule)
		if err != nil {
			// Validation errors from the service are returned as 400
			if isValidationError(err) {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			slog.Error("Failed to create custom rule", "component", "api", "operation", "create_rule", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create rule"})
		}
		return c.JSON(http.StatusCreated, rule)
	})

	protected.DELETE("/custom-rules/:id", func(c echo.Context) error {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		}

		if err := reg.Rules.Delete(uint(id)); err != nil {
			if err.Error() == "rule not found: record not found" {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "Rule not found"})
			}
			slog.Error("Failed to delete custom rule", "component", "api", "operation", "delete_rule", "id", id, "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete rule"})
		}
		return c.NoContent(http.StatusNoContent)
	})
}

// isValidationError returns true if err represents a user-input validation
// failure (missing fields, invalid effect, etc.) rather than an internal error.
func isValidationError(err error) bool {
	msg := err.Error()
	switch msg {
	case "field, operator, and value are required",
		"effect field is required":
		return true
	}
	// Effect enum validation
	if len(msg) > 20 && msg[:20] == "effect must be one o" {
		return true
	}
	return false
}
