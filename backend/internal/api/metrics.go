package api

import (
	"encoding/json"
	"net/http"

	"github.com/capacitarr/capacitarr/backend/internal/config"
	"github.com/capacitarr/capacitarr/backend/internal/db"
)

func MetricsHistoryHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolution := r.URL.Query().Get("resolution")
		if resolution == "" {
			resolution = "raw"
		}

		var history []db.LibraryHistory
		if err := db.DB.Where("resolution = ?", resolution).Order("timestamp asc").Limit(1000).Find(&history).Error; err != nil {
			http.Error(w, "Error fetching metrics", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "success", "data": history})
	}
}
