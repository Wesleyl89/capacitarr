package services

import (
	"testing"
	"time"

	"capacitarr/internal/config"
	"capacitarr/internal/db"

	"golang.org/x/crypto/bcrypt"
)

func testConfig() *config.Config {
	return &config.Config{
		JWTSecret: "test-secret-for-service-tests",
	}
}

func TestAuthService_Login_Success(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	cfg := testConfig()
	svc := NewAuthService(database, bus, cfg)

	// Seed a user
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	auth := db.AuthConfig{Username: "admin", Password: string(hash)}
	if err := database.Create(&auth).Error; err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	token, err := svc.Login("admin", "password123")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}

	// Verify event
	select {
	case evt := <-ch:
		if evt.EventType() != "login" {
			t.Errorf("expected event type 'login', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for login event")
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	cfg := testConfig()
	svc := NewAuthService(database, bus, cfg)

	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.MinCost)
	database.Create(&db.AuthConfig{Username: "admin", Password: string(hash)})

	_, err := svc.Login("admin", "wrong")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestAuthService_Login_UserNotFound(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	cfg := testConfig()
	svc := NewAuthService(database, bus, cfg)

	_, err := svc.Login("ghost", "password")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestAuthService_ChangePassword(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	cfg := testConfig()
	svc := NewAuthService(database, bus, cfg)

	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpass"), bcrypt.MinCost)
	database.Create(&db.AuthConfig{Username: "admin", Password: string(hash)})

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	if err := svc.ChangePassword("admin", "oldpass", "newpass"); err != nil {
		t.Fatalf("ChangePassword returned error: %v", err)
	}

	// Verify new password works
	_, err := svc.Login("admin", "newpass")
	if err != nil {
		t.Errorf("Login with new password failed: %v", err)
	}

	// Old password should fail
	_, err = svc.Login("admin", "oldpass")
	if err == nil {
		t.Error("expected login with old password to fail")
	}

	// Verify event
	select {
	case evt := <-ch:
		if evt.EventType() != "password_changed" {
			t.Errorf("expected event type 'password_changed', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for password_changed event")
	}
}

func TestAuthService_ChangePassword_WrongCurrent(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	cfg := testConfig()
	svc := NewAuthService(database, bus, cfg)

	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.MinCost)
	database.Create(&db.AuthConfig{Username: "admin", Password: string(hash)})

	err := svc.ChangePassword("admin", "wrong", "newpass")
	if err == nil {
		t.Fatal("expected error for wrong current password")
	}
}

func TestAuthService_ChangeUsername(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	cfg := testConfig()
	svc := NewAuthService(database, bus, cfg)

	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	database.Create(&db.AuthConfig{Username: "admin", Password: string(hash)})

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	if err := svc.ChangeUsername("admin", "newadmin", "password"); err != nil {
		t.Fatalf("ChangeUsername returned error: %v", err)
	}

	// Verify new username works for login
	_, err := svc.Login("newadmin", "password")
	if err != nil {
		t.Errorf("Login with new username failed: %v", err)
	}

	// Old username should fail
	_, err = svc.Login("admin", "password")
	if err == nil {
		t.Error("expected login with old username to fail")
	}

	// Verify event
	select {
	case evt := <-ch:
		if evt.EventType() != "username_changed" {
			t.Errorf("expected event type 'username_changed', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for username_changed event")
	}
}

func TestAuthService_ChangeUsername_WrongPassword(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	cfg := testConfig()
	svc := NewAuthService(database, bus, cfg)

	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.MinCost)
	database.Create(&db.AuthConfig{Username: "admin", Password: string(hash)})

	err := svc.ChangeUsername("admin", "newadmin", "wrong")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestAuthService_GenerateAPIKey(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	cfg := testConfig()
	svc := NewAuthService(database, bus, cfg)

	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	database.Create(&db.AuthConfig{Username: "admin", Password: string(hash)})

	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	plaintext, err := svc.GenerateAPIKey("admin")
	if err != nil {
		t.Fatalf("GenerateAPIKey returned error: %v", err)
	}

	if len(plaintext) != 64 {
		t.Errorf("expected 64-char hex key, got %d chars", len(plaintext))
	}

	// Verify stored hash and hint
	var auth db.AuthConfig
	database.Where("username = ?", "admin").First(&auth)
	if auth.APIKey == "" {
		t.Error("expected API key hash to be stored")
	}
	if auth.APIKeyHint != plaintext[60:] {
		t.Errorf("expected hint %q, got %q", plaintext[60:], auth.APIKeyHint)
	}

	// Verify event
	select {
	case evt := <-ch:
		if evt.EventType() != "api_key_generated" {
			t.Errorf("expected event type 'api_key_generated', got %q", evt.EventType())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for api_key_generated event")
	}
}

func TestAuthService_GenerateAPIKey_UserNotFound(t *testing.T) {
	database := setupTestDB(t)
	bus := newTestBus(t)
	cfg := testConfig()
	svc := NewAuthService(database, bus, cfg)

	_, err := svc.GenerateAPIKey("ghost")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}
