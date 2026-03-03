package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// slackEmoji returns a contextual emoji prefix for the event title.
func slackEmoji(eventType string) string {
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

// Slack Block Kit payload types.
type slackPayload struct {
	Blocks []slackBlock `json:"blocks"`
}

type slackBlock struct {
	Type   string      `json:"type"`
	Text   *slackText  `json:"text,omitempty"`
	Fields []slackText `json:"fields,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SendSlack posts a notification event to a Slack webhook URL using Block Kit format.
func SendSlack(webhookURL string, event NotificationEvent) error {
	if webhookURL == "" {
		return fmt.Errorf("slack webhook URL is empty")
	}

	blocks := []slackBlock{
		{
			Type: "header",
			Text: &slackText{
				Type: "plain_text",
				Text: slackEmoji(event.Type) + " " + event.Title,
			},
		},
		{
			Type: "section",
			Text: &slackText{
				Type: "mrkdwn",
				Text: event.Message,
			},
		},
	}

	// Add fields block if there are key-value pairs
	if len(event.Fields) > 0 {
		var fields []slackText
		for k, v := range event.Fields {
			fields = append(fields, slackText{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*%s:*\n%s", k, v),
			})
		}
		blocks = append(blocks, slackBlock{
			Type:   "section",
			Fields: fields,
		})
	}

	payload := slackPayload{Blocks: blocks}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := webhookHTTPClient.Do(req) //nolint:gosec // G704: URL is from admin-configured webhook settings
	if err != nil {
		return fmt.Errorf("slack webhook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}

	return nil
}
