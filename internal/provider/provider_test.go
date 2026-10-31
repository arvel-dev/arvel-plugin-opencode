// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-plugin-opencode/internal/models"
	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

type fakeDetector struct {
	info      detect.Info
	detectErr error
	providers []detect.Provider
	authErr   error
	v1models  []detect.RawModel
	v2models  []detect.RawModel
	modelsErr error
}

func (f *fakeDetector) Detect(context.Context) (detect.Info, error) { return f.info, f.detectErr }

func (f *fakeDetector) Auth(context.Context, detect.Flavour) ([]detect.Provider, error) {
	return f.providers, f.authErr
}

func (f *fakeDetector) ModelsV1(context.Context) ([]detect.RawModel, error) {
	return f.v1models, f.modelsErr
}

func (f *fakeDetector) ModelsV2(context.Context) ([]detect.RawModel, error) {
	return f.v2models, f.modelsErr
}

func newTestProvider(fd *fakeDetector, bin string) *Provider {
	p := New()
	p.newDetector = func(string) detector { return fd }
	p.bin = bin
	return p
}

func v1Info() detect.Info {
	return detect.Info{Bin: "/x/opencode", Version: detect.Version{Major: 1, Minor: 19, Patch: 5, Original: "1.19.5"}}
}

func v2Info() detect.Info {
	return detect.Info{Bin: "/x/opencode", Version: detect.Version{Major: 2, Minor: 2, Patch: 5, Original: "opencode v2.2.5"}}
}

func TestStatus_NotInstalled(t *testing.T) {
	t.Parallel()

	st, err := newTestProvider(&fakeDetector{}, "").Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if err := st.Validate(); err != nil {
		t.Fatalf("status does not validate: %v", err)
	}
	if st.State != protocol.StateNotInstalled {
		t.Fatalf("state = %q, want not_installed", st.State)
	}
	if len(st.Actions) != 1 || st.Actions[0].ID != "install_help" {
		t.Fatalf("actions = %+v", st.Actions)
	}
}

func TestStatus_Outdated(t *testing.T) {
	t.Parallel()

	fd := &fakeDetector{info: detect.Info{Bin: "/x/opencode", Version: detect.Version{Major: 1, Minor: 3, Patch: 15, Original: "1.3.15"}}}
	st, err := newTestProvider(fd, "/x/opencode").Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if err := st.Validate(); err != nil {
		t.Fatalf("status does not validate: %v", err)
	}
	if st.State != protocol.StateOutdated {
		t.Fatalf("state = %q, want outdated", st.State)
	}
	if st.Binary == nil || st.Binary.Version != "1.3.15" {
		t.Fatalf("binary = %+v", st.Binary)
	}
}

func TestStatus_NeedsLogin(t *testing.T) {
	t.Parallel()

	fd := &fakeDetector{info: v1Info()}
	st, err := newTestProvider(fd, "/x/opencode").Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if err := st.Validate(); err != nil {
		t.Fatalf("status does not validate: %v", err)
	}
	if st.State != protocol.StateNeedsLogin {
		t.Fatalf("state = %q, want needs_login", st.State)
	}
	if len(st.Actions) != 1 || st.Actions[0].ID != "login" {
		t.Fatalf("actions = %+v", st.Actions)
	}
}

func TestStatus_Ready(t *testing.T) {
	t.Parallel()

	fd := &fakeDetector{info: v1Info(), providers: []detect.Provider{{Name: "DeepSeek", ID: "DeepSeek"}}}
	st, err := newTestProvider(fd, "/x/opencode").Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if err := st.Validate(); err != nil {
		t.Fatalf("status does not validate: %v", err)
	}
	if st.State != protocol.StateReady {
		t.Fatalf("state = %q, want ready", st.State)
	}
	if st.Message != "1 provider configured" {
		t.Fatalf("message = %q", st.Message)
	}
}

func TestStatus_Ready_KeylessLocal(t *testing.T) {
	t.Parallel()

	// No stored credentials, but the catalogue is non-empty (e.g. Ollama):
	// SPEC §9.1 counts a keyless local provider as ready.
	fd := &fakeDetector{
		info:     v1Info(),
		v1models: []detect.RawModel{{ID: "ollama/llama3", Provider: "ollama"}},
	}
	st, err := newTestProvider(fd, "/x/opencode").Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if err := st.Validate(); err != nil {
		t.Fatalf("status does not validate: %v", err)
	}
	if st.State != protocol.StateReady {
		t.Fatalf("state = %q, want ready (keyless local provider)", st.State)
	}
	if st.Message != "OpenCode is ready" {
		t.Fatalf("message = %q", st.Message)
	}
}

func TestStatus_DetectError(t *testing.T) {
	t.Parallel()

	fd := &fakeDetector{detectErr: errors.New("boom")}
	st, err := newTestProvider(fd, "/x/opencode").Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.State != protocol.StateError {
		t.Fatalf("state = %q, want error", st.State)
	}
}

func TestModels_NotInstalled(t *testing.T) {
	t.Parallel()

	got, err := newTestProvider(&fakeDetector{}, "").Models(context.Background(), false)
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("models do not validate: %v", err)
	}
	want := models.Empty()
	if len(got.Models) != len(want.Models) || got.Default != want.Default {
		t.Fatalf("Models = %+v, want Empty()", got)
	}
}

func TestModels_V2BuildsAndCaches(t *testing.T) {
	t.Parallel()

	fd := &fakeDetector{info: v2Info(), v2models: []detect.RawModel{{ID: "deepseek/deepseek-flash", Provider: "deepseek"}}}
	p := newTestProvider(fd, "/x/opencode")

	first, err := p.Models(context.Background(), false)
	if err != nil {
		t.Fatalf("first Models: %v", err)
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("first models do not validate: %v", err)
	}
	if len(first.Models) != 1 {
		t.Fatalf("first models = %d, want 1", len(first.Models))
	}

	// Mutating the source must not affect a cache hit.
	fd.v2models = nil
	second, err := p.Models(context.Background(), false)
	if err != nil {
		t.Fatalf("second Models: %v", err)
	}
	if len(second.Models) != 1 || second.Default != first.Default {
		t.Fatalf("cache miss: second = %+v", second)
	}
}

func TestModels_V1Builds(t *testing.T) {
	t.Parallel()

	fd := &fakeDetector{
		info: v1Info(),
		v1models: []detect.RawModel{{
			ID:       "deepseek/deepseek-v4-pro",
			Provider: "deepseek",
			Name:     "DeepSeek V4 Pro",
			Variants: map[string]any{"low": nil, "high": nil, "none": nil},
		}},
	}
	got, err := newTestProvider(fd, "/x/opencode").Models(context.Background(), false)
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("models do not validate: %v", err)
	}
	if len(got.Models) != 1 || got.Models[0].Label != "DeepSeek V4 Pro · deepseek" {
		t.Fatalf("models = %+v", got.Models)
	}
	if len(got.Models[0].Efforts) != 2 {
		t.Fatalf("efforts = %v, want [low high]", got.Models[0].Efforts)
	}
}

func TestModels_EmptyDegrades(t *testing.T) {
	t.Parallel()

	fd := &fakeDetector{info: v2Info()}
	got, err := newTestProvider(fd, "/x/opencode").Models(context.Background(), false)
	if err != nil {
		t.Fatalf("Models returned an error, want a degraded empty catalogue: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("degraded catalogue does not validate: %v", err)
	}
	if len(got.Models) != 1 || !got.Models[0].Hidden {
		t.Fatalf("Models = %+v, want the hidden placeholder", got)
	}
}

func TestAction_Login(t *testing.T) {
	t.Parallel()

	p := newTestProvider(&fakeDetector{}, "/x/opencode")
	got, err := p.Action(context.Background(), "login")
	if err != nil {
		t.Fatalf("Action: %v", err)
	}
	if got.Kind != protocol.ActionKindTerminal || len(got.Command) != 3 || got.Command[0] != "/x/opencode" || got.Command[1] != "auth" || got.Command[2] != "login" {
		t.Fatalf("action = %+v", got)
	}
}

func TestAction_InstallHelp(t *testing.T) {
	t.Parallel()

	got, err := newTestProvider(&fakeDetector{}, "").Action(context.Background(), "install_help")
	if err != nil {
		t.Fatalf("Action: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("action does not validate: %v", err)
	}
	if got.Kind != protocol.ActionKindURL || got.URL != installDocsURL {
		t.Fatalf("action = %+v", got)
	}
}

func TestAction_Unknown(t *testing.T) {
	t.Parallel()

	_, err := newTestProvider(&fakeDetector{}, "").Action(context.Background(), "bogus")
	var rpcErr *protocol.RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error = %v, want *protocol.RPCError", err)
	}
}

func TestNewSessionNotImplemented(t *testing.T) {
	t.Parallel()

	_, err := New().NewSession(context.Background(), plugin.SessionParams{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("NewSession error = %v, want ErrNotImplemented", err)
	}
}
