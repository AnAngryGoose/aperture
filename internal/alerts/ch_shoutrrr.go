// Single dispatcher for channels that route through the Shoutrrr library.
// Replaces the prior hand-rolled ch_{discord,slack,ntfy,gotify}.go files —
// each was ~50-80 LOC of HTTP plumbing duplicating what shoutrrr does once.
//
// Two paths exist here:
//   1. Native "shoutrrr" channel type: user pastes a raw Shoutrrr URL
//      (https://containrrr.dev/shoutrrr/services). One channel type, 16+
//      services unlocked (Telegram, Matrix, Signal, Pushover, etc.) for free.
//   2. Legacy types ("discord", "slack", "ntfy", "gotify"): their existing
//      config schemas are translated to a Shoutrrr URL at send time. No DB
//      migration needed — rows persist with their original `type` and
//      `config`, but the wire dispatch is now via Shoutrrr.
//
// The legacy "webhook" type stays on its own simple sender (ch_webhook.go)
// because its JSON-POST body shape doesn't map cleanly onto Shoutrrr's
// generic:// service.
package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/containrrr/shoutrrr"
	"github.com/containrrr/shoutrrr/pkg/router"
	stypes "github.com/containrrr/shoutrrr/pkg/types"
)

// ShoutrrrConfig is the user-supplied config for the native "shoutrrr"
// channel type. The URL field accepts any of Shoutrrr's service schemas
// (discord://, slack://, telegram://, matrix://, pushover://, ntfy://,
// gotify://, gchat://, teams://, generic://, mattermost://, rocketchat://,
// pushbullet://, opsgenie://, ifttt://, join://, lark://, wecom://,
// zulip://, bark://).
type ShoutrrrConfig struct {
	URL string `json:"url"`
}

// ShoutrrrSender dispatches via the Shoutrrr router. The router is built
// lazily on the first Send so a misconfigured URL surfaces as a send-time
// error (validated by buildSender) rather than a panic at module init.
type ShoutrrrSender struct {
	URL string
}

func (s *ShoutrrrSender) Send(ctx context.Context, n Notification) error {
	if s.URL == "" {
		return fmt.Errorf("shoutrrr: url is required")
	}
	r, err := router.New(nil, s.URL)
	if err != nil {
		return fmt.Errorf("shoutrrr: build router: %w", err)
	}
	// Shoutrrr has a per-router Timeout; the dispatcher's 15s timeout in
	// Notifier.Dispatch is the outer cap. Match the inner timeout slightly
	// shorter so the outer context cancel logs cleanly when the network
	// stalls (rather than leaving the goroutine hung on a socket).
	r.Timeout = 12 * time.Second
	params := buildShoutrrrParams(n)
	// Shoutrrr's Send returns []error (one per recipient URL). With a
	// single-URL router we collect the first non-nil error if any.
	errs := r.Send(formatShoutrrrMessage(n), params)
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	// Best-effort respect of ctx — if it's already done, surface that.
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// formatShoutrrrMessage is the single-line message body. Most services
// (Discord, Slack, etc.) get richer formatting via Shoutrrr's per-service
// params (title, color), set by buildShoutrrrParams. This is what shows up
// for services without those params (Telegram, plain text channels).
func formatShoutrrrMessage(n Notification) string {
	return fmtNotifTitle(n) + "\n" + fmtNotifBody(n)
}

// buildShoutrrrParams populates per-service formatting hints — Shoutrrr
// reads keys like "title", "color", "priority" depending on the service.
// Unknown keys are ignored, so we can set them all once.
func buildShoutrrrParams(n Notification) *stypes.Params {
	p := stypes.Params{}
	p["title"] = fmtNotifTitle(n)
	// Color as a hex string ("#RRGGBB") for Discord/Slack/Mattermost; the
	// underlying SeverityColor returns an int.
	p["color"] = fmt.Sprintf("#%06x", SeverityColor(n.Rule.Severity, n.Resolved))
	// ntfy / pushover / gotify priority mapping. Critical → high/urgent.
	switch n.Rule.Severity {
	case "critical":
		p["priority"] = "high"
	case "warning":
		p["priority"] = "default"
	default:
		p["priority"] = "low"
	}
	if n.Resolved {
		p["priority"] = "low"
	}
	return &p
}

// ToShoutrrrURL translates a stored channel (legacy type + config) into a
// Shoutrrr service URL. Returns the channel's own URL unchanged for the
// native "shoutrrr" type. Used by buildSender so the rest of the alert
// pipeline can route everything through one code path.
func ToShoutrrrURL(ch channelLike) (string, error) {
	switch ch.GetType() {
	case "shoutrrr":
		var cfg ShoutrrrConfig
		if err := json.Unmarshal(ch.GetConfig(), &cfg); err != nil {
			return "", fmt.Errorf("shoutrrr config: %w", err)
		}
		if cfg.URL == "" {
			return "", fmt.Errorf("shoutrrr: url is required")
		}
		return cfg.URL, nil

	case "discord":
		var cfg DiscordConfig
		if err := json.Unmarshal(ch.GetConfig(), &cfg); err != nil {
			return "", fmt.Errorf("discord config: %w", err)
		}
		return discordWebhookToShoutrrr(cfg.WebhookURL)

	case "slack":
		var cfg SlackConfig
		if err := json.Unmarshal(ch.GetConfig(), &cfg); err != nil {
			return "", fmt.Errorf("slack config: %w", err)
		}
		return slackWebhookToShoutrrr(cfg.WebhookURL)

	case "ntfy":
		var cfg NtfyConfig
		if err := json.Unmarshal(ch.GetConfig(), &cfg); err != nil {
			return "", fmt.Errorf("ntfy config: %w", err)
		}
		return ntfyConfigToShoutrrr(cfg)

	case "gotify":
		var cfg GotifyConfig
		if err := json.Unmarshal(ch.GetConfig(), &cfg); err != nil {
			return "", fmt.Errorf("gotify config: %w", err)
		}
		return gotifyConfigToShoutrrr(cfg)
	}
	return "", fmt.Errorf("channel type %q is not Shoutrrr-routable", ch.GetType())
}

// channelLike is a minimal interface so ToShoutrrrURL works with either a
// types.AlertChannel or a smaller test fixture without depending on the
// concrete type.
type channelLike interface {
	GetType() string
	GetConfig() []byte
}

// discordWebhookToShoutrrr converts a Discord webhook URL
// (https://discord.com/api/webhooks/<id>/<token>) to Shoutrrr's
// `discord://<token>@<id>` schema.
func discordWebhookToShoutrrr(webhookURL string) (string, error) {
	if webhookURL == "" {
		return "", fmt.Errorf("discord: webhook_url is required")
	}
	u, err := url.Parse(webhookURL)
	if err != nil {
		return "", fmt.Errorf("discord: parse webhook URL: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Path should be: api/webhooks/<id>/<token>
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "webhooks" {
		return "", fmt.Errorf("discord: not a webhook URL (path=%q)", u.Path)
	}
	id, token := parts[2], parts[3]
	return fmt.Sprintf("discord://%s@%s", token, id), nil
}

// slackWebhookToShoutrrr converts a Slack incoming-webhook URL
// (https://hooks.slack.com/services/T0/B0/X0) to Shoutrrr's
// `slack://hook:T0-B0-X0@webhook` schema.
func slackWebhookToShoutrrr(webhookURL string) (string, error) {
	if webhookURL == "" {
		return "", fmt.Errorf("slack: webhook_url is required")
	}
	u, err := url.Parse(webhookURL)
	if err != nil {
		return "", fmt.Errorf("slack: parse webhook URL: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Path: services/<T>/<B>/<X>
	if len(parts) < 4 || parts[0] != "services" {
		return "", fmt.Errorf("slack: not a services URL (path=%q)", u.Path)
	}
	t, b, x := parts[1], parts[2], parts[3]
	return fmt.Sprintf("slack://hook:%s-%s-%s@webhook", t, b, x), nil
}

// ntfyConfigToShoutrrr builds Shoutrrr's ntfy URL from the legacy
// {url, topic, token, priority} config.
func ntfyConfigToShoutrrr(cfg NtfyConfig) (string, error) {
	if cfg.URL == "" || cfg.Topic == "" {
		return "", fmt.Errorf("ntfy: url and topic are required")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return "", fmt.Errorf("ntfy: parse URL: %w", err)
	}
	scheme := u.Scheme
	if scheme != "http" && scheme != "https" {
		scheme = "https"
	}
	// Shoutrrr ntfy URL: ntfy://[token@]<host>/<topic>?scheme=<http|https>
	// Token is passed as the basic-auth user portion in shoutrrr.
	out := "ntfy://"
	if cfg.Token != "" {
		out += url.QueryEscape(cfg.Token) + "@"
	}
	out += u.Host + "/" + strings.TrimLeft(cfg.Topic, "/")
	out += "?scheme=" + scheme
	if cfg.Priority != "" {
		out += "&priority=" + cfg.Priority
	}
	return out, nil
}

// gotifyConfigToShoutrrr builds Shoutrrr's gotify URL from {url, token}.
// Shoutrrr gotify URL format: gotify://<host>/<token>?disableTLS=true|false
func gotifyConfigToShoutrrr(cfg GotifyConfig) (string, error) {
	if cfg.URL == "" || cfg.Token == "" {
		return "", fmt.Errorf("gotify: url and token are required")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return "", fmt.Errorf("gotify: parse URL: %w", err)
	}
	out := "gotify://" + u.Host + "/" + cfg.Token
	if u.Scheme == "http" {
		out += "?disableTLS=true"
	}
	return out, nil
}

// Compile-time check: the function reference is referenced so unused-import
// linters don't strip shoutrrr if a future refactor stops using Send.
var _ = shoutrrr.Send
