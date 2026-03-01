package integrations

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// sharedHTTPClient is a package-level HTTP client with a 30-second timeout.
var sharedHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

// DoAPIRequest creates a GET request to the given URL, sets the specified header,
// executes with the shared client, checks for 401/non-200, and reads the body.
func DoAPIRequest(url, headerKey, headerValue string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if headerKey != "" {
		req.Header.Set(headerKey, headerValue)
	}

	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("unauthorized: invalid API key or token")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
