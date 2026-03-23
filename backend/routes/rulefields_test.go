package routes_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"capacitarr/internal/db"
	"capacitarr/internal/testutil"
)

// ---------- GET /api/rule-fields ----------

func TestGetRuleFields_NoFilter(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	req := testutil.AuthenticatedRequest(t, http.MethodGet, "/api/rule-fields", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var fields []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should return at least the base fields + type field
	if len(fields) < 10 {
		t.Errorf("Expected at least 10 base fields, got %d", len(fields))
	}

	// Verify the structure of returned fields
	for _, f := range fields {
		if f["field"] == nil {
			t.Error("Expected 'field' key in field definition")
		}
		if f["label"] == nil {
			t.Error("Expected 'label' key in field definition")
		}
		if f["type"] == nil {
			t.Error("Expected 'type' key in field definition")
		}
		if f["operators"] == nil {
			t.Error("Expected 'operators' key in field definition")
		}
	}

	// Verify "type" (Media Type) field is always present
	found := false
	for _, f := range fields {
		if f["field"] == "type" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'type' (Media Type) field to always be present")
	}
}

func TestGetRuleFields_SonarrFilter(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	req := testutil.AuthenticatedRequest(t, http.MethodGet, "/api/rule-fields?service_type=sonarr", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var fields []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// When filtering by sonarr, we should get sonarr-specific fields
	fieldNames := make(map[string]bool)
	for _, f := range fields {
		if name, ok := f["field"].(string); ok {
			fieldNames[name] = true
		}
	}

	// Sonarr-specific fields
	sonarrFields := []string{"seriesstatus", "seasoncount", "episodecount"}
	for _, sf := range sonarrFields {
		if !fieldNames[sf] {
			t.Errorf("Expected sonarr-specific field %q to be present with service_type=sonarr", sf)
		}
	}
}

func TestGetRuleFields_RadarrFilter(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	req := testutil.AuthenticatedRequest(t, http.MethodGet, "/api/rule-fields?service_type=radarr", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var fields []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Radarr should NOT have sonarr-specific fields
	fieldNames := make(map[string]bool)
	for _, f := range fields {
		if name, ok := f["field"].(string); ok {
			fieldNames[name] = true
		}
	}

	sonarrOnly := []string{"seasoncount", "episodecount"}
	for _, sf := range sonarrOnly {
		if fieldNames[sf] {
			t.Errorf("Radarr filter should NOT include sonarr-specific field %q", sf)
		}
	}
}

// ---------- GET /api/rule-values ----------

func TestGetRuleValues_MissingParams(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	tests := []struct {
		name  string
		query string
	}{
		{"no params", ""},
		{"missing action", "?integration_id=1"},
		{"missing integration_id", "?action=quality"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := testutil.AuthenticatedRequest(t, http.MethodGet, "/api/rule-values"+tc.query, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("Expected 400 for %s, got %d: %s", tc.name, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGetRuleValues_InvalidIntegrationID(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	req := testutil.AuthenticatedRequest(t, http.MethodGet, "/api/rule-values?integration_id=notanumber&action=quality", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid integration_id, got %d", rec.Code)
	}
}

func TestGetRuleValues_StaticActions(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	tests := []struct {
		name       string
		action     string
		expectType string // "closed" or "free"
	}{
		{"seriesstatus", "seriesstatus", "closed"},
		{"monitored", "monitored", "closed"},
		{"requested", "requested", "closed"},
		{"type", "type", "closed"},
		{"title", "title", "free"},
		{"rating", "rating", "free"},
		{"sizebytes", "sizebytes", "free"},
		{"timeinlibrary", "timeinlibrary", "free"},
		{"year", "year", "free"},
		{"seasoncount", "seasoncount", "free"},
		{"episodecount", "episodecount", "free"},
		{"playcount", "playcount", "free"},
		{"requestcount", "requestcount", "free"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := fmt.Sprintf("/api/rule-values?integration_id=1&action=%s", tc.action)
			req := testutil.AuthenticatedRequest(t, http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
			}

			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			respType, ok := resp["type"].(string)
			if !ok {
				t.Fatal("Expected 'type' field in response")
			}
			if respType != tc.expectType {
				t.Errorf("Expected type %q, got %q", tc.expectType, respType)
			}
		})
	}
}

func TestGetRuleValues_ClosedOptionsHaveValues(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	// Test that closed-type static actions return options
	closedActions := []string{"seriesstatus", "monitored", "type"}

	for _, action := range closedActions {
		t.Run(action, func(t *testing.T) {
			path := fmt.Sprintf("/api/rule-values?integration_id=1&action=%s", action)
			req := testutil.AuthenticatedRequest(t, http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
			}

			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			options, ok := resp["options"].([]any)
			if !ok {
				t.Fatal("Expected 'options' array in closed response")
			}
			if len(options) == 0 {
				t.Errorf("Expected non-empty options for %q", action)
			}
		})
	}
}

func TestGetRuleValues_DynamicAction_IntegrationNotFound(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	// Request a dynamic action (quality) for a non-existent integration
	req := testutil.AuthenticatedRequest(t, http.MethodGet, "/api/rule-values?integration_id=99999&action=quality", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for non-existent integration, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetRuleValues_UnknownAction(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	// Seed an integration so we get past the "not found" check
	cfg := db.IntegrationConfig{
		Type:    "sonarr",
		Name:    "Test Sonarr",
		URL:     "http://localhost:8989",
		APIKey:  "test-key-12345",
		Enabled: true,
	}
	if err := database.Create(&cfg).Error; err != nil {
		t.Fatalf("Failed to seed integration: %v", err)
	}

	path := fmt.Sprintf("/api/rule-values?integration_id=%d&action=unknownfield", cfg.ID)
	req := testutil.AuthenticatedRequest(t, http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for unknown action, got %d: %s", rec.Code, rec.Body.String())
	}
}
