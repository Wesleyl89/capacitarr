package routes

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"capacitarr/internal/db"
)

// RegisterAuditRoutes sets up the API endpoints for audit logs
func RegisterAuditRoutes(g *echo.Group, database *gorm.DB) {
	// Activity sparkline: audit log counts grouped by time buckets, split by flagged/deleted
	g.GET("/audit/activity", func(c echo.Context) error {
		since := c.QueryParam("since")
		if since == "" {
			since = "24h"
		}

		dur, err := parseDuration(since)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid since parameter"})
		}

		cutoff := time.Now().UTC().Add(-dur).Format("2006-01-02 15:04:05")

		// Auto-adjust bucket size based on time range
		bucketMinutes := bucketMinutesForDuration(dur)

		type BucketRow struct {
			Bucket  string `json:"bucket"`
			Flagged int    `json:"flagged"`
			Deleted int    `json:"deleted"`
		}

		var rows []BucketRow
		query := fmt.Sprintf(
			`SELECT strftime('%%Y-%%m-%%d %%H:', created_at) || printf('%%02d', (CAST(strftime('%%M', created_at) AS INTEGER) / %d) * %d) AS bucket,
			 SUM(CASE WHEN action = 'Dry-Run' THEN 1 ELSE 0 END) AS flagged,
			 SUM(CASE WHEN action = 'Deleted' THEN 1 ELSE 0 END) AS deleted
			 FROM audit_logs WHERE created_at >= ? GROUP BY bucket ORDER BY bucket`,
			bucketMinutes, bucketMinutes,
		)
		if err := database.Raw(query, cutoff).Scan(&rows).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to query activity"})
		}

		type ActivityPoint struct {
			Timestamp string `json:"timestamp"`
			Flagged   int    `json:"flagged"`
			Deleted   int    `json:"deleted"`
		}

		result := make([]ActivityPoint, len(rows))
		for i, r := range rows {
			result[i] = ActivityPoint{Timestamp: r.Bucket, Flagged: r.Flagged, Deleted: r.Deleted}
		}

		return c.JSON(http.StatusOK, result)
	})

	g.GET("/audit", func(c echo.Context) error {
		limit := 50
		if l := c.QueryParam("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		offset := 0
		if o := c.QueryParam("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				offset = parsed
			}
		}

		var logs []db.AuditLog
		var total int64

		// Get total count
		database.Model(&db.AuditLog{}).Count(&total)

		// Get paginated logs, ordered by newest first
		if err := database.Order("created_at desc").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
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

// bucketMinutesForDuration returns the grouping bucket size in minutes based on the time range.
func bucketMinutesForDuration(d time.Duration) int {
	switch {
	case d <= 1*time.Hour:
		return 5
	case d <= 6*time.Hour:
		return 15
	case d <= 24*time.Hour:
		return 15
	case d <= 7*24*time.Hour:
		return 60
	default:
		return 360 // 6 hours
	}
}

// parseDuration parses shorthand duration strings like "1h", "24h", "7d", "30d".
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	suffix := s[len(s)-1:]
	numStr := s[:len(s)-1]

	n, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid duration number: %s", numStr)
	}

	switch suffix {
	case "h":
		return time.Duration(n) * time.Hour, nil
	case "d":
		return time.Duration(n) * 24 * time.Hour, nil
	case "m":
		return time.Duration(n) * time.Minute, nil
	default:
		return 0, fmt.Errorf("unsupported duration suffix: %s", suffix)
	}
}
