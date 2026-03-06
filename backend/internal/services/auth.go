package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"capacitarr/internal/config"
	"capacitarr/internal/db"
	"capacitarr/internal/events"
)

// BcryptCost is the cost factor for bcrypt password hashing across all auth
// operations. The Go default is 10; we use 12 for stronger brute-force
// resistance while keeping hashing under ~250ms on typical hardware.
const BcryptCost = 12

// AuthService manages authentication, password changes, username changes, and API keys.
type AuthService struct {
	db  *gorm.DB
	bus *events.EventBus
	cfg *config.Config
}

// NewAuthService creates a new AuthService.
func NewAuthService(database *gorm.DB, bus *events.EventBus, cfg *config.Config) *AuthService {
	return &AuthService{db: database, bus: bus, cfg: cfg}
}

// Login verifies credentials and returns a JWT token on success.
func (s *AuthService) Login(username, password string) (string, error) {
	var auth db.AuthConfig
	if err := s.db.Where("username = ?", username).First(&auth).Error; err != nil {
		return "", fmt.Errorf("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(auth.Password), []byte(password)); err != nil {
		return "", fmt.Errorf("invalid credentials")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": username,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	s.bus.Publish(events.LoginEvent{Username: username})
	return tokenString, nil
}

// ChangePassword verifies the current password and sets a new one.
func (s *AuthService) ChangePassword(username, currentPwd, newPwd string) error {
	var auth db.AuthConfig
	if err := s.db.Where("username = ?", username).First(&auth).Error; err != nil {
		return fmt.Errorf("user not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(auth.Password), []byte(currentPwd)); err != nil {
		return fmt.Errorf("current password is incorrect")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPwd), BcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := s.db.Model(&auth).Update("password", string(hash)).Error; err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	s.bus.Publish(events.PasswordChangedEvent{Username: username})
	return nil
}

// ChangeUsername verifies the password and updates the username.
func (s *AuthService) ChangeUsername(currentUser, newUsername, password string) error {
	var auth db.AuthConfig
	if err := s.db.Where("username = ?", currentUser).First(&auth).Error; err != nil {
		return fmt.Errorf("user not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(auth.Password), []byte(password)); err != nil {
		return fmt.Errorf("password is incorrect")
	}

	if err := s.db.Model(&auth).Update("username", newUsername).Error; err != nil {
		return fmt.Errorf("failed to update username: %w", err)
	}

	s.bus.Publish(events.UsernameChangedEvent{
		OldUsername: currentUser,
		NewUsername: newUsername,
	})
	return nil
}

// GenerateAPIKey creates a new API key, stores its SHA-256 hash and hint,
// and returns the plaintext key (shown only once).
func (s *AuthService) GenerateAPIKey(username string) (string, error) {
	var auth db.AuthConfig
	if err := s.db.Where("username = ?", username).First(&auth).Error; err != nil {
		return "", fmt.Errorf("user not found")
	}

	// Generate 32 random bytes → 64 hex characters
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}
	plaintext := hex.EncodeToString(keyBytes)

	// Store SHA-256 hash and hint (last 4 chars)
	hashBytes := sha256.Sum256([]byte(plaintext))
	hashHex := "sha256:" + hex.EncodeToString(hashBytes[:])
	hint := plaintext[len(plaintext)-4:]

	if err := s.db.Model(&auth).Updates(map[string]interface{}{
		"api_key":      hashHex,
		"api_key_hint": hint,
	}).Error; err != nil {
		return "", fmt.Errorf("failed to store API key: %w", err)
	}

	s.bus.Publish(events.APIKeyGeneratedEvent{
		Username: username,
		Hint:     hint,
	})

	return plaintext, nil
}
