package integrations

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// EmbyClient provides access to the Emby API for watch history data.
// Emby's API is structurally similar to Jellyfin (Jellyfin forked from Emby),
// using the same X-Emby-Token auth header and similar endpoint patterns.
type EmbyClient struct {
	URL    string
	APIKey string
}

func NewEmbyClient(url, apiKey string) *EmbyClient {
	return &EmbyClient{
		URL:    strings.TrimRight(url, "/"),
		APIKey: apiKey,
	}
}

func (e *EmbyClient) doRequest(endpoint string) ([]byte, error) {
	fullURL := e.URL + endpoint
	return DoAPIRequest(fullURL, "X-Emby-Token", e.APIKey)
}

// TestConnection verifies the Emby URL and API key by calling /System/Info
func (e *EmbyClient) TestConnection() error {
	body, err := e.doRequest("/System/Info")
	if err != nil {
		return err
	}
	var info struct {
		ServerName string `json:"ServerName"`
		Version    string `json:"Version"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return fmt.Errorf("failed to parse Emby system info: %w", err)
	}
	if info.ServerName == "" && info.Version == "" {
		return fmt.Errorf("unexpected Emby response — no server name or version")
	}
	return nil
}

// EmbyWatchData contains watch history for a media item from Emby
type EmbyWatchData struct {
	PlayCount      int
	LastPlayedDate time.Time
	Played         bool
}

// GetWatchHistory fetches play history for a specific item by its Emby ID.
func (e *EmbyClient) GetWatchHistory(embyID, userID string) (*EmbyWatchData, error) {
	endpoint := fmt.Sprintf("/Users/%s/Items/%s", userID, embyID)
	body, err := e.doRequest(endpoint)
	if err != nil {
		return nil, err
	}

	var item struct {
		UserData struct {
			PlayCount      int    `json:"PlayCount"`
			LastPlayedDate string `json:"LastPlayedDate"`
			Played         bool   `json:"Played"`
		} `json:"UserData"`
	}

	if err := json.Unmarshal(body, &item); err != nil {
		return nil, fmt.Errorf("failed to parse Emby item: %w", err)
	}

	data := &EmbyWatchData{
		PlayCount: item.UserData.PlayCount,
		Played:    item.UserData.Played,
	}

	if item.UserData.LastPlayedDate != "" {
		data.LastPlayedDate, _ = time.Parse(time.RFC3339, item.UserData.LastPlayedDate)
	}

	return data, nil
}

// GetAdminUserID returns the first admin user's ID for making user-specific queries.
func (e *EmbyClient) GetAdminUserID() (string, error) {
	body, err := e.doRequest("/Users")
	if err != nil {
		return "", err
	}

	var users []struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Policy struct {
			IsAdministrator bool `json:"IsAdministrator"`
		} `json:"Policy"`
	}

	if err := json.Unmarshal(body, &users); err != nil {
		return "", fmt.Errorf("failed to parse Emby users: %w", err)
	}

	for _, u := range users {
		if u.Policy.IsAdministrator {
			return u.ID, nil
		}
	}

	if len(users) > 0 {
		return users[0].ID, nil
	}

	return "", fmt.Errorf("no Emby users found")
}
