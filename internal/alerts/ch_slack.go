package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type SlackConfig struct {
	WebhookURL string `json:"webhook_url"`
}

type SlackSender struct{ cfg SlackConfig }

func (s *SlackSender) Send(ctx context.Context, n Notification) error {
	if s.cfg.WebhookURL == "" {
		return fmt.Errorf("slack: webhook_url is required")
	}
	color := "warning"
	switch n.Rule.Severity {
	case "critical":
		color = "danger"
	case "info":
		color = "good"
	}
	if n.Resolved {
		color = "good"
	}
	attachment := map[string]any{
		"color": color,
		"fields": []map[string]any{
			{"title": "Host", "value": n.Host.Name, "short": true},
			{"title": "Metric", "value": n.Rule.Metric, "short": true},
			{"title": "Value", "value": fmt.Sprintf("%.4g", n.Event.Value), "short": true},
			{"title": "Threshold", "value": fmt.Sprintf("%s %.4g", n.Rule.Op, n.Rule.Threshold), "short": true},
		},
	}
	payload, _ := json.Marshal(map[string]any{
		"text":        fmtNotifTitle(n),
		"attachments": []any{attachment},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}
	return nil
}
