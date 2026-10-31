// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
)

func TestTranslate_ModelValue(t *testing.T) {
	cases := []struct {
		name   string
		f      detect.Flavour
		model  string
		effort string
		want   string
	}{
		{"v1 no effort", detect.V1, "a/b", "", "a/b"},
		{"v1 default effort", detect.V1, "a/b", "default", "a/b"},
		{"v1 high effort", detect.V1, "a/b", "high", "a/b/high"},
		{"v2 keeps bare model", detect.V2, "a/b", "high", "a/b"},
		{"empty model", detect.V1, "", "high", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := modelValue(c.f, c.model, c.effort); got != c.want {
				t.Fatalf("modelValue = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTranslate_ModeValue(t *testing.T) {
	if got := modeValue(""); got != "build" {
		t.Fatalf("modeValue(\"\") = %q", got)
	}
	if got := modeValue("plan"); got != "plan" {
		t.Fatalf("modeValue(plan) = %q", got)
	}
}

func TestTranslate_HasOption(t *testing.T) {
	opts := []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{Id: "model"}},
		{Boolean: &acp.SessionConfigOptionBoolean{Id: "flag"}},
	}
	if !hasOption(opts, "model") {
		t.Fatal("want model present")
	}
	if hasOption(opts, "flag") {
		t.Fatal("boolean option must not count as a select")
	}
	if hasOption(opts, "mode") {
		t.Fatal("want mode absent")
	}
}
