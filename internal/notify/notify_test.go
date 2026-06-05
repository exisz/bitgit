package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendDisabledNoOp(t *testing.T) {
	c := New(Config{})
	if c.Enabled() {
		t.Fatal("expected disabled")
	}
	if err := c.Send(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
}

func TestSendPosts(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var msg struct {
			Content string `json:"content"`
		}
		_ = json.Unmarshal(body, &msg)
		got = msg.Content
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := New(Config{WebhookURL: srv.URL})
	if err := c.Send(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("server got %q", got)
	}
}

func TestSendErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c := New(Config{WebhookURL: srv.URL})
	if err := c.Send(context.Background(), "x"); err == nil {
		t.Fatal("expected error on 500")
	}
}
