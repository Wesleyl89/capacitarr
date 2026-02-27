package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/capacitarr/capacitarr/backend/internal/api"
	"github.com/capacitarr/capacitarr/backend/internal/config"
	"github.com/capacitarr/capacitarr/backend/internal/db"
	"github.com/capacitarr/capacitarr/backend/internal/jobs"
	"github.com/capacitarr/capacitarr/backend/internal/logger"
	"github.com/capacitarr/capacitarr/backend/internal/poller"
)

func main() {
	cfg := config.Load()
	logger.Init(cfg.Debug)

	slog.Info("Starting Capacitarr backend", "port", cfg.Port, "base_url", cfg.BaseURL)

	if err := db.Init(cfg); err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}

	mux := api.SetupRouter(cfg)

	// Start background processes
	poller.Start(15 * time.Second) // Poll frequently to simulate active capacity ingestion
	jobs.Start()

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
