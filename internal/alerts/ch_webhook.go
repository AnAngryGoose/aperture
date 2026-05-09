package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type WebhookSender struct{ cfg WebhookConfig }

func (s *WebhookSender) Send(ctx context.Context, n Notification) error {
	if s.cfg.URL == "" {
		return fmt.Errorf("webhook: url is required")
	}
	eventType := "alert_fired"
	if n.Resolved {
		eventType = "alert_resolved"
	}
	var resolvedAt *string
	if n.ResolvedAt != nil {
		t := n.ResolvedAt.UTC().Format(time.RFC3339)
		resolvedAt = &t
	}
	body, _ := json.Marshal(map[string]any{
		"type": eventType,
		"host": n.Host,
		"rule": map[string]any{
			"id":        n.Rule.ID,
			"metric":    n.Rule.Metric,
			"op":        n.Rule.Op,
			"threshold": n.Rule.Threshold,
			"severity":  n.Rule.Severity,
		},
		"event": map[string]any{
			"id":       n.Event.ID,
			"fired_at": n.Event.FiredAt.UTC().Format(time.RFC3339),
			"value":    n.Event.Value,
		},
		"resolved_at": resolvedAt,
	})
	method := s.cfg.Method
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, s.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range s.cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
