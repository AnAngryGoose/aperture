package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type GotifyConfig struct {
	URL      string `json:"url"`
	Token    string `json:"token"`
	Priority int    `json:"priority,omitempty"`
}

type GotifySender struct{ cfg GotifyConfig }

func (s *GotifySender) Send(ctx context.Context, n Notification) error {
	if s.cfg.URL == "" || s.cfg.Token == "" {
		return fmt.Errorf("gotify: url and token are required")
	}
	priority := s.cfg.Priority
	if priority == 0 {
		if n.Resolved {
			priority = 2
		} else {
			switch n.Rule.Severity {
			case "critical":
				priority = 10
			case "warning":
				priority = 5
			default:
				priority = 1
			}
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"title":    fmtNotifTitle(n),
		"message":  fmtNotifBody(n),
		"priority": priority,
	})
	base := strings.TrimRight(s.cfg.URL, "/")
	url := fmt.Sprintf("%s/message?token=%s", base, s.cfg.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
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
		return fmt.Errorf("gotify returned %d", resp.StatusCode)
	}
	return nil
}
