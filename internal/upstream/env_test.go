// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"strings"
	"testing"

	"github.com/arvel-dev/arvel-plugin-opencode/internal/detect"
)

func hasEnv(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}

func TestEnv_Defaults(t *testing.T) {
	env := Env(Config{})
	if !hasEnv(env, "TERM") || envValue(env, "TERM") != "dumb" {
		t.Fatalf("want TERM=dumb, got %q", envValue(env, "TERM"))
	}
	if !hasEnv(env, "OPENCODE_DISABLE_PROJECT_CONFIG") {
		t.Fatal("want OPENCODE_DISABLE_PROJECT_CONFIG for loadProjectConfig=false")
	}
}

func TestEnv_DisableProjectConfig(t *testing.T) {
	if !hasEnv(Env(Config{LoadProjectConfig: false}), "OPENCODE_DISABLE_PROJECT_CONFIG") {
		t.Fatal("false must set OPENCODE_DISABLE_PROJECT_CONFIG")
	}
	if hasEnv(Env(Config{LoadProjectConfig: true}), "OPENCODE_DISABLE_PROJECT_CONFIG") {
		t.Fatal("true must not set OPENCODE_DISABLE_PROJECT_CONFIG")
	}
}

func TestEnv_Proxy(t *testing.T) {
	const proxy = "http://127.0.0.1:41234"
	env := Env(Config{Proxy: proxy})
	for _, k := range []string{"HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY"} {
		if got := envValue(env, k); got != proxy {
			t.Fatalf("%s = %q, want %q", k, got, proxy)
		}
	}
	if got := envValue(env, "NO_PROXY"); got != "localhost,127.0.0.1" {
		t.Fatalf("NO_PROXY = %q", got)
	}
	if hasEnv(Env(Config{}), "HTTPS_PROXY") {
		t.Fatal("empty proxy must not set HTTPS_PROXY")
	}
}

func TestEnv_NeverPass(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	t.Setenv("GH_TOKEN", "x")
	t.Setenv("GITHUB_TOKEN", "y")
	t.Setenv("GIT_ASKPASS", "z")
	env := Env(Config{})
	for _, k := range []string{"SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN", "GIT_ASKPASS"} {
		if hasEnv(env, k) {
			t.Fatalf("%s leaked into the child env", k)
		}
	}
}

func TestEnv_InlineConfigAndSkillExclusion(t *testing.T) {
	v1 := Env(Config{Flavour: detect.V1, LoadProjectConfig: false, InlineConfig: "{}"})
	if got := envValue(v1, "OPENCODE_CONFIG_CONTENT"); got != "{}" {
		t.Fatalf("OPENCODE_CONFIG_CONTENT = %q, want {}", got)
	}
	if !hasEnv(v1, "OPENCODE_DISABLE_PROJECT_CONFIG") {
		t.Fatal("v1: want OPENCODE_DISABLE_PROJECT_CONFIG")
	}
	if !hasEnv(v1, "OPENCODE_DISABLE_EXTERNAL_SKILLS") {
		t.Fatal("v1: want OPENCODE_DISABLE_EXTERNAL_SKILLS")
	}
	if hasEnv(v1, "OPENCODE_DISABLE_CLAUDE_CODE_SKILLS") {
		t.Fatal("v1: must not set OPENCODE_DISABLE_CLAUDE_CODE_SKILLS")
	}

	v2 := Env(Config{Flavour: detect.V2, InlineConfig: "{}"})
	if !hasEnv(v2, "OPENCODE_DISABLE_CLAUDE_CODE_SKILLS") {
		t.Fatal("v2: want OPENCODE_DISABLE_CLAUDE_CODE_SKILLS")
	}
	if hasEnv(v2, "OPENCODE_DISABLE_EXTERNAL_SKILLS") {
		t.Fatal("v2: must not set OPENCODE_DISABLE_EXTERNAL_SKILLS")
	}

	if hasEnv(Env(Config{Flavour: detect.V1}), "OPENCODE_CONFIG_CONTENT") {
		t.Fatal("empty inline config must not set OPENCODE_CONFIG_CONTENT")
	}
}
