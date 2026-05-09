package alerts

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
)

type NtfyConfig struct {
	URL      string `json:"url"`
	Topic    string `json:"topic"`
	Token    string `json:"token,omitempty"`
	Priority string `json:"priority,omitempty"`
}

type NtfySender struct{ cfg NtfyConfig }

func (s *NtfySender) Send(ctx context.Context, n Notification) error {
	if s.cfg.URL == "" || s.cfg.Topic == "" {
		return fmt.Errorf("ntfy: url and topic are required")
	}
	base := strings.TrimRight(s.cfg.URL, "/")
	url := fmt.Sprintf("%s/%s", base, s.cfg.Topic)

	priority := s.cfg.Priority
	if priority == "" {
		if n.Resolved {
			priority = "low"
		} else {
			switch n.Rule.Severity {
			case "critical":
				priority = "urgent"
			case "warning":
				priority = "high"
			default:
				priority = "default"
			}
		}
	}

	tags := "rotating_light"
	if n.Resolved {
		tags = "white_check_mark"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(fmtNotifBody(n)))
	if err != nil {
		return err
	}
	req.Header.Set("Title", fmtNotifTitle(n))
	req.Header.Set("Priority", priority)
	req.Header.Set("Tags", tags)
	if s.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned %d", resp.StatusCode)
	}
	return nil
}
