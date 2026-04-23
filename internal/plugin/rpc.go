package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// Client is a JSON-RPC 2.0 stdio client for a single plugin subprocess.
//
// One Client per plugin per bitgit invocation. Lifetimes are short: spawn,
// invoke a few hooks, close. The protocol is documented in
// docs/plugin-protocol.md.
//
// This is the chassis transport. The dispatcher (Dispatch) is the higher-level
// API verbs should use; Client is exposed for advanced cases and tests.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	mu      sync.Mutex
	nextID  atomic.Int64
	pending map[int64]chan rpcResp
	closed  atomic.Bool
}

type rpcReq struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcErr) Error() string { return fmt.Sprintf("plugin rpc error %d: %s", e.Code, e.Message) }

// Spawn starts a plugin subprocess from a manifest.
func Spawn(ctx context.Context, m Manifest) (*Client, error) {
	entry := m.Entrypoint
	if entry == "" {
		return nil, fmt.Errorf("plugin %s: empty entrypoint", m.Name)
	}
	if !filepath.IsAbs(entry) {
		entry = filepath.Join(m.Dir, entry)
	}

	cmd := exec.CommandContext(ctx, entry)
	cmd.Dir = m.Dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil // surface plugin stderr to user terminal? configurable later
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn plugin %s: %w", m.Name, err)
	}

	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		pending: make(map[int64]chan rpcResp),
	}
	go c.readLoop()
	return c, nil
}

func (c *Client) readLoop() {
	dec := json.NewDecoder(c.stdout)
	for {
		var resp rpcResp
		if err := dec.Decode(&resp); err != nil {
			c.failPending(err)
			return
		}
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		delete(c.pending, resp.ID)
		c.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
}

func (c *Client) failPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- rpcResp{ID: id, Error: &rpcErr{Code: -32000, Message: err.Error()}}
		delete(c.pending, id)
	}
}

// Call invokes a JSON-RPC method on the plugin and decodes result into out.
func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	if c.closed.Load() {
		return fmt.Errorf("plugin client closed")
	}
	id := c.nextID.Add(1)
	ch := make(chan rpcResp, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	req := rpcReq{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	buf, err := json.Marshal(req)
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	if _, err := c.stdin.Write(buf); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return resp.Error
		}
		if out != nil && len(resp.Result) > 0 {
			return json.Unmarshal(resp.Result, out)
		}
		return nil
	}
}

// Close terminates the plugin subprocess.
func (c *Client) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	err := c.cmd.Wait()
	if err != nil && !strings.Contains(err.Error(), "signal: killed") {
		return err
	}
	return nil
}
