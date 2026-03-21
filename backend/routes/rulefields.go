package routes

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/services"
)

// RegisterRuleFieldRoutes sets up the /rule-fields and /rule-values endpoints.
// These are extracted from RegisterRuleRoutes for modularity.
func RegisterRuleFieldRoutes(protected *echo.Group, reg *services.Registry) {
	// ---------------------------------------------------------
	// RULE FIELD OPTIONS (dynamic based on integrations)
	// Accepts optional ?service_type=sonarr to filter fields.
	// Without the parameter, returns all fields (backward compat).
	// Delegates to RulesService.GetFieldDefinitions() for the
	// actual field list construction.
	// ---------------------------------------------------------
	protected.GET("/rule-fields", func(c echo.Context) error {
		serviceType := c.QueryParam("service_type")
		enrichment := reg.Integration.DetectEnrichment()
		fields := reg.Rules.GetFieldDefinitions(serviceType, enrichment)
		return c.JSON(http.StatusOK, fields)
	})

	// ---------------------------------------------------------
	// RULE VALUES — Autocomplete for rule value input
	// GET /api/v1/rule-values?integration_id=X&action=Y
	// Delegates to IntegrationService.FetchRuleValues() which handles
	// caching, external API calls, and static field metadata.
	// ---------------------------------------------------------
	protected.GET("/rule-values", func(c echo.Context) error {
		integrationIDStr := c.QueryParam("integration_id")
		action := c.QueryParam("action")
		if integrationIDStr == "" || action == "" {
			return apiError(c, http.StatusBadRequest, "integration_id and action are required")
		}

		integrationID, err := strconv.ParseUint(integrationIDStr, 10, 64)
		if err != nil {
			return apiError(c, http.StatusBadRequest, "Invalid integration_id")
		}

		result, fetchErr := reg.Integration.FetchRuleValues(uint(integrationID), action)
		if fetchErr != nil {
			switch {
			case errors.Is(fetchErr, services.ErrNotFound):
				return apiError(c, http.StatusNotFound, "Integration not found")
			case errors.Is(fetchErr, services.ErrUnsupportedIntegrationType),
				errors.Is(fetchErr, services.ErrIntegrationNoRuleValues):
				return apiError(c, http.StatusBadRequest, fetchErr.Error())
			case errors.Is(fetchErr, services.ErrUnknownAction):
				return apiError(c, http.StatusBadRequest, fetchErr.Error())
			default:
				slog.Warn("Failed to fetch rule values", "component", "api", "integrationId", integrationID, "action", action, "error", fetchErr)
				return apiError(c, http.StatusInternalServerError, "Failed to fetch rule values")
			}
		}

		return c.JSON(http.StatusOK, result)
	})
}
