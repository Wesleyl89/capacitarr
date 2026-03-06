package routes

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"capacitarr/internal/db"
	"capacitarr/internal/services"
)

// bcryptCost references the shared bcrypt cost factor from the services package.
const bcryptCost = services.BcryptCost

// LoginRequest holds the JSON body of login requests.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RegisterAuthRoutes sets up login, logout, password change, first-user
// bootstrap, and API key management endpoints.
func RegisterAuthRoutes(public *echo.Group, protected *echo.Group, reg *services.Registry) {
	database := reg.DB
	cfg := reg.Cfg

	// Auth status — public endpoint for first-login UX detection
	public.GET("/auth/status", func(c echo.Context) error {
		var count int64
		database.Model(&db.AuthConfig{}).Count(&count)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"initialized": count > 0,
		})
	})

	// Rate-limit login endpoint: 10 attempts per IP per 15-minute window
	loginRL := newLoginRateLimiter(10, 15*time.Minute)

	public.POST("/auth/login", func(c echo.Context) error {
		var req LoginRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		if req.Username == "" || req.Password == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Username and password are required"})
		}

		var user db.AuthConfig
		if err := database.Where("username = ?", req.Username).First(&user).Error; err != nil {
			// If no user exists in DB at all, bootstrap the first user.
			// Use a transaction to prevent a race condition where two concurrent
			// requests both see count==0 and create duplicate users. The unique
			// index on username provides an additional safety net.
			var bootstrapped bool
			txErr := database.Transaction(func(tx *gorm.DB) error {
				var count int64
				if err := tx.Model(&db.AuthConfig{}).Count(&count).Error; err != nil {
					return err
				}
				if count > 0 {
					return nil // Another request already created the first user
				}
				hashed, hashErr := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
				if hashErr != nil {
					return hashErr
				}
				user = db.AuthConfig{Username: req.Username, Password: string(hashed)}
				if err := tx.Create(&user).Error; err != nil {
					return err
				}
				bootstrapped = true
				slog.Info("First user bootstrapped", "component", "auth", "username", req.Username)
				return nil
			})
			if txErr != nil {
				slog.Error("First-user bootstrap failed", "component", "auth", "operation", "bootstrap_user", "error", txErr)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create initial user"})
			}
			if !bootstrapped {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid credentials"})
			}
		}

		// Delegate credential check + JWT generation + event publishing to AuthService
		tokenString, loginErr := reg.Auth.Login(req.Username, req.Password)
		if loginErr != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid credentials"})
		}

		// Set HttpOnly JWT cookie for secure transport
		c.SetCookie(&http.Cookie{
			Name:     "jwt",
			Value:    tokenString,
			Expires:  time.Now().Add(24 * time.Hour),
			HttpOnly: true,
			Secure:   cfg.SecureCookies,
			Path:     cfg.BaseURL,
			SameSite: http.SameSiteLaxMode,
		})

		// Set a non-HttpOnly cookie so the SPA can detect auth state
		c.SetCookie(&http.Cookie{
			Name:     "authenticated",
			Value:    "true",
			Expires:  time.Now().Add(24 * time.Hour),
			HttpOnly: false,
			Secure:   cfg.SecureCookies,
			Path:     cfg.BaseURL,
			SameSite: http.SameSiteLaxMode,
		})

		return c.JSON(http.StatusOK, map[string]string{"message": "success", "token": tokenString})
	}, LoginRateLimit(loginRL))

	// Password change — delegates to AuthService
	protected.PUT("/auth/password", func(c echo.Context) error {
		username, ok := c.Get("user").(string)
		if !ok || username == "" {
			return echo.ErrUnauthorized
		}

		var req struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		if req.CurrentPassword == "" || req.NewPassword == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Current and new password are required"})
		}
		if len(req.NewPassword) < 8 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "New password must be at least 8 characters"})
		}

		if err := reg.Auth.ChangePassword(username, req.CurrentPassword, req.NewPassword); err != nil {
			// Distinguish between "wrong password" and other errors
			if err.Error() == "current password is incorrect" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "Password changed successfully"})
	})

	// Username change — delegates to AuthService
	protected.PUT("/auth/username", func(c echo.Context) error {
		currentUser, ok := c.Get("user").(string)
		if !ok || currentUser == "" {
			return echo.ErrUnauthorized
		}

		var req struct {
			NewUsername     string `json:"newUsername"`
			CurrentPassword string `json:"currentPassword"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		if req.NewUsername == "" || req.CurrentPassword == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "New username and current password are required"})
		}
		if len(req.NewUsername) < 3 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Username must be at least 3 characters"})
		}

		// Check if new username is already taken
		var existing db.AuthConfig
		if err := database.Where("username = ?", req.NewUsername).First(&existing).Error; err == nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": "Username already taken"})
		}

		if err := reg.Auth.ChangeUsername(currentUser, req.NewUsername, req.CurrentPassword); err != nil {
			if err.Error() == "password is incorrect" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Current password is incorrect"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "Username changed successfully"})
	})

	// Generate API key — delegates to AuthService
	protected.POST("/auth/apikey", func(c echo.Context) error {
		username, ok := c.Get("user").(string)
		if !ok || username == "" {
			return echo.ErrUnauthorized
		}

		plaintext, err := reg.Auth.GenerateAPIKey(username)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]string{"api_key": plaintext})
	})

	// Check API key status
	protected.GET("/auth/apikey", func(c echo.Context) error {
		username, ok := c.Get("user").(string)
		if !ok || username == "" {
			return echo.ErrUnauthorized
		}

		var user db.AuthConfig
		if err := database.Where("username = ?", username).First(&user).Error; err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
		}

		// Never return the actual API key (it's hashed in the DB). Instead
		// return whether a key has been generated and the last 4 chars hint
		// so the UI can show a recognisable masked version.
		hasKey := user.APIKey != ""
		return c.JSON(http.StatusOK, map[string]interface{}{
			"has_key": hasKey,
			"hint":    user.APIKeyHint,
		})
	})
}
