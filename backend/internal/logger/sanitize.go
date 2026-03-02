package logger

import "net/url"

// SanitizeURL strips query parameters and fragments from a URL string to
// prevent accidental logging of API keys, tokens, or other sensitive data
// embedded in query strings. Returns only scheme://host/path.
func SanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "[invalid-url]"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
