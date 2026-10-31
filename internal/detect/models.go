// SPDX-License-Identifier: Apache-2.0

package detect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// RawModel is one model as reported by the opencode CLI, before mapping to a
// protocol.Model. Name is empty for v2 (the plain listing has no display name).
type RawModel struct {
	ID            string         // provider/model
	Provider      string         // provider segment
	Name          string         // display name (v1 only)
	Variants      map[string]any // v1 variants map (key = effort name)
	ContextWindow int            // v1 limit.context, 0 when unknown
	InputImage    bool           // v1 capabilities.input.image
}

// ModelsV2 runs `opencode models` and parses one provider/model per line.
func (d *Detector) ModelsV2(ctx context.Context) ([]RawModel, error) {
	out, err := d.run(ctx, d.bin, "models")
	if err != nil {
		return nil, err
	}
	return parseModelsV2(string(out)), nil
}

func parseModelsV2(s string) []RawModel {
	var out []RawModel
	for _, ln := range strings.Split(s, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || !strings.Contains(t, "/") {
			continue
		}
		provider, model, ok := strings.Cut(t, "/")
		if !ok || provider == "" || model == "" {
			continue
		}
		out = append(out, RawModel{ID: t, Provider: provider})
	}
	return out
}

// ModelsV1 runs `opencode models --verbose` and parses each provider/model line
// plus the JSON object that follows it.
func (d *Detector) ModelsV1(ctx context.Context) ([]RawModel, error) {
	out, err := d.run(ctx, d.bin, "models", "--verbose")
	if err != nil {
		return nil, err
	}
	return parseModelsV1(string(out))
}

// v1VerboseModel is the JSON object printed after each provider/model line.
type v1VerboseModel struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
	Name       string `json:"name"`
	Limit      struct {
		Context int `json:"context"`
	} `json:"limit"`
	Caps struct {
		Input map[string]bool `json:"input"`
	} `json:"capabilities"`
	Variants map[string]any `json:"variants"`
}

func parseModelsV1(s string) ([]RawModel, error) {
	lines := strings.Split(s, "\n")
	var out []RawModel
	var header string
	var buf []string

	flush := func() error {
		if header == "" {
			return nil
		}
		m, err := parseV1Block(header, buf)
		if err != nil {
			return err
		}
		out = append(out, m)
		return nil
	}

	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if isModelHeader(t) {
			if err := flush(); err != nil {
				return nil, err
			}
			header = t
			buf = nil
			continue
		}
		if header != "" {
			buf = append(buf, ln)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}

// isModelHeader reports whether t is a "provider/model" header line. JSON body
// lines contain quotes, braces, brackets or spaces and never match.
func isModelHeader(t string) bool {
	if t == "" || !strings.Contains(t, "/") {
		return false
	}
	return !strings.ContainsAny(t, " \t{}[]\":,")
}

func parseV1Block(header string, buf []string) (RawModel, error) {
	provider, _, _ := strings.Cut(header, "/")
	var vm v1VerboseModel
	if err := json.Unmarshal([]byte(strings.Join(buf, "\n")), &vm); err != nil {
		return RawModel{}, fmt.Errorf("detect: parse v1 model %s: %w", header, err)
	}
	return RawModel{
		ID:            header,
		Provider:      provider,
		Name:          vm.Name,
		Variants:      vm.Variants,
		ContextWindow: vm.Limit.Context,
		InputImage:    vm.Caps.Input["image"],
	}, nil
}
