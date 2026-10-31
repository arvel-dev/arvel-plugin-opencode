// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitize_RedactsSecretKeys(t *testing.T) {
	in := []byte(`{"apiKey":"abc","Authorization":"Bearer zzz","nested":{"token":"x"},"name":"OpenCode"}`)
	var got map[string]any
	if err := json.Unmarshal(sanitize(in), &got); err != nil {
		t.Fatalf("sanitize returned invalid JSON: %v", err)
	}
	if got["apiKey"] != "REDACTED" || got["Authorization"] != "REDACTED" {
		t.Fatalf("top-level secrets not redacted: %v", got)
	}
	nested, _ := got["nested"].(map[string]any)
	if nested == nil || nested["token"] != "REDACTED" {
		t.Fatalf("nested secret not redacted: %v", got)
	}
	if got["name"] != "OpenCode" {
		t.Fatalf("non-secret value changed: %v", got["name"])
	}
}

func TestSanitize_RedactsSkValues(t *testing.T) {
	got := string(sanitize([]byte(`{"note":"key sk-abcdefghijklmnop1234"}`)))
	if strings.Contains(got, "abcdefghijklmnop") {
		t.Fatalf("sk- value leaked: %s", got)
	}
	if !strings.Contains(got, "sk-REDACTED") {
		t.Fatalf("sk- value not replaced: %s", got)
	}
}

func TestSanitizeEnvValue_RedactsJSONSecret(t *testing.T) {
	in := `{"provider":{"key":"abc"},"permission":{"edit":"deny"}}`
	got := sanitizeEnvValue(in)
	if strings.Contains(got, `"abc"`) {
		t.Fatalf("provider key leaked in env value: %s", got)
	}
	if !strings.Contains(got, `"edit":"deny"`) {
		t.Fatalf("non-secret config lost: %s", got)
	}
}

func TestSanitize_PassesNonSecret(t *testing.T) {
	in := []byte(`{"name":"OpenCode","version":"2.2.5"}`)
	got := string(sanitize(in))
	if !strings.Contains(got, "OpenCode") || !strings.Contains(got, "2.2.5") {
		t.Fatalf("non-secret content changed: %s", got)
	}
}
