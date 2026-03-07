package routes

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"capacitarr/internal/db"
	"capacitarr/internal/services"
)

// RegisterRuleFieldRoutes sets up the /rule-fields and /rule-values endpoints.
// These are extracted from RegisterRuleRoutes for modularity.
func RegisterRuleFieldRoutes(protected *echo.Group, reg *services.Registry) {
	// ---------------------------------------------------------
	// RULE FIELD OPTIONS (dynamic based on integrations)
	// Accepts optional ?service_type=sonarr to filter fields.
	// Without the parameter, returns all fields (backward compat).
	// ---------------------------------------------------------
	protected.GET("/rule-fields", func(c echo.Context) error {
		serviceType := c.QueryParam("service_type")

		// Base fields available for all *arr integration types
		fields := []map[string]interface{}{
			{"field": "title", "label": "Title", "type": "string", "operators": []string{"==", "!=", "contains", "!contains"}},
			{"field": "quality", "label": "Quality Profile", "type": "string", "operators": []string{"==", "!="}},
			{"field": "tag", "label": "Tags", "type": "string", "operators": []string{"contains", "!contains"}},
			{"field": "genre", "label": "Genre", "type": "string", "operators": []string{"==", "!=", "contains", "!contains"}},
			{"field": "rating", "label": "Rating", "type": "number", "operators": []string{"==", "!=", ">", ">=", "<", "<="}},
			{"field": "sizebytes", "label": "Size (bytes)", "type": "number", "operators": []string{">", ">=", "<", "<="}},
			{"field": "timeinlibrary", "label": "Time in Library (days)", "type": "number", "operators": []string{">", ">=", "<", "<=", "in_last", "over_ago"}},
			{"field": "monitored", "label": "Monitored", "type": "boolean", "operators": []string{"=="}},
			{"field": "year", "label": "Year", "type": "number", "operators": []string{"==", "!=", ">", ">=", "<", "<="}},
			{"field": "language", "label": "Language", "type": "string", "operators": []string{"==", "!="}},
		}

		// When service_type is specified, add type-specific fields
		if serviceType == intTypeSonarr || serviceType == "" {
			// Sonarr-specific fields (TV)
			sonarrFields := []map[string]interface{}{
				{"field": "seriesstatus", "label": "Show Status", "type": "string", "operators": []string{"==", "!="}},
				{"field": "seasoncount", "label": "Season Count", "type": "number", "operators": []string{"==", "!=", ">", ">=", "<", "<="}},
				{"field": "episodecount", "label": "Episode Count", "type": "number", "operators": []string{"==", "!=", ">", ">=", "<", "<="}},
			}

			if serviceType == intTypeSonarr {
				fields = append(fields, sonarrFields...)
			} else {
				// No service_type filter: conditionally add based on enabled integrations
				var configs []db.IntegrationConfig
				configs, _ = reg.Integration.ListEnabled()
				hasTV := false
				for _, cfg := range configs {
					if cfg.Type == intTypeSonarr {
						hasTV = true
						break
					}
				}
				if hasTV {
					fields = append(fields, sonarrFields...)
				}
			}
		}

		// Enrichment fields from Tautulli / Overseerr / media servers
		// These apply to all *arr services when the enrichment service is enabled
		if serviceType == "" {
			// No filter: check which enrichment services are enabled
			var configs []db.IntegrationConfig
			configs, _ = reg.Integration.ListEnabled()
			hasTautulli := false
			hasOverseerr := false
			hasMediaServer := false
			for _, cfg := range configs {
				switch cfg.Type {
				case intTypeTautulli:
					hasTautulli = true
				case intTypeOverseerr:
					hasOverseerr = true
				case intTypePlex, intTypeJellyfin, intTypeEmby:
					hasMediaServer = true
				}
			}
			if hasTautulli || hasMediaServer {
				fields = append(fields,
					map[string]interface{}{"field": "playcount", "label": "Play Count", "type": "number", "operators": []string{"==", "!=", ">", ">=", "<", "<="}},
					map[string]interface{}{"field": "lastplayed", "label": "Last Watched", "type": "date", "operators": []string{"in_last", "over_ago", "never"}},
				)
			}
			if hasOverseerr {
				fields = append(fields,
					map[string]interface{}{"field": "requested", "label": "Is Requested", "type": "boolean", "operators": []string{"=="}},
					map[string]interface{}{"field": "requestcount", "label": "Request Count", "type": "number", "operators": []string{"==", "!=", ">", ">=", "<", "<="}},
					map[string]interface{}{"field": "requestedby", "label": "Requested By", "type": "string", "operators": []string{"==", "!=", "contains", "!contains"}},
				)
			}
			if hasMediaServer {
				fields = append(fields,
					map[string]interface{}{"field": "incollection", "label": "In Collection", "type": "boolean", "operators": []string{"=="}},
				)
			}
			if hasOverseerr && (hasTautulli || hasMediaServer) {
				fields = append(fields,
					map[string]interface{}{"field": "watchedbyreq", "label": "Watched by Requestor", "type": "boolean", "operators": []string{"=="}},
				)
			}
		} else {
			// service_type is specified — enrichment fields always available for *arr services
			arrTypes := map[string]bool{intTypeSonarr: true, intTypeRadarr: true, intTypeLidarr: true, intTypeReadarr: true}
			if arrTypes[serviceType] {
				var configs []db.IntegrationConfig
				configs, _ = reg.Integration.ListEnabled()
				hasTautulli := false
				hasOverseerr := false
				hasMediaServer := false
				for _, cfg := range configs {
					switch cfg.Type {
					case intTypeTautulli:
						hasTautulli = true
					case intTypeOverseerr:
						hasOverseerr = true
					case intTypePlex, intTypeJellyfin, intTypeEmby:
						hasMediaServer = true
					}
				}
				if hasTautulli || hasMediaServer {
					fields = append(fields,
						map[string]interface{}{"field": "playcount", "label": "Play Count", "type": "number", "operators": []string{"==", "!=", ">", ">=", "<", "<="}},
						map[string]interface{}{"field": "lastplayed", "label": "Last Watched", "type": "date", "operators": []string{"in_last", "over_ago", "never"}},
					)
				}
				if hasOverseerr {
					fields = append(fields,
						map[string]interface{}{"field": "requested", "label": "Is Requested", "type": "boolean", "operators": []string{"=="}},
						map[string]interface{}{"field": "requestcount", "label": "Request Count", "type": "number", "operators": []string{"==", "!=", ">", ">=", "<", "<="}},
						map[string]interface{}{"field": "requestedby", "label": "Requested By", "type": "string", "operators": []string{"==", "!=", "contains", "!contains"}},
					)
				}
				if hasMediaServer {
					fields = append(fields,
						map[string]interface{}{"field": "incollection", "label": "In Collection", "type": "boolean", "operators": []string{"=="}},
					)
				}
				if hasOverseerr && (hasTautulli || hasMediaServer) {
					fields = append(fields,
						map[string]interface{}{"field": "watchedbyreq", "label": "Watched by Requestor", "type": "boolean", "operators": []string{"=="}},
					)
				}
			}
		}

		// Media Type field (always available)
		fields = append(fields,
			map[string]interface{}{"field": "type", "label": "Media Type", "type": "string", "operators": []string{"==", "!="}},
		)

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
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "integration_id and action are required"})
		}

		integrationID, err := strconv.ParseUint(integrationIDStr, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid integration_id"})
		}

		result, fetchErr := reg.Integration.FetchRuleValues(uint(integrationID), action)
		if fetchErr != nil {
			errMsg := fetchErr.Error()
			switch errMsg {
			case "integration not found: record not found":
				return c.JSON(http.StatusNotFound, map[string]string{"error": "Integration not found"})
			case "unsupported integration type for rule values",
				"integration does not support rule value lookups":
				return c.JSON(http.StatusBadRequest, map[string]string{"error": errMsg})
			default:
				if strings.HasPrefix(errMsg, "unknown action:") {
					return c.JSON(http.StatusBadRequest, map[string]string{"error": errMsg})
				}
				slog.Warn("Failed to fetch rule values", "component", "api", "integrationId", integrationID, "action", action, "error", fetchErr)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch rule values"})
			}
		}

		return c.JSON(http.StatusOK, result)
	})
}
