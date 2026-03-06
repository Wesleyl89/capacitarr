package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"capacitarr/internal/db"
	"capacitarr/internal/engine"
	"capacitarr/internal/events"
	"capacitarr/internal/integrations"
)

// DeleteJob describes a media item to be deleted.
type DeleteJob struct {
	Client     integrations.Integration
	Item       integrations.MediaItem
	Reason     string
	Score      float64
	Factors    []engine.ScoreFactor
	RunStatsID uint // Engine run stats row to increment Deleted counter
}

// DeletionService manages the background deletion worker and queue.
// It replaces the old init()-based goroutine and package-level globals.
type DeletionService struct {
	db          *gorm.DB
	bus         *events.EventBus
	auditLog    *AuditLogService
	queue       chan DeleteJob
	rateLimiter *rate.Limiter
	done        chan struct{}

	// Observable state
	currentlyDeleting atomic.Value // string
	processed         atomic.Int64
	failed            atomic.Int64
}

// NewDeletionService creates a new DeletionService.
func NewDeletionService(database *gorm.DB, bus *events.EventBus, auditLog *AuditLogService) *DeletionService {
	return &DeletionService{
		db:          database,
		bus:         bus,
		auditLog:    auditLog,
		queue:       make(chan DeleteJob, 500),
		rateLimiter: rate.NewLimiter(rate.Every(3*time.Second), 1),
		done:        make(chan struct{}),
	}
}

// Start begins the background deletion worker.
func (s *DeletionService) Start() {
	go s.worker()
}

// Stop signals the worker to finish and waits for completion.
func (s *DeletionService) Stop() {
	close(s.queue)
	<-s.done
}

// QueueDeletion enqueues a media item for background deletion.
// Returns an error if the queue is full.
func (s *DeletionService) QueueDeletion(job DeleteJob) error {
	select {
	case s.queue <- job:
		return nil
	default:
		return fmt.Errorf("deletion queue is full")
	}
}

// CurrentlyDeleting returns the name of the item currently being deleted, or empty string.
func (s *DeletionService) CurrentlyDeleting() string {
	v := s.currentlyDeleting.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

// Processed returns the total number of items processed (deleted or dry-deleted).
func (s *DeletionService) Processed() int64 {
	return s.processed.Load()
}

// Failed returns the total number of failed deletion attempts.
func (s *DeletionService) Failed() int64 {
	return s.failed.Load()
}

func (s *DeletionService) worker() {
	defer close(s.done)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Panic recovered in deletion worker", "component", "services", "panic", r)
		}
	}()

	for job := range s.queue {
		_ = s.rateLimiter.Wait(context.Background()) //nolint:errcheck
		s.processJob(job)
	}
}

func (s *DeletionService) processJob(job DeleteJob) {
	s.currentlyDeleting.Store(job.Item.Title)
	defer s.currentlyDeleting.Store("")

	// Check whether actual deletions are enabled
	var prefs db.PreferenceSet
	deletionsEnabled := false
	if err := s.db.First(&prefs, 1).Error; err == nil {
		deletionsEnabled = prefs.DeletionsEnabled
	}

	factorsJSON, _ := json.Marshal(job.Factors) //nolint:errcheck

	if !deletionsEnabled {
		// Dry-Delete: log but do not actually remove the file
		s.processed.Add(1)

		logEntry := db.AuditLogEntry{
			MediaName:    job.Item.Title,
			MediaType:    string(job.Item.Type),
			Reason:       fmt.Sprintf("Score: %.2f (%s)", job.Score, job.Reason),
			ScoreDetails: string(factorsJSON),
			Action:       db.ActionDryDelete,
			SizeBytes:    job.Item.SizeBytes,
		}
		if err := s.auditLog.Create(logEntry); err != nil {
			slog.Error("Failed to create audit log entry", "component", "services", "error", err)
		}

		s.bus.Publish(events.DeletionDryRunEvent{
			MediaName: job.Item.Title,
			MediaType: string(job.Item.Type),
			SizeBytes: job.Item.SizeBytes,
		})

		slog.Info("Dry-Delete completed", "component", "services",
			"media", job.Item.Title, "action", "Dry-Delete", "freed", job.Item.SizeBytes)
		return
	}

	// Actual deletion
	if err := job.Client.DeleteMediaItem(job.Item); err != nil {
		slog.Error("Deletion failed", "component", "services", "item", job.Item.Title, "error", err)
		s.failed.Add(1)

		s.bus.Publish(events.DeletionFailedEvent{
			MediaName:     job.Item.Title,
			MediaType:     string(job.Item.Type),
			IntegrationID: job.Item.IntegrationID,
			Error:         err.Error(),
		})
		return
	}

	s.processed.Add(1)

	// Increment deleted counter and freed bytes on the engine run stats row
	if job.RunStatsID > 0 {
		s.db.Model(&db.EngineRunStats{}).Where("id = ?", job.RunStatsID).
			UpdateColumns(map[string]interface{}{
				"deleted":     gorm.Expr("deleted + ?", 1),
				"freed_bytes": gorm.Expr("freed_bytes + ?", job.Item.SizeBytes),
			})
	}

	// Increment lifetime stats
	s.db.Model(&db.LifetimeStats{}).Where("id = 1").
		UpdateColumns(map[string]interface{}{
			"total_bytes_reclaimed": gorm.Expr("total_bytes_reclaimed + ?", job.Item.SizeBytes),
			"total_items_removed":   gorm.Expr("total_items_removed + ?", 1),
		})

	logEntry := db.AuditLogEntry{
		MediaName:    job.Item.Title,
		MediaType:    string(job.Item.Type),
		Reason:       fmt.Sprintf("Score: %.2f (%s)", job.Score, job.Reason),
		ScoreDetails: string(factorsJSON),
		Action:       db.ActionDeleted,
		SizeBytes:    job.Item.SizeBytes,
	}
	if err := s.auditLog.Create(logEntry); err != nil {
		slog.Error("Failed to create audit log entry", "component", "services", "error", err)
	}

	s.bus.Publish(events.DeletionSuccessEvent{
		MediaName:     job.Item.Title,
		MediaType:     string(job.Item.Type),
		SizeBytes:     job.Item.SizeBytes,
		IntegrationID: job.Item.IntegrationID,
	})

	slog.Info("Deletion completed", "component", "services",
		"media", job.Item.Title, "action", "Deleted", "freed", job.Item.SizeBytes)
}
