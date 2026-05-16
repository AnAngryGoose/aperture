package alerts

import (
	"encoding/json"
	"strings"
	"testing"
)

type fakeChannel struct {
	t   string
	cfg []byte
}

func (f fakeChannel) GetType() string   { return f.t }
func (f fakeChannel) GetConfig() []byte { return f.cfg }

func newCh(t string, cfg any) fakeChannel {
	b, _ := json.Marshal(cfg)
	return fakeChannel{t: t, cfg: b}
}

func TestDiscordWebhookToShoutrrr(t *testing.T) {
	got, err := discordWebhookToShoutrrr("https://discord.com/api/webhooks/123456/abcDEF")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "discord://abcDEF@123456" {
		t.Errorf("got %q", got)
	}
	if _, err := discordWebhookToShoutrrr(""); err == nil {
		t.Errorf("expected error on empty URL")
	}
	if _, err := discordWebhookToShoutrrr("https://example.com/not/discord"); err == nil {
		t.Errorf("expected error on non-webhook URL")
	}
}

func TestSlackWebhookToShoutrrr(t *testing.T) {
	got, err := slackWebhookToShoutrrr("https://hooks.slack.com/services/T01ABC/B01DEF/XYZ123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "slack://hook:T01ABC-B01DEF-XYZ123@webhook" {
		t.Errorf("got %q", got)
	}
}

func TestNtfyConfigToShoutrrr(t *testing.T) {
	got, err := ntfyConfigToShoutrrr(NtfyConfig{URL: "https://ntfy.sh", Topic: "alerts", Priority: "high"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "ntfy://ntfy.sh/alerts") || !strings.Contains(got, "scheme=https") || !strings.Contains(got, "priority=high") {
		t.Errorf("unexpected URL: %q", got)
	}

	// With token.
	got, _ = ntfyConfigToShoutrrr(NtfyConfig{URL: "https://ntfy.sh", Topic: "alerts", Token: "tk_abc"})
	if !strings.Contains(got, "tk_abc@ntfy.sh/alerts") {
		t.Errorf("expected token in URL, got %q", got)
	}

	// http scheme preserved.
	got, _ = ntfyConfigToShoutrrr(NtfyConfig{URL: "http://localhost:8080", Topic: "alerts"})
	if !strings.Contains(got, "scheme=http") {
		t.Errorf("expected scheme=http for http URL, got %q", got)
	}
}

func TestGotifyConfigToShoutrrr(t *testing.T) {
	got, err := gotifyConfigToShoutrrr(GotifyConfig{URL: "https://gotify.example.com", Token: "AyqB1cd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "gotify://gotify.example.com/AyqB1cd" {
		t.Errorf("got %q", got)
	}

	// http URL adds disableTLS=true.
	got, _ = gotifyConfigToShoutrrr(GotifyConfig{URL: "http://gotify.lan", Token: "tk"})
	if !strings.Contains(got, "disableTLS=true") {
		t.Errorf("expected disableTLS=true for http URL, got %q", got)
	}
}

func TestToShoutrrrURLDispatches(t *testing.T) {
	cases := []struct {
		ch   fakeChannel
		want string
	}{
		{newCh("shoutrrr", ShoutrrrConfig{URL: "telegram://token@chats=-100"}), "telegram://token@chats=-100"},
		{newCh("discord", DiscordConfig{WebhookURL: "https://discord.com/api/webhooks/1/2"}), "discord://2@1"},
	}
	for _, c := range cases {
		got, err := ToShoutrrrURL(c.ch)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.ch.t, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.ch.t, got, c.want)
		}
	}

	// Unknown / non-Shoutrrr-routable type errors out.
	if _, err := ToShoutrrrURL(newCh("webhook", nil)); err == nil {
		t.Errorf("expected error for webhook (non-Shoutrrr type)")
	}
}
