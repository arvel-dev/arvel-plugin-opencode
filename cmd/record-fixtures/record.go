// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// frame is one sanitized ACP JSONL line.
type frame struct {
	Direction string          `json:"direction"` // client_to_server | server_to_client
	Message   json.RawMessage `json:"message"`
}

// Metadata describes one recording.
type Metadata struct {
	Scenario string            `json:"scenario"`
	Label    string            `json:"label"`
	Bin      string            `json:"bin"`
	Version  string            `json:"version"` // opencode --version output
	OS       string            `json:"os"`
	Arch     string            `json:"arch"`
	Captured string            `json:"captured"` // RFC3339
	Env      map[string]string `json:"env,omitempty"`
}

// indexEntry is one row of testdata/fixtures/index.json.
type indexEntry struct {
	Label    string `json:"label"`
	Scenario string `json:"scenario"`
	Path     string `json:"path"` // relative to the fixtures root
	Version  string `json:"version"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Captured string `json:"captured"`
}

// recordEnv is what a scenario function receives.
type recordEnv struct {
	Bin           string
	Dir           string
	Args          []string
	Prompt        string
	Model         string
	Effort        string
	ACPStandalone bool
	CLIStandalone bool
	AutoApprove   bool
	ApproveAlways bool
	CWD           string
}

// scenarioFunc captures a scenario into r.Dir and returns its metadata.
type scenarioFunc func(ctx context.Context, r recordEnv) (Metadata, error)

var scenarios = map[string]scenarioFunc{
	"version":      scenarioVersion,
	"models":       scenarioModels,
	"auth-list":    scenarioAuthList,
	"debug-paths":  scenarioDebugPaths,
	"debug-config": scenarioDebugConfig,
	"debug-agents": scenarioDebugAgents,
	"debug-skill":  scenarioDebugSkill,
	"debug-help":   scenarioDebugHelp,
	"handshake":    scenarioHandshake,
	"session-new":  scenarioSessionNew,
	"resume":       scenarioResume,
	"prompt":       scenarioPrompt,
	"config-error": scenarioConfigError,
	"effort-error": scenarioEffortError,
}

func scenarioNames() []string {
	names := make([]string, 0, len(scenarios))
	for name := range scenarios {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func runRecord(args []string) error {
	fs := flag.NewFlagSet("record", flag.ExitOnError)
	scenario := fs.String("scenario", "", "scenario name to record")
	bin := fs.String("bin", "", "absolute path to opencode (else $PATH)")
	label := fs.String("label", "", "output directory name (default: scenario)")
	out := fs.String("out", "testdata/fixtures", "fixtures root")
	timeout := fs.Duration("timeout", 60*time.Second, "per-scenario timeout")
	extra := fs.String("args", "", "extra CLI arguments for CLI scenarios, space separated")
	prompt := fs.String("prompt", "Reply with exactly: pong", "prompt text for the prompt scenario")
	model := fs.String("model", "", "model id for the prompt scenario")
	effort := fs.String("effort", "", "effort value for the prompt scenario")
	acpStandalone := fs.Bool("acp-standalone", false, "run \"opencode --standalone acp\" instead of \"opencode acp\"")
	cliStandalone := fs.Bool("cli-standalone", false, "prepend --standalone to CLI scenarios")
	autoApprove := fs.Bool("approve", false, "auto-approve ACP permission requests with allow_once")
	approveAlways := fs.Bool("approve-always", false, "auto-approve ACP permission requests with allow_always (implies --approve)")
	cwd := fs.String("cwd", "", "working directory for ACP session/new (default: the fixture dir)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *scenario == "" {
		return fmt.Errorf("--scenario is required (one of: %s)", strings.Join(scenarioNames(), ", "))
	}
	fn, ok := scenarios[*scenario]
	if !ok {
		return fmt.Errorf("unknown scenario %q (available: %s)", *scenario, strings.Join(scenarioNames(), ", "))
	}
	resolved := *bin
	if resolved == "" {
		resolved = envBin()
	}
	if resolved == "" {
		return fmt.Errorf("opencode not found: pass --bin or set ARVEL_OPENCODE_BIN and ARVEL_DEV=1")
	}
	name := *label
	if name == "" {
		name = *scenario
	}
	root, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	md, err := fn(ctx, recordEnv{
		Bin:           resolved,
		Dir:           dir,
		Args:          strings.Fields(*extra),
		Prompt:        *prompt,
		Model:         *model,
		Effort:        *effort,
		ACPStandalone: *acpStandalone,
		CLIStandalone: *cliStandalone,
		AutoApprove:   *autoApprove || *approveAlways,
		ApproveAlways: *approveAlways,
		CWD:           *cwd,
	})
	if err != nil {
		return err
	}
	md.Scenario = *scenario
	md.Label = name
	md.Bin = resolved
	md.Version = versionOf(ctx, resolved)
	md.OS = runtime.GOOS
	md.Arch = runtime.GOARCH
	md.Captured = time.Now().UTC().Format(time.RFC3339)
	md.Env = opencodeEnv()

	if err := writeMetadata(dir, md); err != nil {
		return err
	}
	if err := updateIndex(root, md, dir); err != nil {
		return err
	}
	fmt.Printf("recorded %s (opencode %s, %s/%s)\n", dir, md.Version, md.OS, md.Arch)
	return nil
}

// envBin resolves the opencode binary: an explicit ARVEL_OPENCODE_BIN is only
// honoured in development (ARVEL_DEV=1), everything else uses $PATH.
func envBin() string {
	if os.Getenv("ARVEL_DEV") == "1" {
		if p := os.Getenv("ARVEL_OPENCODE_BIN"); p != "" {
			return p
		}
	}
	p, err := exec.LookPath("opencode")
	if err != nil {
		return ""
	}
	return p
}

// versionOf returns the trimmed `opencode --version` output, or "" on failure.
func versionOf(ctx context.Context, bin string) string {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(c, bin, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// opencodeEnv records the OPENCODE_* variables that shaped a recording. They
// select config files and switches, never credentials.
func opencodeEnv() map[string]string {
	out := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(k, "OPENCODE_CONFIG") || strings.HasPrefix(k, "OPENCODE_DISABLE_") ||
			strings.HasPrefix(k, "XDG_") ||
			k == "HTTPS_PROXY" || k == "HTTP_PROXY" || k == "ALL_PROXY" || k == "NO_PROXY" {
			out[k] = sanitizeEnvValue(v)
		}
	}
	return out
}

// sanitizeEnvValue redacts an environment value before it is stored in
// metadata.json. JSON values (OPENCODE_CONFIG_CONTENT) are walked so
// secret-named fields such as provider apiKey are removed; anything else is
// scrubbed as text.
func sanitizeEnvValue(v string) string {
	if json.Valid([]byte(v)) {
		return string(sanitize([]byte(v)))
	}
	return sanitizeText(v)
}

func writeMetadata(dir string, md Metadata) error {
	data, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644)
}

func updateIndex(root string, md Metadata, dir string) error {
	path := filepath.Join(root, "index.json")
	var entries []indexEntry
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &entries); err != nil {
			return fmt.Errorf("%s is not a valid index: %w", path, err)
		}
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return err
	}
	entry := indexEntry{
		Label:    md.Label,
		Scenario: md.Scenario,
		Path:     filepath.ToSlash(rel),
		Version:  md.Version,
		OS:       md.OS,
		Arch:     md.Arch,
		Captured: md.Captured,
	}
	replaced := false
	for i := range entries {
		if entries[i].Label == entry.Label {
			entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Label < entries[j].Label })

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
