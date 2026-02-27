package poller

import (
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/capacitarr/capacitarr/backend/internal/db"
)

// Start begins the continuous polling simulator.
func Start(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		var angle float64
		// Simulate a 1 TB drive
		totalCapacity := int64(1000 * 1024 * 1024 * 1024)
		for range ticker.C {
			// Simulate used capacity with a sine wave plus some random noise
			angle += 0.05
			noise := rand.Float64() * 50 * 1024 * 1024 * 1024
			baseUsed := int64(500 * 1024 * 1024 * 1024)
			sineOffset := int64(math.Sin(angle) * 200 * 1024 * 1024 * 1024)
			usedCapacity := baseUsed + sineOffset + int64(noise)

			record := db.LibraryHistory{
				Timestamp:     time.Now(),
				TotalCapacity: totalCapacity,
				UsedCapacity:  usedCapacity,
				Resolution:    "raw",
			}
			if err := db.DB.Create(&record).Error; err != nil {
				slog.Error("Failed to save polled capacity", "error", err)
			}
		}
	}()
}
