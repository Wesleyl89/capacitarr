package config

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	Port        string
	BaseURL     string
	Database    string
	Debug       bool
	JWTSecret   string
	CORSOrigins []string
	SecureCookies bool
}

func Load() *Config {
	debug := strings.ToLower(os.Getenv("DEBUG")) == "true"

	port := os.Getenv("PORT")
	if port == "" {
		port = "2187"
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "/"
	}
	if !strings.HasPrefix(baseURL, "/") {
		baseURL = "/" + baseURL
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL = baseURL + "/"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "capacitarr.db"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		if debug {
			jwtSecret = "development_secret_do_not_use_in_production"
			slog.Warn("Using default JWT secret — this is only acceptable in debug mode")
		} else {
			// Generate a random secret for this run and warn the user
			bytes := make([]byte, 32)
			if _, err := rand.Read(bytes); err != nil {
				slog.Error("Failed to generate random JWT secret", "error", err)
				os.Exit(1)
			}
			jwtSecret = hex.EncodeToString(bytes)
			slog.Warn("No JWT_SECRET set — generated a random secret for this session. Sessions will not persist across restarts. Set JWT_SECRET environment variable for persistent sessions.")
		}
	}

	// CORS origins configuration
	corsOrigins := []string{}
	corsEnv := os.Getenv("CORS_ORIGINS")
	if corsEnv != "" {
		for _, origin := range strings.Split(corsEnv, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				corsOrigins = append(corsOrigins, origin)
			}
		}
	} else if debug {
		corsOrigins = []string{"*"}
	}
	// If no CORS origins and not debug, leave empty (same-origin only)

	secureCookies := strings.ToLower(os.Getenv("SECURE_COOKIES")) == "true"

	return &Config{
		Port:          port,
		BaseURL:       baseURL,
		Database:      dbPath,
		Debug:         debug,
		JWTSecret:     jwtSecret,
		CORSOrigins:   corsOrigins,
		SecureCookies: secureCookies,
	}
}
