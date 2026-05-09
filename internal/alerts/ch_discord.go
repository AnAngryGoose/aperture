package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type DiscordConfig struct {
	WebhookURL string `json:"webhook_url"`
}

type DiscordSender struct{ cfg DiscordConfig }

func (d *DiscordSender) Send(ctx context.Context, n Notification) error {
	if d.cfg.WebhookURL == "" {
		return fmt.Errorf("discord: webhook_url is required")
	}
	color := SeverityColor(n.Rule.Severity, n.Resolved)
	embed := map[string]any{
		"title":       fmtNotifTitle(n),
		"description": fmtNotifBody(n),
		"color":       color,
		"fields": []map[string]any{
			{"name": "Host", "value": n.Host.Name, "inline": true},
			{"name": "Metric", "value": n.Rule.Metric, "inline": true},
			{"name": "Value", "value": fmt.Sprintf("%.4g", n.Event.Value), "inline": true},
			{"name": "Threshold", "value": fmt.Sprintf("%s %.4g", n.Rule.Op, n.Rule.Threshold), "inline": true},
			{"name": "Severity", "value": n.Rule.Severity, "inline": true},
		},
		"footer":    map[string]string{"text": "Aperture"},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	payload, _ := json.Marshal(map[string]any{"embeds": []any{embed}})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.WebhookURL, bytes.NewReader(payload))
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
		return fmt.Errorf("discord webhook returned %d", resp.StatusCode)
	}
	return nil
}
