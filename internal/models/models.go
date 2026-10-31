// SPDX-License-Identifier: Apache-2.0

// Package models maps the opencode CLI catalogue to Arvel protocol models.
package models

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

// EmptyID is the id of the hidden placeholder model returned by Empty.
const EmptyID = "opencode/unavailable"

// v2Efforts is the global reasoning-effort vocabulary OpenCode v2 advertises
// via the `effort` config option (FINDINGS.md V2): low..max. `default` is
// "model default" and maps to "no explicit effort", so it is not offered.
var v2Efforts = []protocol.Effort{
	protocol.EffortLow, protocol.EffortMedium, protocol.EffortHigh,
	protocol.EffortXHigh, protocol.EffortMax,
}

// effortOrder keeps mapped efforts in the fixed protocol order.
var effortOrder = map[protocol.Effort]int{
	protocol.EffortLow:    0,
	protocol.EffortMedium: 1,
	protocol.EffortHigh:   2,
	protocol.EffortXHigh:  3,
	protocol.EffortMax:    4,
}

// apiProviders are providers billed per API call. Everything else is
// "unknown" unless a clearer signal exists (documented in the README).
var apiProviders = map[string]bool{
	"deepseek":   true,
	"openrouter": true,
	"moonshot":   true,
	"zai":        true,
	"z":          true,
	"groq":       true,
}

// Empty returns the not-installed fallback: a single hidden placeholder, so the
// result always passes protocol.ModelsResult.Validate (an empty list is
// invalid). The host never creates sessions while status is not_installed.
func Empty() protocol.ModelsResult {
	return protocol.ModelsResult{
		Models: []protocol.Model{{
			ID:      EmptyID,
			Label:   "OpenCode not detected",
			Billing: protocol.BillingUnknown,
			Hidden:  true,
		}},
		Default: EmptyID,
	}
}

// Build maps raw CLI models plus a flavour to a validated ModelsResult. Models
// are sorted by ID for determinism; Default is the first model.
func Build(raw []detect.RawModel, f detect.Flavour) (protocol.ModelsResult, error) {
	seen := map[string]bool{}
	out := make([]protocol.Model, 0, len(raw))
	for _, r := range raw {
		if r.ID == "" || !strings.Contains(r.ID, "/") || seen[r.ID] {
			continue
		}
		seen[r.ID] = true

		_, slug, _ := strings.Cut(r.ID, "/")
		label := fmt.Sprintf("%s · %s", slug, r.Provider)
		if r.Name != "" {
			label = fmt.Sprintf("%s · %s", r.Name, r.Provider)
		}
		inputs := []string{"text"}
		if r.InputImage {
			inputs = []string{"text", "image"}
		}
		out = append(out, protocol.Model{
			ID:            r.ID,
			Label:         label,
			Efforts:       effortsFor(r, f),
			ContextWindow: r.ContextWindow,
			Inputs:        inputs,
			Billing:       billingFor(r.Provider),
			Hidden:        false,
		})
	}
	if len(out) == 0 {
		return protocol.ModelsResult{}, errors.New("models: no valid models")
	}

	slices.SortFunc(out, func(a, b protocol.Model) int { return strings.Compare(a.ID, b.ID) })
	result := protocol.ModelsResult{Models: out, Default: out[0].ID}
	if err := result.Validate(); err != nil {
		return protocol.ModelsResult{}, fmt.Errorf("models: invalid catalogue: %w", err)
	}
	return result, nil
}

// effortsFor maps a raw model's variants (v1) or the global vocabulary (v2) to
// the fixed protocol effort list, dropping non-vocabulary names.
func effortsFor(m detect.RawModel, f detect.Flavour) []protocol.Effort {
	if f == detect.V2 {
		return slices.Clone(v2Efforts)
	}
	var out []protocol.Effort
	for name := range m.Variants {
		e := protocol.Effort(name)
		if e.Valid() { // drops none/minimal/… (not in the enum)
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b protocol.Effort) int { return effortOrder[a] - effortOrder[b] })
	return out
}

// billingFor maps a provider id to a per-model billing mode.
func billingFor(provider string) protocol.Billing {
	if apiProviders[provider] {
		return protocol.BillingAPI
	}
	return protocol.BillingUnknown
}

// Cache is a 6-hour complete-catalogue cache keyed by flavour. Its owner (the
// provider) serializes all access.
type Cache struct {
	value   protocol.ModelsResult
	at      time.Time
	flavour detect.Flavour
}

// Get returns a defensive copy when the cached catalogue matches the flavour
// and is younger than 6 hours, unless refresh is set.
func (c *Cache) Get(now time.Time, f detect.Flavour, refresh bool) (protocol.ModelsResult, bool) {
	if refresh || c.at.IsZero() || now.Before(c.at) || now.Sub(c.at) >= 6*time.Hour || c.flavour != f {
		return protocol.ModelsResult{}, false
	}
	return clone(c.value), true
}

// Put publishes a complete validated catalogue with no shared mutable slices.
func (c *Cache) Put(now time.Time, f detect.Flavour, v protocol.ModelsResult) {
	c.at = now
	c.flavour = f
	c.value = clone(v)
}

func clone(v protocol.ModelsResult) protocol.ModelsResult {
	v.Models = slices.Clone(v.Models)
	for i := range v.Models {
		m := &v.Models[i]
		m.Efforts = slices.Clone(m.Efforts)
		m.Inputs = slices.Clone(m.Inputs)
		m.Aliases = slices.Clone(m.Aliases)
		m.RecommendedFor = slices.Clone(m.RecommendedFor)
	}
	return v
}
