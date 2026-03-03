package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

const (
	plexPinURL         = "https://plex.tv/api/v2/pins"
	plexClientID       = "capacitarr"
	plexProduct        = "Capacitarr"
	plexPinRequestBody = "strong=true"
	plexAPITimeout     = 15 * time.Second
)

// plexPinResponse represents the relevant fields from the Plex PIN API.
type plexPinResponse struct {
	ID        int    `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
}

// RegisterPlexAuthRoutes adds Plex OAuth PIN authentication endpoints.
func RegisterPlexAuthRoutes(protected *echo.Group) {
	plexAuth := protected.Group("/integrations/plex/auth")

	plexAuth.POST("/pin", handleCreatePlexPin)
	plexAuth.GET("/pin/:id", handleCheckPlexPin)
}

// handleCreatePlexPin creates a new Plex PIN for the OAuth flow.
func handleCreatePlexPin(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), plexAPITimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, plexPinURL, strings.NewReader(plexPinRequestBody))
	if err != nil {
		slog.Error("Failed to create Plex PIN request", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create PIN request"})
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Plex-Client-Identifier", plexClientID)
	req.Header.Set("X-Plex-Product", plexProduct)

	client := &http.Client{Timeout: plexAPITimeout}
	resp, err := client.Do(req) //nolint:gosec // G704: URL is a well-known Plex API endpoint
	if err != nil {
		slog.Error("Plex PIN API request failed", "error", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to reach Plex API"})
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Failed to read Plex PIN response", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read Plex response"})
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		slog.Error("Plex PIN API returned unexpected status", "status", resp.StatusCode, "body", string(body)) //nolint:gosec // G706: resp values are from Plex API, not user input
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("Plex API returned status %d", resp.StatusCode),
		})
	}

	var pin plexPinResponse
	if err := json.Unmarshal(body, &pin); err != nil {
		slog.Error("Failed to parse Plex PIN response", "error", err, "body", string(body))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to parse Plex response"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":   pin.ID,
		"code": pin.Code,
	})
}

// handleCheckPlexPin polls a Plex PIN to check if it has been claimed.
func handleCheckPlexPin(c echo.Context) error {
	pinID := c.Param("id")
	if pinID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "PIN ID is required"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), plexAPITimeout)
	defer cancel()

	url := fmt.Sprintf("%s/%s", plexPinURL, pinID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Error("Failed to create Plex PIN check request", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create check request"})
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Client-Identifier", plexClientID)
	req.Header.Set("X-Plex-Product", plexProduct)

	client := &http.Client{Timeout: plexAPITimeout}
	resp, err := client.Do(req) //nolint:gosec // G704: URL is a well-known Plex API endpoint
	if err != nil {
		slog.Error("Plex PIN check request failed", "error", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to reach Plex API"})
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Failed to read Plex PIN check response", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read Plex response"})
	}

	if resp.StatusCode == http.StatusNotFound {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "PIN not found or expired"})
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("Plex PIN check returned unexpected status", "status", resp.StatusCode, "body", string(body)) //nolint:gosec // G706: resp values are from Plex API, not user input
		return c.JSON(http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("Plex API returned status %d", resp.StatusCode),
		})
	}

	var pin plexPinResponse
	if err := json.Unmarshal(body, &pin); err != nil {
		slog.Error("Failed to parse Plex PIN check response", "error", err, "body", string(body))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to parse Plex response"})
	}

	if pin.AuthToken != "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"claimed":   true,
			"authToken": pin.AuthToken,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"claimed": false,
	})
}
