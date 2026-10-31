// SPDX-License-Identifier: Apache-2.0

// Package provider implements the plugin.Provider for OpenCode. O1 wired
// detection, the model catalogue, status and actions; O2 adds the per-session
// upstream child and the proxy bridge; usage and typed errors arrive in O4.
package provider

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-plugin-opencode/internal/models"
	"github.com/arvel-dev/arvel-plugin-opencode/internal/proxy"
	"github.com/arvel-dev/arvel-plugin-opencode/internal/usage"
	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
	"github.com/arvel-dev/arvel-sdk/redact"
)

// installDocsURL is the install-help link, matching arvel.json
// requires[].install.docs.
const installDocsURL = "https://opencode.ai/v2/docs/"

// detector is the detection surface the provider depends on. *detect.Detector
// implements it; tests substitute a fake so no test spawns a real CLI.
type detector interface {
	Detect(ctx context.Context) (detect.Info, error)
	Auth(ctx context.Context, f detect.Flavour) ([]detect.Provider, error)
	ModelsV1(ctx context.Context) ([]detect.RawModel, error)
	ModelsV2(ctx context.Context) ([]detect.RawModel, error)
}

// Provider is the OpenCode provider.
type Provider struct {
	mu          sync.RWMutex
	host        plugin.Host
	log         *slog.Logger
	rd          *redact.Redactor
	bin         string                    // resolved binary path ("" = not located)
	cfgPath     string                    // opencodePath from the connection config
	info        *detect.Info              // cached Detect result (guarded by detMu)
	cache       models.Cache              // model catalogue cache (guarded by catMu)
	newDetector func(bin string) detector // test seam; defaults to detect.New

	// O2: session bridge inputs, immutable after Initialize.
	sandbox           *protocol.SandboxInfo
	loadProjectConfig bool
	agent             string
	dataDir           string // plugin data dir (per-session artifacts, O3)

	tracker *usage.Tracker // O4: aggregated spend for _arvel/usage

	detMu sync.Mutex // guards info (detection cache)
	catMu sync.Mutex // guards cache (model catalogue)
}

var _ plugin.Provider = (*Provider)(nil)
var _ plugin.HostAware = (*Provider)(nil)

// New returns the OpenCode provider. No process is spawned until Status or
// Models is called.
func New() *Provider {
	return &Provider{rd: redact.New(), newDetector: func(bin string) detector { return detect.New(bin) }}
}

// SetHost stores the SDK host (logger/status-change) before Initialize.
func (p *Provider) SetHost(h plugin.Host) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.host = h
	if h != nil {
		p.log = h.Logger()
	}
}

// Initialize resolves the binary path and stores the connection config. It does
// not run opencode (that is Status/Models work).
func (p *Provider) Initialize(_ context.Context, meta protocol.InitMeta) (plugin.Features, error) {
	// info is guarded by detMu everywhere else; reset it under the same lock so
	// the guard is consistent (Initialize never overlaps Status/Models in the
	// SDK lifecycle, but the race detector must agree).
	p.detMu.Lock()
	p.info = nil
	p.detMu.Unlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	p.sandbox = meta.Sandbox
	p.dataDir = meta.DataDir
	p.tracker = usage.New(meta.DataDir)
	if s, ok := meta.Connection.Config["opencodePath"].(string); ok {
		p.cfgPath = s
	}
	if b, ok := meta.Connection.Config["loadProjectConfig"].(bool); ok {
		p.loadProjectConfig = b
	}
	if a, ok := meta.Connection.Config["agent"].(string); ok && a != "" {
		p.agent = a
	}
	bin, err := detect.ResolveBin(p.cfgPath)
	if err != nil {
		p.bin = ""
		if p.log != nil {
			p.log.Debug("opencode binary not located", "err", p.rd.String(err.Error()))
		}
	} else {
		p.bin = bin
	}
	return plugin.Features{
		LoadSession:      true,
		Skills:           &protocol.SkillsInfo{ProjectDirs: []string{".opencode/skills"}, Spec: "agentskills/1"},
		InstructionFiles: []string{"AGENTS.md"},
		Subagents:        true,
		MCPHTTP:          true,
		MCPSSE:           false,
	}, nil
}

// Status reports install/version/login readiness (SPEC.md §9.1).
func (p *Provider) Status(ctx context.Context) (protocol.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, rpcBudget)
	defer cancel()

	p.mu.RLock()
	bin := p.bin
	p.mu.RUnlock()
	if bin == "" {
		return notInstalledStatus(), nil
	}

	info, err := p.detect(ctx)
	if err != nil {
		p.warn("cannot detect opencode", err)
		return protocol.Status{State: protocol.StateError, Message: "Cannot detect OpenCode"}, nil
	}
	binInfo := &protocol.BinaryInfo{Name: "opencode", Path: p.rd.String(bin), Version: info.Version.String()}
	if info.Version.IsOutdated() {
		return protocol.Status{
			State:   protocol.StateOutdated,
			Message: "OpenCode " + info.Version.String() + " is older than " + detect.MinVersion,
			Binary:  binInfo,
		}, nil
	}

	det := p.newDetector(bin)
	providers, err := det.Auth(ctx, info.Flavour())
	if err != nil {
		p.warn("cannot read opencode providers", err)
		return protocol.Status{State: protocol.StateError, Message: "Cannot read OpenCode providers"}, nil
	}
	if len(providers) > 0 {
		return readyStatus(binInfo, len(providers)), nil
	}

	// No stored credentials: a keyless local provider (Ollama, LM Studio) still
	// counts as configured (SPEC §9.1) when the catalogue is non-empty.
	if res := p.catalogue(ctx, info.Flavour(), false); res.Default != models.EmptyID {
		return readyStatus(binInfo, 0), nil
	}
	return protocol.Status{
		State:   protocol.StateNeedsLogin,
		Message: "No OpenCode providers are signed in",
		Binary:  binInfo,
		Actions: []protocol.Action{{ID: "login", Label: "Sign in", Kind: protocol.ActionKindTerminal}},
	}, nil
}

// Models returns the cached OpenCode catalogue (SPEC.md §9.2–9.5).
func (p *Provider) Models(ctx context.Context, refresh bool) (protocol.ModelsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, rpcBudget)
	defer cancel()

	p.mu.RLock()
	bin := p.bin
	p.mu.RUnlock()
	if bin == "" {
		return models.Empty(), nil
	}
	info, err := p.detect(ctx)
	if err != nil {
		p.warn("cannot detect opencode for the model catalogue", err)
		return models.Empty(), nil
	}
	return p.catalogue(ctx, info.Flavour(), refresh), nil
}

// NewSession starts a per-session child and bridges it (SPEC §4, §7).
func (p *Provider) NewSession(ctx context.Context, params plugin.SessionParams) (plugin.Session, error) {
	cfg, err := p.proxyConfig(ctx)
	if err != nil {
		return nil, err
	}
	return proxy.New(ctx, cfg, params)
}

// LoadSession resumes a session after a process restart (PROTOCOL §7.4).
func (p *Provider) LoadSession(ctx context.Context, lp plugin.LoadParams) (plugin.Session, error) {
	cfg, err := p.proxyConfig(ctx)
	if err != nil {
		return nil, err
	}
	return proxy.Load(ctx, cfg, lp.SessionID, lp.SessionParams)
}

// proxyConfig snapshots the immutable bridge inputs and ensures detection.
func (p *Provider) proxyConfig(ctx context.Context) (proxy.Config, error) {
	p.mu.RLock()
	bin := p.bin
	sandbox := p.sandbox
	lpc := p.loadProjectConfig
	agent := p.agent
	dataDir := p.dataDir
	log := p.log
	rd := p.rd
	p.mu.RUnlock()

	if bin == "" {
		return proxy.Config{}, protocol.NewError(protocol.ErrBinaryMissing, "OpenCode is not installed", nil)
	}
	info, err := p.detect(ctx)
	if err != nil {
		return proxy.Config{}, protocol.NewError(protocol.ErrBinaryMissing, "cannot detect OpenCode", nil)
	}
	proxyStr := ""
	if sandbox != nil {
		proxyStr = sandbox.Proxy
	}
	return proxy.Config{
		Bin:               bin,
		Flavour:           info.Flavour(),
		Agent:             agent,
		LoadProjectConfig: lpc,
		DataDir:           dataDir,
		Proxy:             proxyStr,
		Log:               log,
		Redactor:          rd,
		Tracker:           p.usageSink(),
	}, nil
}

// rpcBudget bounds one Status/Models call below the host's 10 s request
// deadline, including a keyless-provider catalogue probe.
const rpcBudget = 9 * time.Second

// detect returns the cached Detect result, spawning --version once. The
// dedicated detMu means a slow --version never blocks Status/Models/sessions
// that do not need detection.
func (p *Provider) detect(ctx context.Context) (detect.Info, error) {
	p.detMu.Lock()
	defer p.detMu.Unlock()
	if p.info != nil {
		return *p.info, nil
	}
	p.mu.RLock()
	bin := p.bin
	p.mu.RUnlock()
	det := p.newDetector(bin)
	info, err := det.Detect(ctx)
	if err != nil {
		return detect.Info{}, err
	}
	p.info = &info
	return info, nil
}

// catalogue returns a valid catalogue: the cache, a fresh build, or the
// hidden placeholder on any fetch/build failure (PROTOCOL §6.2 requires a
// non-empty list; the host gates on status). catMu serialises cache access.
func (p *Provider) catalogue(ctx context.Context, f detect.Flavour, refresh bool) protocol.ModelsResult {
	p.catMu.Lock()
	defer p.catMu.Unlock()
	if cached, ok := p.cache.Get(p.now(), f, refresh); ok {
		if p.log != nil {
			p.log.Debug("model catalog cache hit", "flavour", f)
		}
		return cached
	}
	p.mu.RLock()
	bin := p.bin
	p.mu.RUnlock()
	det := p.newDetector(bin)
	var raw []detect.RawModel
	var err error
	if f == detect.V1 {
		raw, err = det.ModelsV1(ctx)
	} else {
		raw, err = det.ModelsV2(ctx)
	}
	if err != nil {
		p.warn("cannot list opencode models", err)
		return models.Empty()
	}
	if len(raw) == 0 {
		p.warn("opencode reported no models", nil)
		return models.Empty()
	}
	result, err := models.Build(raw, f)
	if err != nil {
		p.warn("cannot build the opencode catalogue", err)
		return models.Empty()
	}
	for i := range result.Models {
		result.Models[i].Label = p.rd.String(result.Models[i].Label)
		result.Models[i].Description = p.rd.String(result.Models[i].Description)
	}
	p.cache.Put(p.now(), f, result)
	if p.log != nil {
		p.log.Debug("model catalog refreshed", "flavour", f, "models", len(result.Models))
	}
	return result
}

func notInstalledStatus() protocol.Status {
	return protocol.Status{
		State:   protocol.StateNotInstalled,
		Message: "OpenCode is not installed",
		Actions: []protocol.Action{{ID: "install_help", Label: "Install OpenCode", Kind: protocol.ActionKindURL}},
	}
}

// readyStatus builds a ready status; providers==0 means a keyless local setup.
func readyStatus(bin *protocol.BinaryInfo, providers int) protocol.Status {
	msg := "OpenCode is ready"
	switch providers {
	case 1:
		msg = "1 provider configured"
	default:
		if providers > 1 {
			msg = fmt.Sprintf("%d providers configured", providers)
		}
	}
	return protocol.Status{
		State:   protocol.StateReady,
		Message: msg,
		Binary:  bin,
		Actions: []protocol.Action{{ID: "login", Label: "Sign in", Kind: protocol.ActionKindTerminal}},
	}
}

// Usage reports the aggregated spend from all sessions so far (SPEC §8,
// PROTOCOL §6.3). Before the first turn it is "unknown"; afterwards "estimated".
func (p *Provider) Usage(context.Context) (protocol.Usage, error) {
	p.mu.RLock()
	t := p.tracker
	p.mu.RUnlock()
	if t == nil {
		return protocol.Usage{Source: protocol.UsageSourceUnknown, Status: protocol.UsageStatusOK}, nil
	}
	return t.Snapshot(), nil
}

// Action runs login and install-help (SPEC.md §9).
func (p *Provider) Action(_ context.Context, id string) (protocol.ActionResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	switch id {
	case "login":
		if p.bin == "" {
			return protocol.ActionResult{}, protocol.NewError(protocol.ErrProviderError, "opencode is not installed", map[string]any{"vendorCode": "not_installed"})
		}
		return protocol.ActionResult{
			Kind:    protocol.ActionKindTerminal,
			Command: []string{p.bin, "auth", "login"},
			Explain: "Finish OpenCode sign-in in this terminal",
		}, nil
	case "install_help":
		return protocol.ActionResult{
			Kind:    protocol.ActionKindURL,
			URL:     installDocsURL,
			Explain: "Open the OpenCode install guide",
		}, nil
	default:
		return protocol.ActionResult{}, protocol.NewError(protocol.ErrProviderError, "unknown action: "+id, map[string]any{"vendorCode": "unknown_action"})
	}
}

// Shutdown closes nothing itself: the SDK closes every session (which kills
// its child) before calling Shutdown.
func (p *Provider) Shutdown(context.Context) error { return nil }

// now is the cache clock; a method so tests can reason about it.
func (p *Provider) now() time.Time { return time.Now() }

// warn logs a non-fatal detection/catalogue failure when a host logger exists.
func (p *Provider) warn(msg string, err error) {
	if p.log == nil {
		return
	}
	if err != nil {
		p.log.Warn(msg, "err", p.rd.String(err.Error()))
		return
	}
	p.log.Warn(msg)
}

// usageSink adapts the proxy.UsageTracker interface to the *usage.Tracker plus
// the host's _arvel/usage_updated push (the SDK throttles it to 5 s).
type usageSink struct {
	t *usage.Tracker
	p *Provider
}

func (s usageSink) Add(tu protocol.TurnUsage, currency string) {
	if s.t == nil {
		return
	}
	if err := s.t.Add(tu, currency); err != nil && s.p.log != nil {
		s.p.log.Warn("cannot persist usage ledger", "err", err.Error())
	}
	s.p.pushUsage()
}

func (s usageSink) RateLimited() {
	if s.t == nil {
		return
	}
	s.t.MarkRateLimited()
	s.p.pushUsage()
}

func (s usageSink) MarkOK() {
	if s.t == nil {
		return
	}
	s.t.MarkOK()
	s.p.pushUsage()
}

// pushUsage sends _arvel/usage_updated with the current snapshot, if the host
// is attached.
func (p *Provider) pushUsage() {
	p.mu.RLock()
	h, t := p.host, p.tracker
	p.mu.RUnlock()
	if h == nil || t == nil {
		return
	}
	h.UsageUpdated(t.Snapshot())
}

// usageSink returns the adapter for a new session, or nil when the tracker is
// not yet initialized.
func (p *Provider) usageSink() proxy.UsageTracker {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.tracker == nil {
		return nil
	}
	return usageSink{t: p.tracker, p: p}
}
