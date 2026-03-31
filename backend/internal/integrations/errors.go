package integrations

import (
	"errors"
	"fmt"
	"strings"
)

// NotFoundError represents a 404 HTTP response from a media server.
type NotFoundError struct {
	URL string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("not found (404): %s", e.URL)
}

// IsNotFoundError checks if an error represents an HTTP 404 from a media server.
// Handles both the typed NotFoundError and the string patterns from the HTTP
// client functions:
//   - DoAPIRequest (GET):          "unexpected status: 404"
//   - DoAPIRequestWithBody (PUT):  "unexpected status 404: <body>"
//   - DoMultipartUpload (POST):    "unexpected status 404: <body>"
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var nfe *NotFoundError
	if errors.As(err, &nfe) {
		return true
	}
	// Match both format variants: "unexpected status: 404" and "unexpected status 404:"
	msg := err.Error()
	return strings.Contains(msg, "unexpected status: 404") ||
		strings.Contains(msg, "unexpected status 404:")
}
