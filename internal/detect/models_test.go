// SPDX-License-Identifier: Apache-2.0

package detect

import "testing"

func findRaw(t *testing.T, raw []RawModel, id string) RawModel {
	t.Helper()
	for _, m := range raw {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("model %q not found in %+v", id, raw)
	return RawModel{}
}

func TestParseModelsV2(t *testing.T) {
	t.Parallel()

	got := parseModelsV2(stdoutOf(t, "v2-models"))
	if len(got) != 10 {
		t.Fatalf("models = %d, want 10", len(got))
	}
	if got[0].ID != "deepseek/deepseek-flash" || got[0].Provider != "deepseek" || got[0].Name != "" {
		t.Fatalf("first model = %+v, want {ID:deepseek/deepseek-flash Provider:deepseek Name:}", got[0])
	}
	for _, m := range got {
		if m.Provider == "" || m.ID == "" {
			t.Fatalf("incomplete model: %+v", m)
		}
	}
}

func TestParseModelsV1(t *testing.T) {
	t.Parallel()

	got, err := parseModelsV1(stdoutOf(t, "v1-models-verbose"))
	if err != nil {
		t.Fatalf("parseModelsV1: %v", err)
	}
	if len(got) != 11 {
		t.Fatalf("models = %d, want 11", len(got))
	}

	beam := findRaw(t, got, "opencode/beam-free")
	if beam.Name != "Beam Free" || beam.Provider != "opencode" || beam.ContextWindow != 1048576 || beam.InputImage {
		t.Fatalf("beam-free = %+v", beam)
	}
	for _, key := range []string{"low", "medium", "high", "xhigh", "max"} {
		if _, ok := beam.Variants[key]; !ok {
			t.Fatalf("beam-free variants missing %q: %+v", key, beam.Variants)
		}
	}

	if bigPickle := findRaw(t, got, "opencode/big-pickle"); len(bigPickle.Variants) != 0 {
		t.Fatalf("big-pickle variants = %+v, want empty", bigPickle.Variants)
	}

	vl := findRaw(t, got, "opencode/nemotron-3.5-super-vl-free")
	if _, ok := vl.Variants["none"]; !ok {
		t.Fatalf("nemotron vl variants missing none: %+v", vl.Variants)
	}
	if _, ok := vl.Variants["minimal"]; !ok {
		t.Fatalf("nemotron vl variants missing minimal: %+v", vl.Variants)
	}

	pro := findRaw(t, got, "deepseek/deepseek-v4-pro")
	if pro.Name != "DeepSeek V4 Pro" || pro.Provider != "deepseek" || pro.ContextWindow != 1048576 {
		t.Fatalf("deepseek-v4-pro = %+v", pro)
	}
}

func TestParseModelsV1_Malformed(t *testing.T) {
	t.Parallel()

	_, err := parseModelsV1("opencode/broken\n{ invalid json\n")
	if err == nil {
		t.Fatal("parseModelsV1 accepted malformed JSON")
	}
}
