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
	"github.com/arvel-dev/arvel-sdk/protocol"
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

// UsageTracker receives per-turn usage and limit events from a session so the
// provider can answer _arvel/usage. Implemented by the provider; nil disables
// reporting.
type UsageTracker interface {
	Add(tu protocol.TurnUsage, currency string)
	RateLimited()
	MarkOK()
}

// Config carries everything the bridge needs to start a child.
type Config struct {
	Bin               string
	Flavour           detect.Flavour
	Agent             string // connection config agent, default "build"
	LoadProjectConfig bool
	DataDir           string // plugin data dir for per-session artifacts
	Proxy             string // sandbox.proxy ("" = none)
	Log               *slog.Logger
	Redactor          *redact.Redactor
	Tracker           UsageTracker // usage reporter; nil disables reporting (O4)
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
	meta    protocol.SessionMeta
	allow   allowList       // session-local "always allow" set (SPEC §6.2)
	baseCtx context.Context // session lifetime; the child's lifetime is tied to it

	inlineConfig  []byte // OPENCODE_CONFIG_CONTENT (computed once)
	runDir        string // <DataDir>/run/<runID>; "" when none
	skillsCleanup func() // skills.Materialize cleanup (never nil after prepare)

	mu          sync.Mutex // guards client, opts and the lifecycle flags
	client      *upstream.Client
	opts        []acp.SessionConfigOption
	closed      bool
	idle        *time.Timer
	cancelTimer *time.Timer
	cancelling  bool
	turning     bool

	// per-child usage baseline for per-turn deltas (guarded by mu)
	lastInput, lastOutput, lastCacheRead int64
	lastCostAmount                       float64
	lastCostCurrency                     string
	hasCost                              bool
}

var _ plugin.Session = (*Session)(nil)
var _ plugin.ConfigOptioner = (*Session)(nil)

// New starts a child and creates the upstream session (session/new).
func New(ctx context.Context, cfg Config, params plugin.SessionParams) (*Session, error) {
	s := &Session{
		cfg: cfg, cwd: params.CWD, mcp: filterMCP(params.MCPServers, cfg.Log),
		meta: params.Meta, model: params.Meta.Model, effort: string(params.Meta.Effort),
		baseCtx: ctx,
	}
	if err := s.prepare(); err != nil {
		s.cleanupArtifacts()
		return nil, err
	}
	c, err := s.start(ctx)
	if err != nil {
		s.cleanupArtifacts()
		return nil, err
	}
	res, err := c.NewSession(ctx, s.cwd, s.mcp)
	if err != nil {
		_ = c.Close()
		s.cleanupArtifacts()
		return nil, fmt.Errorf("proxy: session/new: %w", err)
	}
	s.id = string(res.SessionId)
	s.client = c
	s.opts = res.ConfigOptions
	if err := s.applyInitial(ctx); err != nil {
		_ = c.Close()
		s.cleanupArtifacts()
		return nil, err
	}
	s.armIdle()
	return s, nil
}

// Load starts a child and resumes the upstream session (session/load).
func Load(ctx context.Context, cfg Config, id string, params plugin.SessionParams) (*Session, error) {
	s := &Session{
		id: id, cfg: cfg, cwd: params.CWD, mcp: filterMCP(params.MCPServers, cfg.Log),
		meta: params.Meta, model: params.Meta.Model, effort: string(params.Meta.Effort),
		baseCtx: ctx,
	}
	if err := s.prepare(); err != nil {
		s.cleanupArtifacts()
		return nil, err
	}
	c, err := s.start(ctx)
	if err != nil {
		s.cleanupArtifacts()
		return nil, err
	}
	res, err := c.LoadSession(ctx, acp.SessionId(id), s.cwd, s.mcp)
	if err != nil {
		_ = c.Close()
		s.cleanupArtifacts()
		return nil, fmt.Errorf("proxy: session/load: %w", err)
	}
	s.client = c
	s.opts = res.ConfigOptions
	if err := s.applyInitial(ctx); err != nil {
		_ = c.Close()
		s.cleanupArtifacts()
		return nil, err
	}
	s.armIdle()
	return s, nil
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
	s.cleanupArtifacts()
	return nil
}
