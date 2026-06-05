// Package notify delivers human-facing PR build status notifications.
//
// Currently supports a single transport: HTTP POST to a Discord-compatible
// webhook URL with a JSON body of the form `{"content": "..."}`. This is the
// shape used by:
//
//   - Discord Incoming Webhooks
//   - The OpenClaw "no-AI human inbox" webhook (passes `content` straight
//     through to the channel without invoking an agent turn)
//
// Configured via [notify] in ~/.bitgit/config.toml:
//
//	[notify]
//	webhook_url = "https://…"
//	# Optional: when true, also POST INPROGRESS state changes (default false).
//	notify_inprogress = false
//
// We intentionally do not depend on Discord API specifics so the same field
// works for any webhook that accepts {"content": "<string>"}.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Config controls notify behaviour.
type Config struct {
	WebhookURL       string
	NotifyInProgress bool
}

// Client is a minimal webhook poster.
type Client struct {
	cfg Config
	hc  *http.Client
}

// New builds a Client. A nil cfg or empty WebhookURL disables sending.
func New(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		hc:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled reports whether sending is configured.
func (c *Client) Enabled() bool {
	return c != nil && c.cfg.WebhookURL != ""
}

// Send POSTs a content message. Returns nil silently when not configured so
// callers can call unconditionally.
func (c *Client) Send(ctx context.Context, content string) error {
	if !c.Enabled() {
		return nil
	}
	body, err := json.Marshal(struct {
		Content string `json:"content"`
	}{Content: content})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("notify post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notify post: HTTP %d", resp.StatusCode)
	}
	return nil
}
