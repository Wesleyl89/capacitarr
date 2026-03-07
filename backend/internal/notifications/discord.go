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

// DiscordSender implements Sender for Discord webhook delivery using rich embeds.
type DiscordSender struct{}

// NewDiscordSender creates a new DiscordSender.
func NewDiscordSender() *DiscordSender {
	return &DiscordSender{}
}

// discordPayload matches the Discord webhook embed structure.
type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Author      *discordAuthor `json:"author,omitempty"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields,omitempty"`
}

type discordAuthor struct {
	Name string `json:"name"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// SendDigest delivers a cycle digest notification to a Discord webhook.
func (s *DiscordSender) SendDigest(webhookURL string, digest CycleDigest) error {
	if webhookURL == "" {
		return fmt.Errorf("discord webhook URL is empty")
	}

	// Build author line: "Capacitarr v1.4.0 • auto"
	authorName := fmt.Sprintf("⚡ Capacitarr %s", digest.Version)
	if digest.ExecutionMode != "" {
		authorName += " • " + digest.ExecutionMode
	}

	desc := digestDescription(digest)

	// Append disk usage progress bar for auto mode or all-clear
	if digest.DiskUsagePct > 0 && (digest.ExecutionMode == ModeAuto || digest.Flagged == 0) {
		bar := ProgressBar(digest.DiskUsagePct, 20)
		if digest.ExecutionMode == ModeAuto && digest.Flagged > 0 {
			desc += fmt.Sprintf("\n\n`%s` **%.0f%%** → %.0f%%", bar, digest.DiskUsagePct, digest.DiskTargetPct)
		} else {
			desc += fmt.Sprintf("\n\n`%s` **%.0f%%** / %.0f%%", bar, digest.DiskUsagePct, digest.DiskThreshold)
		}
	}

	// Append version update banner
	if digest.UpdateAvailable && digest.LatestVersion != "" {
		desc += fmt.Sprintf("\n\n📦 **%s** available!", digest.LatestVersion)
	}

	embed := discordEmbed{
		Author:      &discordAuthor{Name: authorName},
		Title:       digestTitle(digest),
		Description: desc,
		Color:       digestColor(digest),
	}

	return sendDiscordPayload(webhookURL, discordPayload{Embeds: []discordEmbed{embed}})
}

// SendAlert delivers an immediate alert notification to a Discord webhook.
func (s *DiscordSender) SendAlert(webhookURL string, alert Alert) error {
	if webhookURL == "" {
		return fmt.Errorf("discord webhook URL is empty")
	}

	authorName := fmt.Sprintf("⚡ Capacitarr %s", alert.Version)

	embed := discordEmbed{
		Author:      &discordAuthor{Name: authorName},
		Title:       alert.Title,
		Description: alert.Message,
		Color:       alertColor(alert.Type),
	}

	// Add fields for alerts that carry structured data
	for k, v := range alert.Fields {
		embed.Fields = append(embed.Fields, discordField{
			Name:   k,
			Value:  v,
			Inline: true,
		})
	}

	return sendDiscordPayload(webhookURL, discordPayload{Embeds: []discordEmbed{embed}})
}

// sendDiscordPayload marshals and sends a Discord webhook payload.
func sendDiscordPayload(webhookURL string, payload discordPayload) error {
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

	resp, err := webhookHTTPClient.Do(req) //nolint:gosec // URL is from admin-configured webhook settings
	if err != nil {
		return fmt.Errorf("discord webhook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}

	return nil
}
