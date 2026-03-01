package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"capacitarr/internal/config"
	"capacitarr/internal/db"
	"capacitarr/internal/jobs"
	"capacitarr/internal/logger"
	"capacitarr/internal/poller"
	"capacitarr/routes"
)

//go:embed frontend/dist/*
var embeddedFiles embed.FS

// getSubFS strips the "frontend/dist" prefix from the embedded filesystem
func getSubFS() fs.FS {
	fsys, err := fs.Sub(embeddedFiles, "frontend/dist")
	if err != nil {
		panic(fmt.Errorf("error stripping prefix from embedded fs: %v", err))
	}
	return fsys
}

// serveEmbeddedFile reads a file from the embedded FS and writes it to the response
// with the correct Content-Type based on the file extension.
func serveEmbeddedFile(c echo.Context, fsys fs.FS, filePath string) error {
	f, err := fsys.Open(filePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	return c.Blob(http.StatusOK, contentType, data)
}

// spaHandler serves static files from the embedded filesystem and falls back
// to 200.html (Nuxt's SPA catch-all) for any path that doesn't match a real
// file. This allows the client-side Vue Router to handle navigation.
func spaHandler(fsys fs.FS, stripPrefix string) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Get the requested path and strip the prefix if configured
		reqPath := c.Request().URL.Path
		if stripPrefix != "" {
			reqPath = strings.TrimPrefix(reqPath, stripPrefix)
		}

		// Clean the path and remove leading slash for fs.Open
		reqPath = path.Clean("/" + reqPath)
		reqPath = strings.TrimPrefix(reqPath, "/")

		// If the path is empty, serve index.html
		if reqPath == "" || reqPath == "." {
			reqPath = "index.html"
		}

		// Try to open the requested file
		f, err := fsys.Open(reqPath)
		if err == nil {
			// Check if it's a directory — if so, look for index.html inside it
			stat, statErr := f.Stat()
			f.Close()
			if statErr == nil && stat.IsDir() {
				indexPath := path.Join(reqPath, "index.html")
				if idxFile, idxErr := fsys.Open(indexPath); idxErr == nil {
					idxFile.Close()
					return serveEmbeddedFile(c, fsys, indexPath)
				}
				// Directory exists but no index.html — fall through to SPA fallback
			} else if statErr == nil {
				// It's a real file, serve it from the embedded FS
				return serveEmbeddedFile(c, fsys, reqPath)
			}
		}

		// File not found — serve the SPA fallback (200.html or index.html)
		// Nuxt generates 200.html specifically for SPA catch-all hosting
		if fallback, fbErr := fsys.Open("200.html"); fbErr == nil {
			fallback.Close()
			return serveEmbeddedFile(c, fsys, "200.html")
		}
		return serveEmbeddedFile(c, fsys, "index.html")
	}
}

func main() {
	cfg := config.Load()
	logger.Init(cfg.Debug)

	slog.Info("Starting Capacitarr backend", "port", cfg.Port, "base_url", cfg.BaseURL)

	if err := db.Init(cfg); err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}

	// Initialize background jobs
	pollerStop := poller.Start(15 * time.Second) // Poll frequently to simulate active capacity ingestion
	cronScheduler := jobs.Start()

	// Initialize Echo instance
	e := echo.New()

	// Middleware
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:    true,
		LogStatus: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			slog.Info("request",
				"uri", v.URI,
				"status", v.Status,
			)
			return nil
		},
	}))
	e.Use(middleware.Recover())

	// Add CORS middleware — only if origins are configured
	if len(cfg.CORSOrigins) > 0 {
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: cfg.CORSOrigins,
			AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
		}))
	}

	// API Routing group mapping to /api/v1
	// Respect configuration's BaseURL for any proxy magic
	prefix := cfg.BaseURL
	if prefix == "/" {
		prefix = "" // allow mapping directly to routes without double slashing
	}
	// Remove trailing slash from prefix for clean route joining
	prefix = strings.TrimRight(prefix, "/")
	apiGroup := e.Group(prefix + "/api/v1")
	routes.RegisterAPIRoutes(apiGroup, db.DB, cfg)

	// Serve the embedded Nuxt static frontend with SPA fallback
	fsys := getSubFS()

	if cfg.BaseURL != "" && cfg.BaseURL != "/" {
		baseURL := strings.TrimRight(cfg.BaseURL, "/")
		uiGroup := e.Group(baseURL)
		uiGroup.GET("/*", spaHandler(fsys, baseURL))
	} else {
		e.GET("/*", spaHandler(fsys, ""))
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		slog.Info("Received shutdown signal", "signal", sig)

		// Stop background jobs
		pollerStop()
		cronScheduler.Stop()
		poller.StopWorker()

		// Shutdown HTTP server with 10s deadline
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := e.Shutdown(ctx); err != nil {
			slog.Error("Server shutdown error", "error", err)
		}
	}()

	// Start Server
	if err := e.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
		slog.Error("Server error", "error", err)
		os.Exit(1)
	}

	slog.Info("Capacitarr shut down gracefully")
}
