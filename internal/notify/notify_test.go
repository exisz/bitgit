package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendDisabledNoOp(t *testing.T) {
	c := New(Config{})
	if c.Enabled() {
		t.Fatal("expected disabled")
	}
	if err := c.Send(context.Background(), Event{Project: "p", Event: "e", Status: "success", Message: "m"}); err != nil {
		t.Fatal(err)
	}
}

func TestSendRouterMode(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := New(Config{WebhookURL: srv.URL})
	if c.detectMode() != "router" {
		t.Fatalf("want router mode, got %s", c.detectMode())
	}
	if err := c.Send(context.Background(), Event{Project: "P/r", Event: "pr.ci", Status: "success", Message: "ok", URL: "u"}); err != nil {
		t.Fatal(err)
	}
	if got["project"] != "P/r" || got["status"] != "success" {
		t.Fatalf("payload: %+v", got)
	}
}

func TestSendDiscordModeAutodetect(t *testing.T) {
	var got map[string]any
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(204)
	}))
	defer srv.Close()
	// Force discord mode via Mode override (URL autodetect requires real
	// discord.com host which we can't fake here without real DNS).
	c := New(Config{WebhookURL: srv.URL, Mode: "discord"})
	if err := c.Send(context.Background(), Event{Project: "P/r", Status: "success", Message: "ok", URL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("server not called")
	}
	content, _ := got["content"].(string)
	if !strings.Contains(content, "P/r") || !strings.Contains(content, "✅") {
		t.Fatalf("content missing pieces: %q", content)
	}
}

func TestModeAutodetect(t *testing.T) {
	if got := (&Client{cfg: Config{WebhookURL: "https://discord.com/api/webhooks/1/abc"}}).detectMode(); got != "discord" {
		t.Fatalf("discord URL got %s", got)
	}
	if got := (&Client{cfg: Config{WebhookURL: "https://linux.queue-musical.ts.net/webhook/hook/x"}}).detectMode(); got != "router" {
		t.Fatalf("router URL got %s", got)
	}
}

func TestSendErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c := New(Config{WebhookURL: srv.URL})
	if err := c.Send(context.Background(), Event{Project: "p", Status: "success", Message: "m"}); err == nil {
		t.Fatal("expected error on 500")
	}
}
