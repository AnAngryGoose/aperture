package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aperture/aperture/internal/store"
	"github.com/aperture/aperture/internal/types"
)

// Notification is the data passed to every channel sender.
type Notification struct {
	Event    types.AlertEvent
	Rule     types.AlertRule
	Host     types.Host
	Resolved bool
	// ResolvedAt is set when Resolved == true.
	ResolvedAt *time.Time
}

// Sender delivers a single notification to one external channel.
type Sender interface {
	Send(ctx context.Context, n Notification) error
}

// Notifier loads enabled channels from the store and dispatches
// notifications when alerts fire or resolve.
type Notifier struct {
	store *store.Store
	log   *slog.Logger
}

func NewNotifier(st *store.Store, log *slog.Logger) *Notifier {
	if log == nil {
		log = slog.Default()
	}
	return &Notifier{store: st, log: log}
}

// Dispatch is called (in a goroutine) by the Evaluator when an alert fires
// or resolves. It loads enabled channels, filters by severity and
// notify_resolve, then sends in parallel goroutines.
func (n *Notifier) Dispatch(ctx context.Context, event types.AlertEvent, rule types.AlertRule, resolved bool) {
	host, err := n.store.GetHost(ctx, event.HostID)
	if err != nil || host == nil {
		h := types.Host{ID: event.HostID, Name: event.HostID}
		host = &h
	}

	channels, err := n.store.ListEnabledChannels(ctx)
	if err != nil {
		n.log.Error("notifier: list channels", "err", err)
		return
	}

	ruleSev := SeverityLevel(rule.Severity)
	notif := Notification{
		Event:    event,
		Rule:     rule,
		Host:     *host,
		Resolved: resolved,
	}
	if resolved {
		now := time.Now().UTC()
		notif.ResolvedAt = &now
	}

	for _, ch := range channels {
		if SeverityLevel(ch.MinSeverity) > ruleSev {
			continue
		}
		if resolved && !ch.NotifyResolve {
			continue
		}
		sender, err := buildSender(ch)
		if err != nil {
			n.log.Error("notifier: build sender", "channel_id", ch.ID, "type", ch.Type, "err", err)
			continue
		}
		go func(s Sender, cid int64, cname string) {
			if err := s.Send(ctx, notif); err != nil {
				n.log.Error("notifier: send", "channel_id", cid, "name", cname, "err", err)
			} else {
				n.log.Info("notifier: sent", "channel_id", cid, "name", cname, "resolved", resolved)
			}
		}(sender, ch.ID, ch.Name)
	}
}

// SeverityLevel converts a severity string to an integer for comparison.
// Higher = more severe.
func SeverityLevel(s string) int {
	switch s {
	case "critical":
		return 2
	case "warning":
		return 1
	default: // "info" or empty
		return 0
	}
}

// SeverityColor returns an embed color integer for each severity.
func SeverityColor(severity string, resolved bool) int {
	if resolved {
		return 0x2ecc71 // green
	}
	switch severity {
	case "critical":
		return 0xe74c3c // red
	case "warning":
		return 0xf39c12 // orange
	default:
		return 0x3498db // blue
	}
}

// BuildSender constructs the appropriate Sender for a channel.
// Exported so the API test-channel handler can call it directly.
func BuildSender(ch types.AlertChannel) (Sender, error) {
	return buildSender(ch)
}

// buildSender constructs the appropriate Sender for a channel.
func buildSender(ch types.AlertChannel) (Sender, error) {
	switch ch.Type {
	case "discord":
		var cfg DiscordConfig
		if err := json.Unmarshal(ch.Config, &cfg); err != nil {
			return nil, fmt.Errorf("discord config: %w", err)
		}
		return &DiscordSender{cfg: cfg}, nil
	case "slack":
		var cfg SlackConfig
		if err := json.Unmarshal(ch.Config, &cfg); err != nil {
			return nil, fmt.Errorf("slack config: %w", err)
		}
		return &SlackSender{cfg: cfg}, nil
	case "ntfy":
		var cfg NtfyConfig
		if err := json.Unmarshal(ch.Config, &cfg); err != nil {
			return nil, fmt.Errorf("ntfy config: %w", err)
		}
		return &NtfySender{cfg: cfg}, nil
	case "gotify":
		var cfg GotifyConfig
		if err := json.Unmarshal(ch.Config, &cfg); err != nil {
			return nil, fmt.Errorf("gotify config: %w", err)
		}
		return &GotifySender{cfg: cfg}, nil
	case "webhook":
		var cfg WebhookConfig
		if err := json.Unmarshal(ch.Config, &cfg); err != nil {
			return nil, fmt.Errorf("webhook config: %w", err)
		}
		return &WebhookSender{cfg: cfg}, nil
	}
	return nil, fmt.Errorf("unknown channel type %q", ch.Type)
}

// fmtNotifTitle builds a short one-line title for a notification.
func fmtNotifTitle(n Notification) string {
	state := "Firing"
	if n.Resolved {
		state = "Resolved"
	}
	return fmt.Sprintf("Alert %s — %s %s %.4g on %s",
		state, n.Rule.Metric, n.Rule.Op, n.Rule.Threshold, n.Host.Name)
}

// fmtNotifBody builds a short description line.
func fmtNotifBody(n Notification) string {
	if n.Resolved {
		return fmt.Sprintf("%s · %s returned to normal (was %.4g)",
			n.Host.Name, n.Rule.Metric, n.Event.Value)
	}
	return fmt.Sprintf("%s · %s = %.4g (threshold %s %.4g)",
		n.Host.Name, n.Rule.Metric, n.Event.Value, n.Rule.Op, n.Rule.Threshold)
}
