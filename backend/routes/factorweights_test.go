package routes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"capacitarr/internal/testutil"
)

func TestListFactorWeights(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	req := testutil.AuthenticatedRequest(t, http.MethodGet, "/api/scoring-factor-weights", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var weights []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &weights); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(weights) == 0 {
		t.Fatal("Expected at least one factor weight, got empty list")
	}

	// Verify expected fields are present on each weight
	for _, w := range weights {
		if _, ok := w["key"]; !ok {
			t.Error("Expected 'key' field on factor weight")
		}
		if _, ok := w["name"]; !ok {
			t.Error("Expected 'name' field on factor weight")
		}
		if _, ok := w["weight"]; !ok {
			t.Error("Expected 'weight' field on factor weight")
		}
		if _, ok := w["defaultWeight"]; !ok {
			t.Error("Expected 'defaultWeight' field on factor weight")
		}
	}
}

func TestUpdateFactorWeights(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	// Update watch_history weight to 7
	payload := `{"watch_history": 7, "file_size": 3}`
	req := testutil.AuthenticatedRequest(t, http.MethodPut, "/api/scoring-factor-weights", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var weights []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &weights); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify the updated weights
	for _, w := range weights {
		key, _ := w["key"].(string)
		weight, _ := w["weight"].(float64)
		switch key {
		case "watch_history":
			if int(weight) != 7 {
				t.Errorf("Expected watch_history weight 7, got %d", int(weight))
			}
		case "file_size":
			if int(weight) != 3 {
				t.Errorf("Expected file_size weight 3, got %d", int(weight))
			}
		}
	}
}

func TestUpdateFactorWeights_InvalidKey(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	payload := `{"nonexistent_factor": 5}`
	req := testutil.AuthenticatedRequest(t, http.MethodPut, "/api/scoring-factor-weights", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for unknown factor key, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateFactorWeights_OutOfRange(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	// Weight of 11 exceeds maximum of 10
	payload := `{"watch_history": 11}`
	req := testutil.AuthenticatedRequest(t, http.MethodPut, "/api/scoring-factor-weights", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for out-of-range weight, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateFactorWeights_NegativeValue(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	payload := `{"watch_history": -1}`
	req := testutil.AuthenticatedRequest(t, http.MethodPut, "/api/scoring-factor-weights", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for negative weight, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFactorWeightRoutes_RequireAuth(t *testing.T) {
	database := testutil.SetupTestDB(t)
	e := testutil.SetupTestServer(t, database)

	// GET without auth
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/scoring-factor-weights", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for unauthenticated GET, got %d", rec.Code)
	}

	// PUT without auth
	req, _ = http.NewRequestWithContext(context.Background(), http.MethodPut, "/api/scoring-factor-weights", strings.NewReader(`{"watch_history": 5}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for unauthenticated PUT, got %d", rec.Code)
	}
}
