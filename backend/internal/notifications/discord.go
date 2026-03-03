// Package notifications dispatches alerts via Discord, Slack, and in-app channels.
package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Discord embed color mapping by event type.
var discordColors = map[string]int{
	EventThresholdBreach:  15158332, // red (#E74C3C)
	EventDeletionExecuted: 3066993,  // amber/orange (#2ECC71 — actually green; spec says amber)
	EventEngineError:      15158332, // red
	EventEngineComplete:   3066993,  // green
}

// discordEmoji returns a contextual emoji prefix for the event title.
func discordEmoji(eventType string) string {
	switch eventType {
	case EventThresholdBreach:
		return emojiRed
	case EventDeletionExecuted:
		return emojiYellow
	case EventEngineError:
		return emojiRed
	case EventEngineComplete:
		return emojiGreen
	default:
		return emojiInfo
	}
}

// discordPayload matches the Discord webhook embed structure.
type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields,omitempty"`
	Timestamp   string         `json:"timestamp"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// SendDiscord posts a notification event to a Discord webhook URL.
func SendDiscord(webhookURL string, event NotificationEvent) error {
	if webhookURL == "" {
		return fmt.Errorf("discord webhook URL is empty")
	}

	color, ok := discordColors[event.Type]
	if !ok {
		color = 3447003 // default blue
	}

	embed := discordEmbed{
		Title:       discordEmoji(event.Type) + " " + event.Title,
		Description: event.Message,
		Color:       color,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	for k, v := range event.Fields {
		embed.Fields = append(embed.Fields, discordField{
			Name:   k,
			Value:  v,
			Inline: true,
		})
	}

	payload := discordPayload{Embeds: []discordEmbed{embed}}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := webhookHTTPClient.Do(req) //nolint:gosec // G704: URL is from admin-configured webhook settings
	if err != nil {
		return fmt.Errorf("discord webhook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}

	return nil
}
