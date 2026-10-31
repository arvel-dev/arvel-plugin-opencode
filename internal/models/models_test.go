// SPDX-License-Identifier: Apache-2.0

package models

import (
	"testing"
	"time"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

func effortStrings(e []protocol.Effort) []string {
	out := make([]string, len(e))
	for i := range e {
		out[i] = string(e[i])
	}
	return out
}

func TestBuild_V1(t *testing.T) {
	t.Parallel()

	raw := []detect.RawModel{
		{
			ID:            "deepseek/deepseek-v4-pro",
			Provider:      "deepseek",
			Name:          "DeepSeek V4 Pro",
			Variants:      map[string]any{"low": nil, "high": nil, "max": nil},
			ContextWindow: 1048576,
		},
		{
			ID:            "opencode/beam-free",
			Provider:      "opencode",
			Name:          "Beam Free",
			Variants:      map[string]any{"none": nil, "minimal": nil, "low": nil, "medium": nil, "high": nil, "xhigh": nil, "max": nil},
			ContextWindow: 1048576,
		},
	}
	got, err := Build(raw, detect.V1)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("Build result does not validate: %v", err)
	}
	if got.Default != "deepseek/deepseek-v4-pro" {
		t.Fatalf("Default = %q, want the lexicographically-first id", got.Default)
	}

	byID := map[string]protocol.Model{}
	for _, m := range got.Models {
		byID[m.ID] = m
	}
	pro := byID["deepseek/deepseek-v4-pro"]
	if pro.Label != "DeepSeek V4 Pro · deepseek" {
		t.Fatalf("label = %q", pro.Label)
	}
	if want := []string{"low", "high", "max"}; !equal(effortStrings(pro.Efforts), want) {
		t.Fatalf("efforts = %v, want %v", effortStrings(pro.Efforts), want)
	}
	if pro.ContextWindow != 1048576 || len(pro.Inputs) != 1 || pro.Inputs[0] != "text" {
		t.Fatalf("pro = %+v", pro)
	}
	if pro.Billing != protocol.BillingAPI {
		t.Fatalf("deepseek billing = %q, want api", pro.Billing)
	}

	beam := byID["opencode/beam-free"]
	if want := []string{"low", "medium", "high", "xhigh", "max"}; !equal(effortStrings(beam.Efforts), want) {
		t.Fatalf("beam efforts = %v, want %v (none/minimal dropped)", effortStrings(beam.Efforts), want)
	}
	if beam.Billing != protocol.BillingUnknown {
		t.Fatalf("opencode billing = %q, want unknown", beam.Billing)
	}
}

func TestBuild_V2(t *testing.T) {
	t.Parallel()

	raw := []detect.RawModel{
		{ID: "deepseek/deepseek-flash", Provider: "deepseek"},
		{ID: "opencode/beam-free", Provider: "opencode"},
	}
	got, err := Build(raw, detect.V2)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("Build result does not validate: %v", err)
	}
	for _, m := range got.Models {
		if want := []string{"low", "medium", "high", "xhigh", "max"}; !equal(effortStrings(m.Efforts), want) {
			t.Fatalf("%s efforts = %v, want %v", m.ID, effortStrings(m.Efforts), want)
		}
		if m.ContextWindow != 0 {
			t.Fatalf("%s contextWindow = %d, want 0", m.ID, m.ContextWindow)
		}
	}
	if got.Models[0].Label != "deepseek-flash · deepseek" {
		t.Fatalf("label = %q, want slug form", got.Models[0].Label)
	}
}

func TestBuild_DuplicateIDsDropped(t *testing.T) {
	t.Parallel()

	got, err := Build([]detect.RawModel{
		{ID: "opencode/beam-free", Provider: "opencode"},
		{ID: "opencode/beam-free", Provider: "opencode"},
	}, detect.V2)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Models) != 1 {
		t.Fatalf("models = %d, want 1", len(got.Models))
	}
}

func TestBuild_NoModels(t *testing.T) {
	t.Parallel()

	if _, err := Build(nil, detect.V2); err == nil {
		t.Fatal("Build(nil) succeeded, want an error")
	}
}

func TestEmpty(t *testing.T) {
	t.Parallel()

	got := Empty()
	if err := got.Validate(); err != nil {
		t.Fatalf("Empty does not validate: %v", err)
	}
	if len(got.Models) != 1 || !got.Models[0].Hidden || got.Models[0].ID != EmptyID {
		t.Fatalf("Empty = %+v", got)
	}
}

func TestEffortsFor_V1_DropsNoneMinimal(t *testing.T) {
	t.Parallel()

	got := effortsFor(detect.RawModel{Variants: map[string]any{"none": nil, "minimal": nil, "low": nil, "high": nil}}, detect.V1)
	if want := []string{"low", "high"}; !equal(effortStrings(got), want) {
		t.Fatalf("efforts = %v, want %v", effortStrings(got), want)
	}
}

func TestEffortsFor_V2_Global(t *testing.T) {
	t.Parallel()

	got := effortsFor(detect.RawModel{}, detect.V2)
	if want := []string{"low", "medium", "high", "xhigh", "max"}; !equal(effortStrings(got), want) {
		t.Fatalf("efforts = %v, want %v", effortStrings(got), want)
	}
}

func TestBillingFor(t *testing.T) {
	t.Parallel()

	cases := map[string]protocol.Billing{
		"deepseek":   protocol.BillingAPI,
		"openrouter": protocol.BillingAPI,
		"opencode":   protocol.BillingUnknown,
		"ollama":     protocol.BillingUnknown,
	}
	for provider, want := range cases {
		if got := billingFor(provider); got != want {
			t.Fatalf("billingFor(%q) = %q, want %q", provider, got, want)
		}
	}
}

func TestCache_FlavourKeyed(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var c Cache
	a, err := Build([]detect.RawModel{{ID: "opencode/a", Provider: "opencode"}}, detect.V1)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	c.Put(now, detect.V1, a)

	if _, ok := c.Get(now, detect.V2, false); ok {
		t.Fatal("Get(V2) hit a V1 entry")
	}
	got, ok := c.Get(now, detect.V1, false)
	if !ok {
		t.Fatal("Get(V1) missed")
	}
	if len(got.Models) != 1 || got.Models[0].ID != "opencode/a" {
		t.Fatalf("cached = %+v", got)
	}
}

func TestCache_ExpiryAndRefresh(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var c Cache
	r, err := Build([]detect.RawModel{{ID: "opencode/a", Provider: "opencode"}}, detect.V2)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	c.Put(now, detect.V2, r)

	if _, ok := c.Get(now.Add(6*time.Hour), detect.V2, false); ok {
		t.Fatal("Get hit past the 6-hour TTL")
	}
	if _, ok := c.Get(now, detect.V2, true); ok {
		t.Fatal("Get hit despite refresh=true")
	}
}

func TestCache_Clone(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var c Cache
	r, err := Build([]detect.RawModel{{ID: "opencode/a", Provider: "opencode"}}, detect.V2)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	c.Put(now, detect.V2, r)

	first, ok := c.Get(now, detect.V2, false)
	if !ok {
		t.Fatal("Get missed")
	}
	first.Models[0].ID = "mutated"
	second, ok := c.Get(now, detect.V2, false)
	if !ok || second.Models[0].ID != "opencode/a" {
		t.Fatalf("cache is not isolated: %+v", second)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
