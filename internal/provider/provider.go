// SPDX-License-Identifier: Apache-2.0

// Package provider implements the plugin.Provider for OpenCode. O1 wires
// detection, the model catalogue, status and actions; sessions and usage
// arrive in O2–O4.
package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-plugin-opencode/internal/models"
	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
	"github.com/arvel-dev/arvel-sdk/redact"
)

// ErrNotImplemented marks provider surfaces that arrive in later milestones.
var ErrNotImplemented = errors.New("not implemented until a later milestone")

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
	info        *detect.Info              // cached Detect result (the binary is fixed per process)
	cache       models.Cache              // model catalogue cache
	newDetector func(bin string) detector // test seam; defaults to detect.New
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

// Initialize resolves the binary path from the connection config. It does not
// run opencode (that is Status/Models work).
func (p *Provider) Initialize(_ context.Context, meta protocol.InitMeta) (plugin.Features, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.info = nil
	if s, ok := meta.Connection.Config["opencodePath"].(string); ok {
		p.cfgPath = s
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
	return plugin.Features{LoadSession: false}, nil
}

// Status reports install/version/login readiness (SPEC.md §9.1).
func (p *Provider) Status(ctx context.Context) (protocol.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, rpcBudget)
	defer cancel()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.bin == "" {
		return notInstalledStatus(), nil
	}

	info, err := p.detectLocked(ctx)
	if err != nil {
		p.warn("cannot detect opencode", err)
		return protocol.Status{State: protocol.StateError, Message: "Cannot detect OpenCode"}, nil
	}
	binInfo := &protocol.BinaryInfo{Name: "opencode", Path: p.rd.String(p.bin), Version: info.Version.String()}
	if info.Version.IsOutdated() {
		return protocol.Status{
			State:   protocol.StateOutdated,
			Message: "OpenCode " + info.Version.String() + " is older than " + detect.MinVersion,
			Binary:  binInfo,
		}, nil
	}

	det := p.newDetector(p.bin)
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
	if res := p.catalogueLocked(ctx, info.Flavour(), false); res.Default != models.EmptyID {
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

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.bin == "" {
		return models.Empty(), nil
	}
	info, err := p.detectLocked(ctx)
	if err != nil {
		p.warn("cannot detect opencode for the model catalogue", err)
		return models.Empty(), nil
	}
	return p.catalogueLocked(ctx, info.Flavour(), refresh), nil
}

// rpcBudget bounds one Status/Models call below the host's 10 s request
// deadline, including a keyless-provider catalogue probe.
const rpcBudget = 9 * time.Second

// detectLocked returns the cached Detect result, spawning --version once.
func (p *Provider) detectLocked(ctx context.Context) (detect.Info, error) {
	if p.info != nil {
		return *p.info, nil
	}
	det := p.newDetector(p.bin)
	info, err := det.Detect(ctx)
	if err != nil {
		return detect.Info{}, err
	}
	p.info = &info
	return info, nil
}

// catalogueLocked returns a valid catalogue: the cache, a fresh build, or the
// hidden placeholder on any fetch/build failure (PROTOCOL §6.2 requires a
// non-empty list; the host gates on status).
func (p *Provider) catalogueLocked(ctx context.Context, f detect.Flavour, refresh bool) protocol.ModelsResult {
	if cached, ok := p.cache.Get(p.now(), f, refresh); ok {
		if p.log != nil {
			p.log.Debug("model catalog cache hit", "flavour", f)
		}
		return cached
	}
	det := p.newDetector(p.bin)
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

// Usage reports unknown usage: OpenCode spend tracking arrives in O4.
func (p *Provider) Usage(context.Context) (protocol.Usage, error) {
	return protocol.Usage{Source: protocol.UsageSourceUnknown, Status: protocol.UsageStatusOK}, nil
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

// NewSession always fails: session support arrives in O2.
func (p *Provider) NewSession(context.Context, plugin.SessionParams) (plugin.Session, error) {
	return nil, ErrNotImplemented
}

// LoadSession always fails: LoadSession is not announced in O1.
func (p *Provider) LoadSession(context.Context, plugin.LoadParams) (plugin.Session, error) {
	return nil, ErrNotImplemented
}

// Shutdown releases nothing: no child processes exist in O1.
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
