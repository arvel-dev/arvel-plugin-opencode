// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-plugin-opencode/internal/upstream"
	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/redact"
)

// idleAfter is how long a session may sit idle before its child is stopped
// (SPEC §4, PROTOCOL §8). It is a package var so tests can shrink it.
var idleAfter = 15 * time.Minute

// cancelAfter is how long a cancelled turn may run before the child is killed
// (PROTOCOL §7.6). It is a package var so tests can shrink it.
var cancelAfter = 10 * time.Second

// errClosed marks operations on a session whose Close already ran.
var errClosed = errors.New("proxy: session is closed")

// Config carries everything the bridge needs to start a child.
type Config struct {
	Bin               string
	Flavour           detect.Flavour
	Agent             string // connection config agent, default "build"
	LoadProjectConfig bool
	Proxy             string // sandbox.proxy ("" = none)
	Log               *slog.Logger
	Redactor          *redact.Redactor
}

// redact runs s through the redactor when one is configured.
func (c Config) redact(s string) string {
	if c.Redactor == nil {
		return s
	}
	return c.Redactor.String(s)
}

// Session bridges one host ACP session to one upstream child.
//
// Invariant: the host's _meta.arvel is never forwarded to the child. Every
// upstream request is a typed acp.* struct whose Meta field is left nil (the
// host policy is translated into the child's native config by O3, not passed
// as _meta).
type Session struct {
	id      string
	cfg     Config
	cwd     string
	mcp     []acp.McpServer
	model   string
	effort  string
	baseCtx context.Context // session lifetime; the child's lifetime is tied to it

	mu          sync.Mutex // guards client, opts and the lifecycle flags
	client      *upstream.Client
	opts        []acp.SessionConfigOption
	closed      bool
	idle        *time.Timer
	cancelTimer *time.Timer
	cancelling  bool
	turning     bool
}

var _ plugin.Session = (*Session)(nil)
var _ plugin.ConfigOptioner = (*Session)(nil)

// New starts a child and creates the upstream session (session/new).
func New(ctx context.Context, cfg Config, params plugin.SessionParams) (*Session, error) {
	c, err := start(ctx, cfg, params)
	if err != nil {
		return nil, err
	}
	res, err := c.NewSession(ctx, params.CWD, params.MCPServers)
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("proxy: session/new: %w", err)
	}
	s := &Session{
		id: string(res.SessionId), cfg: cfg, cwd: params.CWD, mcp: params.MCPServers,
		baseCtx: ctx, client: c, opts: res.ConfigOptions,
		model: params.Meta.Model, effort: string(params.Meta.Effort),
	}
	if err := s.applyInitial(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	s.armIdle()
	return s, nil
}

// Load starts a child and resumes the upstream session (session/load).
func Load(ctx context.Context, cfg Config, id string, params plugin.SessionParams) (*Session, error) {
	c, err := start(ctx, cfg, params)
	if err != nil {
		return nil, err
	}
	res, err := c.LoadSession(ctx, acp.SessionId(id), params.CWD, params.MCPServers)
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("proxy: session/load: %w", err)
	}
	s := &Session{
		id: id, cfg: cfg, cwd: params.CWD, mcp: params.MCPServers,
		baseCtx: ctx, client: c, opts: res.ConfigOptions,
		model: params.Meta.Model, effort: string(params.Meta.Effort),
	}
	if err := s.applyInitial(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	s.armIdle()
	return s, nil
}

// start spawns the child for a session.
func start(ctx context.Context, cfg Config, params plugin.SessionParams) (*upstream.Client, error) {
	return upstream.Start(ctx, upstream.Config{
		Bin:               cfg.Bin,
		CWD:               params.CWD,
		LoadProjectConfig: cfg.LoadProjectConfig,
		Proxy:             cfg.Proxy,
		Log:               cfg.Log,
		Redactor:          cfg.Redactor,
	})
}

// applyInitial sets the chosen model/effort/agent after session/new or load.
func (s *Session) applyInitial(ctx context.Context) error {
	s.mu.Lock()
	c := s.client
	model, effort := s.model, s.effort
	opts := s.opts
	s.mu.Unlock()
	if c == nil {
		return errClosed
	}

	id := acp.SessionId(s.id)
	if model != "" {
		if _, err := c.SetOption(ctx, id, "model", modelValue(s.cfg.Flavour, model, effort)); err != nil {
			return fmt.Errorf("proxy: set model: %w", err)
		}
	}
	if s.cfg.Flavour == detect.V2 && effort != "" && effort != "default" {
		if _, err := c.SetOption(ctx, id, "effort", effort); err != nil {
			return fmt.Errorf("proxy: set effort: %w", err)
		}
	}
	if hasOption(opts, "mode") {
		if _, err := c.SetOption(ctx, id, "mode", modeValue(s.cfg.Agent)); err != nil && s.cfg.Log != nil {
			s.cfg.Log.Warn("proxy: cannot set mode", "err", s.cfg.redact(err.Error()))
		}
	}
	return nil
}

// ID returns the upstream session id (stable across idle resume).
func (s *Session) ID() string { return s.id }

// ConfigOptions returns the upstream config options for the host.
func (s *Session) ConfigOptions() []acp.SessionConfigOption {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opts
}

// setOpts caches the latest config options after a successful set.
func (s *Session) setOpts(opts []acp.SessionConfigOption) {
	if opts == nil {
		return
	}
	s.mu.Lock()
	s.opts = opts
	s.mu.Unlock()
}

// Close stops the timers and kills the child (idempotent).
func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	if s.idle != nil {
		s.idle.Stop()
	}
	if s.cancelTimer != nil {
		s.cancelTimer.Stop()
		s.cancelTimer = nil
	}
	c := s.client
	s.client = nil
	s.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
	return nil
}
