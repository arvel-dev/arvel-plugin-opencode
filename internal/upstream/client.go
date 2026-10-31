// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-sdk/jsonrpc"
	"github.com/arvel-dev/arvel-sdk/procx"
	"github.com/arvel-dev/arvel-sdk/redact"
)

// Config selects the child and its environment.
type Config struct {
	Bin               string // absolute opencode path
	CWD               string // session working directory
	LoadProjectConfig bool
	Proxy             string // sandbox.proxy ("" = none)
	Log               *slog.Logger
	Redactor          *redact.Redactor // redacts external text in debug logs
}

// Client is an ACP client for one opencode acp child.
type Client struct {
	cfg  Config
	peer *jsonrpc.Peer

	// teardown: kill kills the child tree; closeOut closes the child-stdout
	// write end so the peer read loop EOFs; closeIn closes our stdin write end;
	// serveCancel stops the peer Serve loop.
	kill        func(time.Duration)
	closeOut    func() error
	closeIn     func() error
	serveCancel context.CancelFunc
	done        <-chan struct{} // child exit; nil for pipe tests

	mu     sync.Mutex // guards sink + closed
	sink   Sink
	closed bool
}

// Start spawns `opencode acp` and completes the initialize handshake. It is
// the production entry point; tests use newPipeClient.
func Start(ctx context.Context, cfg Config) (*Client, error) {
	inR, inW := io.Pipe()   // child stdin  = inR; we write to inW
	outR, outW := io.Pipe() // child stdout = outW; we read from outR
	p, err := procx.Start(ctx, procx.Spec{
		Path:   cfg.Bin,
		Args:   []string{"acp"},
		Dir:    cfg.CWD,
		Env:    Env(cfg.LoadProjectConfig, cfg.Proxy),
		Stdin:  inR,
		Stdout: outW,
		Stderr: vendorStderr(cfg.Log, cfg.Redactor),
	})
	if err != nil {
		_ = inR.Close()
		_ = inW.Close()
		_ = outR.Close()
		_ = outW.Close()
		return nil, fmt.Errorf("upstream: start opencode acp: %w", err)
	}
	c := newPipeClient(cfg, outR, inW, inW.Close, outW.Close)
	c.kill = func(g time.Duration) { p.KillTree(g) }
	c.done = p.Done()
	if err := c.handshake(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// newPipeClient wires a peer over r/w and starts its Serve loop. closeIn and
// closeOut close the two pipe write ends (nil for a non-pipe test transport).
func newPipeClient(cfg Config, r io.Reader, w io.Writer, closeIn, closeOut func() error) *Client {
	c := &Client{cfg: cfg, closeIn: closeIn, closeOut: closeOut}
	c.peer = jsonrpc.NewPeer(r, w, jsonrpc.WithLogger(cfg.Log))
	serveCtx, cancel := context.WithCancel(context.Background())
	c.serveCancel = cancel
	go func() { _ = c.peer.Serve(serveCtx) }()
	return c
}

// handshake runs initialize then notifications/initialized and registers the
// child-initiated methods.
func (c *Client) handshake(ctx context.Context) error {
	var initRes acp.InitializeResponse
	if err := c.peer.Call(ctx, "initialize", acp.InitializeRequest{
		ProtocolVersion: 1,
		ClientCapabilities: acp.ClientCapabilities{
			Fs:       acp.FileSystemCapabilities{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
		ClientInfo: &acp.Implementation{Name: "arvel-plugin-opencode", Version: "0.1.0"},
	}, &initRes); err != nil {
		return fmt.Errorf("upstream: initialize: %w", err)
	}
	if initRes.ProtocolVersion != 1 {
		return fmt.Errorf("upstream: unsupported protocol version %d", initRes.ProtocolVersion)
	}
	if err := c.peer.Notify("notifications/initialized", map[string]any{}); err != nil {
		return fmt.Errorf("upstream: notifications/initialized: %w", err)
	}
	c.register()
	return nil
}

// Close tears the child down gracefully and is idempotent.
func (c *Client) Close() error { return c.teardown(500 * time.Millisecond) }

// Kill stops the child tree immediately (used by the cancel watchdog) and
// tears the connection down. Idempotent.
func (c *Client) Kill(grace time.Duration) { _ = c.teardown(grace) }

// teardown kills the child, then closes the pipe write ends so the peer read
// loop EOFs and pending calls fail with ErrClosed, then stops the Serve loop.
func (c *Client) teardown(grace time.Duration) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// Close the pipe write ends first: this EOFs the peer read loop (so pending
	// calls fail with ErrClosed promptly) and unblocks the exec copy goroutines,
	// then kill the child and stop the Serve loop.
	if c.closeOut != nil {
		_ = c.closeOut()
	}
	if c.closeIn != nil {
		_ = c.closeIn()
	}
	if c.kill != nil {
		c.kill(grace)
	}
	if c.serveCancel != nil {
		c.serveCancel()
	}
	return nil
}

// Done reports child exit; nil when the client was built over pipes.
func (c *Client) Done() <-chan struct{} { return c.done }

// redact runs external text through the redactor when one is configured.
func (c *Client) redact(s string) string {
	if c.cfg.Redactor == nil {
		return s
	}
	return c.cfg.Redactor.String(s)
}
