package notifications

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlackSender_SendDigest_Format(t *testing.T) {
	var received slackPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("failed to decode Slack payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := NewSlackSender()
	digest := CycleDigest{
		ExecutionMode: ModeDryRun,
		Evaluated:     97,
		Flagged:       12,
		Deleted:       0,
		Failed:        0,
		FreedBytes:    5368709120, // 5 GB
		DurationMs:    2500,
		DiskUsagePct:  92.0,
		DiskThreshold: 85.0,
		DiskTargetPct: 75.0,
		Version:       "v1.0.0",
	}

	err := sender.SendDigest(server.URL, digest)
	if err != nil {
		t.Fatalf("SendDigest failed: %v", err)
	}

	if len(received.Blocks) == 0 {
		t.Fatal("expected at least 1 Slack block")
	}
}

func TestSlackSender_SendAlert_Format(t *testing.T) {
	var received slackPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("failed to decode Slack payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := NewSlackSender()
	alert := Alert{
		Type:    AlertModeChanged,
		Title:   "Mode Changed",
		Message: "Firefly mode changed from dry-run to auto",
		Fields: map[string]string{
			"Old Mode": "dry-run",
			"New Mode": "auto",
		},
		Version: "v1.0.0",
	}

	err := sender.SendAlert(server.URL, alert)
	if err != nil {
		t.Fatalf("SendAlert failed: %v", err)
	}

	if len(received.Blocks) == 0 {
		t.Fatal("expected at least 1 Slack block")
	}
}
