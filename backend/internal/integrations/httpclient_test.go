package integrations

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestPreserveMethodRedirect_DELETE verifies that DELETE requests preserve
// their method through 301/302 redirects instead of being silently converted
// to GET. This is the core bug fix: without preserveMethodRedirect, Go's
// default CheckRedirect policy converts DELETE to GET on 301/302, causing
// *arr delete calls behind a reverse proxy to become no-op reads that still
// return 200 (the GET endpoint returns the resource JSON).
func TestPreserveMethodRedirect_DELETE(t *testing.T) {
	redirectCodes := []int{
		http.StatusMovedPermanently,  // 301
		http.StatusFound,             // 302
		http.StatusTemporaryRedirect, // 307
		http.StatusPermanentRedirect, // 308
	}

	for _, code := range redirectCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var receivedMethod atomic.Value

			// Target server that records the method it receives.
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod.Store(r.Method)
				w.WriteHeader(http.StatusOK)
			}))
			defer target.Close()

			// Redirect server that sends the test redirect code.
			redirectCode := code
			redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+"/api/v3/movie/123", redirectCode)
			}))
			defer redirect.Close()

			err := arrSimpleDelete(redirect.URL, "test-api-key", "/api/v3/movie/123")
			if err != nil {
				t.Fatalf("arrSimpleDelete returned error: %v", err)
			}

			method, ok := receivedMethod.Load().(string)
			if !ok {
				t.Fatal("Target server never received a request")
			}
			if method != "DELETE" {
				t.Errorf("Expected DELETE after %d redirect, got %s", code, method)
			}
		})
	}
}

// TestPreserveMethodRedirect_HeadersSurviveRedirect verifies that auth
// headers (X-Api-Key) are preserved through redirects. Go's default
// policy strips headers on cross-origin redirects; preserveMethodRedirect
// copies them from the original request.
func TestPreserveMethodRedirect_HeadersSurviveRedirect(t *testing.T) {
	var receivedAPIKey atomic.Value

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKey.Store(r.Header.Get("X-Api-Key"))
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/api/v3/movie/123", http.StatusMovedPermanently)
	}))
	defer redirect.Close()

	const expectedKey = "my-secret-api-key"
	err := arrSimpleDelete(redirect.URL, expectedKey, "/api/v3/movie/123")
	if err != nil {
		t.Fatalf("arrSimpleDelete returned error: %v", err)
	}

	apiKey, ok := receivedAPIKey.Load().(string)
	if !ok {
		t.Fatal("Target server never received a request")
	}
	if apiKey != expectedKey {
		t.Errorf("Expected X-Api-Key %q after redirect, got %q", expectedKey, apiKey)
	}
}

// TestPreserveMethodRedirect_StopsAfter10 verifies the redirect loop limit.
func TestPreserveMethodRedirect_StopsAfter10(t *testing.T) {
	var count atomic.Int64

	// Use a channel to pass the server URL into the handler after creation
	// (avoids using r.URL.String() which triggers semgrep open-redirect).
	var selfURL atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		// Always redirect to self — creates an infinite loop.
		target := selfURL.Load().(string) + "/api/v3/movie/123"
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}))
	defer srv.Close()
	selfURL.Store(srv.URL)

	err := arrSimpleDelete(srv.URL, "test-key", "/api/v3/movie/123")
	if err == nil {
		t.Fatal("Expected error from redirect loop, got nil")
	}

	// Go issues the initial request + up to 10 redirects = 11 total requests.
	if got := count.Load(); got > 11 {
		t.Errorf("Expected at most 11 requests (1 + 10 redirects), got %d", got)
	}
}

// TestPreserveMethodRedirect_GET verifies that GET requests still follow
// redirects normally (no regression from the method-preservation policy).
func TestPreserveMethodRedirect_GET(t *testing.T) {
	var receivedMethod atomic.Value

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod.Store(r.Method)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/api/v3/system/status", http.StatusMovedPermanently)
	}))
	defer redirect.Close()

	_, err := DoAPIRequest(redirect.URL+"/api/v3/system/status", "X-Api-Key", "test-key")
	if err != nil {
		t.Fatalf("DoAPIRequest returned error: %v", err)
	}

	method, ok := receivedMethod.Load().(string)
	if !ok {
		t.Fatal("Target server never received a request")
	}
	if method != "GET" {
		t.Errorf("Expected GET after redirect, got %s", method)
	}
}

// TestPreserveMethodRedirect_DoAPIRequestWithBody_POST verifies that POST
// requests also preserve their method through redirects.
func TestPreserveMethodRedirect_DoAPIRequestWithBody_POST(t *testing.T) {
	var receivedMethod atomic.Value

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod.Store(r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/api/v3/label", http.StatusFound)
	}))
	defer redirect.Close()

	err := DoAPIRequestWithBody("POST", redirect.URL+"/api/v3/label", []byte(`{"label":"test"}`), "X-Api-Key", "test-key")
	if err != nil {
		t.Fatalf("DoAPIRequestWithBody returned error: %v", err)
	}

	method, ok := receivedMethod.Load().(string)
	if !ok {
		t.Fatal("Target server never received a request")
	}
	if method != "POST" {
		t.Errorf("Expected POST after 302 redirect, got %s", method)
	}
}
