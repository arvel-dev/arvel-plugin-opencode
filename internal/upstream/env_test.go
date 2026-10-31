// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"strings"
	"testing"
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
	env := Env(false, "")
	if !hasEnv(env, "TERM") || envValue(env, "TERM") != "dumb" {
		t.Fatalf("want TERM=dumb, got %q", envValue(env, "TERM"))
	}
	if !hasEnv(env, "OPENCODE_DISABLE_PROJECT_CONFIG") {
		t.Fatal("want OPENCODE_DISABLE_PROJECT_CONFIG for loadProjectConfig=false")
	}
}

func TestEnv_DisableProjectConfig(t *testing.T) {
	if !hasEnv(Env(false, ""), "OPENCODE_DISABLE_PROJECT_CONFIG") {
		t.Fatal("false must set OPENCODE_DISABLE_PROJECT_CONFIG")
	}
	if hasEnv(Env(true, ""), "OPENCODE_DISABLE_PROJECT_CONFIG") {
		t.Fatal("true must not set OPENCODE_DISABLE_PROJECT_CONFIG")
	}
}

func TestEnv_Proxy(t *testing.T) {
	const proxy = "http://127.0.0.1:41234"
	env := Env(false, proxy)
	for _, k := range []string{"HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY"} {
		if got := envValue(env, k); got != proxy {
			t.Fatalf("%s = %q, want %q", k, got, proxy)
		}
	}
	if got := envValue(env, "NO_PROXY"); got != "localhost,127.0.0.1" {
		t.Fatalf("NO_PROXY = %q", got)
	}
	if hasEnv(Env(false, ""), "HTTPS_PROXY") {
		t.Fatal("empty proxy must not set HTTPS_PROXY")
	}
}

func TestEnv_NeverPass(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	t.Setenv("GH_TOKEN", "x")
	t.Setenv("GITHUB_TOKEN", "y")
	t.Setenv("GIT_ASKPASS", "z")
	env := Env(false, "")
	for _, k := range []string{"SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN", "GIT_ASKPASS"} {
		if hasEnv(env, k) {
			t.Fatalf("%s leaked into the child env", k)
		}
	}
}
