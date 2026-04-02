// Package notifications dispatches alerts via Discord and Apprise channels.
package notifications

import (
	"encoding/json"
	"fmt"
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
	Name    string `json:"name"`
	IconURL string `json:"icon_url,omitempty"`
}

// capacitarrIconURL is intentionally empty — Discord gracefully ignores
// empty/missing icon_url fields. Populate once a hosted logo is available.
const capacitarrIconURL = ""

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// SendDigest delivers a cycle digest notification to a Discord webhook.
// The level parameter controls which group sections are included based
// on the channel's notification tier.
func (s *DiscordSender) SendDigest(config SenderConfig, digest CycleDigest, level NotificationTier) error {
	webhookURL := config.WebhookURL
	if webhookURL == "" {
		return fmt.Errorf("discord webhook URL is empty")
	}

	groups := filterGroups(digest.Groups, level)
	if len(groups) == 0 {
		return nil // nothing to show at this tier
	}

	// Build author line: "Capacitarr v1.4.0"
	authorName := fmt.Sprintf("⚡ Capacitarr %s", digest.Version)

	// Build per-group sections in the embed description
	desc := ""
	for i, g := range groups {
		if i > 0 {
			desc += "\n"
		}
		desc += fmt.Sprintf("──── %s · %s ────\n", g.MountPath, g.Mode)
		desc += fmt.Sprintf("%s %s\n", groupIcon(g.Mode), groupDescription(g))
		if g.DiskUsagePct > 0 {
			bar := ProgressBar(g.DiskUsagePct, 20)
			if g.Mode == ModeAuto && g.Deleted > 0 {
				desc += fmt.Sprintf("`%s` **%.0f%%** → %.0f%%\n", bar, g.DiskUsagePct, g.DiskTargetPct)
			} else {
				desc += fmt.Sprintf("`%s` **%.0f%%** / %.0f%%\n", bar, g.DiskUsagePct, g.DiskThreshold)
			}
		}
	}

	// Append duration footer
	durSec := float64(digest.DurationMs) / 1000.0
	desc += fmt.Sprintf("\n⏱️ %.1fs", durSec)

	// Append version update banner
	if digest.UpdateAvailable && digest.LatestVersion != "" {
		desc += fmt.Sprintf(" · 📦 **%s** available!", digest.LatestVersion)
	}

	embed := discordEmbed{
		Author:      &discordAuthor{Name: authorName, IconURL: capacitarrIconURL},
		Title:       digestTitle(digest),
		Description: desc,
		Color:       digestColor(groups),
	}

	return sendDiscordPayload(webhookURL, discordPayload{Embeds: []discordEmbed{embed}})
}

// SendAlert delivers an immediate alert notification to a Discord webhook.
func (s *DiscordSender) SendAlert(config SenderConfig, alert Alert) error {
	webhookURL := config.WebhookURL
	if webhookURL == "" {
		return fmt.Errorf("discord webhook URL is empty")
	}

	// Include the trigger label so recipients know what action produced this alert
	authorName := fmt.Sprintf("⚡ Capacitarr %s • %s", alert.Version, TriggerLabel(alert.Type))

	embed := discordEmbed{
		Author:      &discordAuthor{Name: authorName, IconURL: capacitarrIconURL},
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

// sendDiscordPayload marshals and sends a Discord webhook payload with retry.
func sendDiscordPayload(webhookURL string, payload discordPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}
	return sendWebhookRequest(webhookURL, body)
}
