// Package notify delivers human-facing PR build status notifications.
//
// Two transport modes, picked by URL shape:
//
//   - "router"  — POST {project,event,status,message,url} JSON. This is the
//                 shape consumed by exisz/webhook-router's `generic` parser.
//                 Use when notify_url points at .../webhook/hook/<route>.
//                 The router fans out to Discord (any channel/thread).
//
//   - "discord" — POST {"content": "<msg>"} directly. Compatible with raw
//                 Discord webhook URLs. Use when you don't want to depend on
//                 the router (e.g. another machine).
//
// Mode is auto-detected from the URL: anything containing
// "discord.com/api/webhooks" is "discord"; anything else is "router". An
// explicit `notify.mode` config override is honoured if set.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Config controls notify behaviour.
type Config struct {
	WebhookURL       string
	NotifyInProgress bool
	// Mode forces the transport: "router" or "discord". Empty = auto.
	Mode string
}

// Event is the structured payload passed to Send.
type Event struct {
	Project string // e.g. "PROJ/repo"
	Event   string // e.g. "pr.ci"
	Status  string // success | error | pending | info
	Message string // human-readable summary
	URL     string // PR URL
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

// detectMode returns "discord" or "router".
func (c *Client) detectMode() string {
	if c.cfg.Mode != "" {
		return strings.ToLower(c.cfg.Mode)
	}
	if strings.Contains(c.cfg.WebhookURL, "discord.com/api/webhooks") {
		return "discord"
	}
	return "router"
}

// Send delivers an event. Returns nil silently when not configured.
func (c *Client) Send(ctx context.Context, ev Event) error {
	if !c.Enabled() {
		return nil
	}
	var body []byte
	var err error
	switch c.detectMode() {
	case "discord":
		body, err = json.Marshal(struct {
			Content string `json:"content"`
		}{Content: formatDiscord(ev)})
	default: // router
		body, err = json.Marshal(struct {
			Project string `json:"project"`
			Event   string `json:"event"`
			Status  string `json:"status"`
			Message string `json:"message"`
			URL     string `json:"url,omitempty"`
		}{ev.Project, ev.Event, ev.Status, ev.Message, ev.URL})
	}
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

func formatDiscord(ev Event) string {
	icon := "ℹ️"
	switch strings.ToLower(ev.Status) {
	case "success":
		icon = "✅"
	case "error":
		icon = "❌"
	case "pending":
		icon = "⏳"
	}
	if ev.URL != "" {
		return fmt.Sprintf("%s **%s** — %s\n%s", icon, ev.Project, ev.Message, ev.URL)
	}
	return fmt.Sprintf("%s **%s** — %s", icon, ev.Project, ev.Message)
}
