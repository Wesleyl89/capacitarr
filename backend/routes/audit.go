package routes

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"capacitarr/internal/db"
)

// RegisterAuditRoutes sets up the API endpoints for the audit log (history-only).
// Approval queue endpoints are in approval.go.
func RegisterAuditRoutes(g *echo.Group, database *gorm.DB) {
	// Recent audit: lightweight list of the most recent N entries (for dashboard mini-feed)
	g.GET("/audit-log/recent", func(c echo.Context) error {
		limit := 5
		if l := c.QueryParam("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		if limit > 100 {
			limit = 100
		}

		logs := make([]db.AuditLogEntry, 0, limit)
		if err := database.Order("created_at desc").Limit(limit).Find(&logs).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch recent audit logs"})
		}

		return c.JSON(http.StatusOK, logs)
	})

	// Grouped audit: show-level and season-level entries grouped into a tree
	g.GET("/audit-log/grouped", func(c echo.Context) error {
		limit := 200
		if l := c.QueryParam("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		if limit > 2000 {
			limit = 2000
		}

		logs := make([]db.AuditLogEntry, 0)
		if err := database.Order("created_at desc").Limit(limit).Find(&logs).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch audit logs"})
		}

		// Group by show title for TV content (seasons/episodes)
		type AuditGroup struct {
			ShowTitle string             `json:"showTitle"`
			Children  []db.AuditLogEntry `json:"children"`
			TotalSize int64              `json:"totalSize"`
			Action    string             `json:"action"`
			CreatedAt string             `json:"createdAt"`
		}

		groups := make(map[string]*AuditGroup)
		standalone := make([]db.AuditLogEntry, 0)
		var orderedGroupKeys []string

		for _, log := range logs {
			if log.MediaType == "season" || log.MediaType == "episode" {
				showTitle := log.MediaName
				if idx := strings.Index(log.MediaName, " - Season"); idx > 0 {
					showTitle = log.MediaName[:idx]
				} else if idx := strings.Index(log.MediaName, " - S"); idx > 0 {
					showTitle = log.MediaName[:idx]
				}

				if grp, ok := groups[showTitle]; ok {
					grp.Children = append(grp.Children, log)
					grp.TotalSize += log.SizeBytes
				} else {
					groups[showTitle] = &AuditGroup{
						ShowTitle: showTitle,
						Children:  []db.AuditLogEntry{log},
						TotalSize: log.SizeBytes,
						Action:    log.Action,
						CreatedAt: log.CreatedAt.Format(time.RFC3339),
					}
					orderedGroupKeys = append(orderedGroupKeys, showTitle)
				}
			} else {
				standalone = append(standalone, log)
			}
		}

		type GroupedResult struct {
			Type  string            `json:"type"`
			Group *AuditGroup       `json:"group,omitempty"`
			Entry *db.AuditLogEntry `json:"entry,omitempty"`
		}

		result := make([]GroupedResult, 0, len(orderedGroupKeys)+len(standalone))
		for _, key := range orderedGroupKeys {
			grp := groups[key]
			result = append(result, GroupedResult{Type: "group", Group: grp})
		}
		for _, log := range standalone {
			entry := log
			result = append(result, GroupedResult{Type: "single", Entry: &entry})
		}

		return c.JSON(http.StatusOK, result)
	})

	// Paginated audit log with search and sort
	g.GET("/audit-log", func(c echo.Context) error {
		limit := 50
		if l := c.QueryParam("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		if limit > 1000 {
			limit = 1000
		}

		offset := 0
		if o := c.QueryParam("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				offset = parsed
			}
		}

		query := database.Model(&db.AuditLogEntry{})

		if search := strings.TrimSpace(c.QueryParam("search")); search != "" {
			query = query.Where("media_name LIKE ?", "%"+search+"%")
		}

		if action := strings.TrimSpace(c.QueryParam("action")); action != "" {
			query = query.Where("action = ?", action)
		}

		allowedSortColumns := map[string]string{
			"created_at": "created_at",
			"media_name": "media_name",
			"size_bytes": "size_bytes",
			"action":     "action",
		}
		sortBy := "created_at"
		if sb := strings.TrimSpace(c.QueryParam("sort_by")); sb != "" {
			if col, ok := allowedSortColumns[sb]; ok {
				sortBy = col
			}
		}
		sortDir := "desc"
		if sd := strings.ToLower(strings.TrimSpace(c.QueryParam("sort_dir"))); sd == "asc" || sd == "desc" {
			sortDir = sd
		}
		orderClause := sortBy + " " + sortDir

		logs := make([]db.AuditLogEntry, 0)
		var total int64

		query.Count(&total)

		if err := query.Order(orderClause).Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch audit logs"})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"data":   logs,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	})

}
