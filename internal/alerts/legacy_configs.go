// Legacy channel config structs, retained so the API can validate stored
// channel rows and the Shoutrrr dispatcher can unmarshal them. The Send
// implementations that used to accompany each struct have been removed in
// favor of ch_shoutrrr.go.
package alerts

type DiscordConfig struct {
	WebhookURL string `json:"webhook_url"`
}

type SlackConfig struct {
	WebhookURL string `json:"webhook_url"`
}

type NtfyConfig struct {
	URL      string `json:"url"`
	Topic    string `json:"topic"`
	Token    string `json:"token,omitempty"`
	Priority string `json:"priority,omitempty"`
}

type GotifyConfig struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}
