package integrations

import "strings"

// arrBaseClient provides shared functionality for all *arr integration clients
// (Sonarr, Radarr, Lidarr, Readarr). It holds the common URL, API key, and
// API prefix fields, and implements the shared Connectable, DiskReporter, and
// RuleValueFetcher methods that differ only in their API prefix.
//
// Individual *arr clients embed arrBaseClient and add only their unique
// methods (GetMediaItems, DeleteMediaItem) and any overrides (e.g., Lidarr
// returns nil for GetLanguages because Lidarr has no language endpoint).
type arrBaseClient struct {
	URL       string
	APIKey    string `json:"-"`
	APIPrefix string // e.g., "/api/v3" for Sonarr/Radarr, "/api/v1" for Lidarr/Readarr
}

// newArrBaseClient creates a new arrBaseClient with the given URL, API key,
// and API prefix. The URL is right-trimmed of trailing slashes.
func newArrBaseClient(url, apiKey, apiPrefix string) arrBaseClient {
	return arrBaseClient{
		URL:       strings.TrimRight(url, "/"),
		APIKey:    apiKey,
		APIPrefix: apiPrefix,
	}
}

// doRequest performs an authenticated API request to the *arr server.
func (b *arrBaseClient) doRequest(endpoint string) ([]byte, error) {
	return DoAPIRequest(b.URL+endpoint, "X-Api-Key", b.APIKey)
}

// TestConnection verifies the *arr server is reachable and the API key is valid.
func (b *arrBaseClient) TestConnection() error {
	_, err := b.doRequest(b.APIPrefix + "/system/status")
	return err
}

// GetDiskSpace returns disk usage information reported by the *arr server.
func (b *arrBaseClient) GetDiskSpace() ([]DiskSpace, error) {
	return arrFetchDiskSpace(b.doRequest, b.APIPrefix)
}

// GetRootFolders returns the configured root folder paths from the *arr server.
func (b *arrBaseClient) GetRootFolders() ([]string, error) {
	return arrFetchRootFolders(b.doRequest, b.APIPrefix)
}

// GetQualityProfiles returns all quality profiles configured in the *arr server.
func (b *arrBaseClient) GetQualityProfiles() ([]NameValue, error) {
	return arrFetchQualityProfiles(b.doRequest, b.APIPrefix)
}

// GetTags returns all tags configured in the *arr server.
func (b *arrBaseClient) GetTags() ([]NameValue, error) {
	return arrFetchTags(b.doRequest, b.APIPrefix)
}

// GetLanguages returns all languages configured in the *arr server.
// Lidarr overrides this to return nil (no language endpoint).
func (b *arrBaseClient) GetLanguages() ([]NameValue, error) {
	return arrFetchLanguages(b.doRequest, b.APIPrefix)
}
