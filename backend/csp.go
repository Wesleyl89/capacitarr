package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"regexp"
)

// cspNoncePlaceholder is the marker injected into HTML templates at startup.
// It is replaced with a fresh cryptographic nonce on every request.
const cspNoncePlaceholder = "__CSP_NONCE__"

// noncePatterns matches inline <script> tags that need a CSP nonce injection.
// The regex handles whitespace/attribute variations that hardcoded string
// replacement would miss (e.g., <script  type="text/javascript"> with
// extra spaces, or attribute reordering by build tools).
//
// Matched patterns:
//   - <script type="text/javascript">  (theme/splash loader)
//   - <script>window.__NUXT__          (Nuxt runtime config)
//
// NOT matched (by design):
//   - <script … src="…">  (external scripts — covered by CSP 'self')
//   - <script type="application/json"> (non-executable — no script-src)
var noncePatterns = []*regexp.Regexp{
	// Pattern 1: <script type="text/javascript"> with flexible whitespace
	regexp.MustCompile(`(<script\s+type\s*=\s*"text/javascript"\s*)>`),
	// Pattern 2: <script>window.__NUXT__ (bare inline script)
	regexp.MustCompile(`(<script\s*)>(window\.__NUXT__)`),
}

// generateCSPNonce produces a cryptographically random, base64url-encoded
// nonce suitable for Content-Security-Policy script-src directives.
// Each call returns a unique 22-character string (16 random bytes).
func generateCSPNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// injectNoncePlaceholders rewrites inline <script> tags in the given HTML
// to include a nonce="__CSP_NONCE__" attribute. Returns the modified HTML
// and the number of replacements made.
func injectNoncePlaceholders(html []byte) ([]byte, int) {
	placeholder := `nonce="` + cspNoncePlaceholder + `"`
	count := 0
	result := html

	// Pattern 1: <script type="text/javascript"> → add nonce before closing >
	result = noncePatterns[0].ReplaceAllFunc(result, func(match []byte) []byte {
		count++
		groups := noncePatterns[0].FindSubmatch(match)
		out := make([]byte, 0, len(groups[1])+len(placeholder)+4)
		out = append(out, groups[1]...)
		out = append(out, ' ')
		out = append(out, []byte(placeholder)...)
		out = append(out, '>')
		return out
	})

	// Pattern 2: <script>window.__NUXT__ → add nonce before >
	result = noncePatterns[1].ReplaceAllFunc(result, func(match []byte) []byte {
		count++
		groups := noncePatterns[1].FindSubmatch(match)
		out := make([]byte, 0, len(groups[1])+len(placeholder)+len(groups[2])+4)
		out = append(out, groups[1]...)
		out = append(out, ' ')
		out = append(out, []byte(placeholder)...)
		out = append(out, '>')
		out = append(out, groups[2]...)
		return out
	})

	return result, count
}

// applyNonce replaces all __CSP_NONCE__ placeholders in the HTML template
// with the given nonce value. This is called once per request.
func applyNonce(template []byte, nonce string) []byte {
	return bytes.ReplaceAll(template, []byte(cspNoncePlaceholder), []byte(nonce))
}
