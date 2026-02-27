package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/capacitarr/capacitarr/backend/internal/config"
	"github.com/capacitarr/capacitarr/backend/internal/db"
	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserContextKey contextKey = "user"

// RequireAuth validates JWT tokens from cookies or Authorization header, or checks for a valid X-API-Key.
func RequireAuth(cfg *config.Config, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Check API Key
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "" {
			var user db.AuthConfig
			if err := db.DB.Where("api_key = ?", apiKey).First(&user).Error; err == nil {
				ctx := context.WithValue(r.Context(), UserContextKey, user.Username)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		var tokenStr string

		// 2. Try to get token from cookie (for Web UI)
		if cookie, err := r.Cookie("jwt"); err == nil {
			tokenStr = cookie.Value
		}

		// 3. Try to get from Authorization header (Bearer)
		if tokenStr == "" {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if tokenStr == "" {
			http.Error(w, "Unauthorized: missing credentials", http.StatusUnauthorized)
			return
		}

		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Unauthorized: invalid credentials", http.StatusUnauthorized)
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			username, _ := claims["sub"].(string)
			ctx := context.WithValue(r.Context(), UserContextKey, username)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		http.Error(w, "Unauthorized: invalid claims", http.StatusUnauthorized)
	}
}
