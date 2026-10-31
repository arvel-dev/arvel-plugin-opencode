//go:build e2e

// SPDX-License-Identifier: Apache-2.0

// Package e2e runs the built plugin against a real opencode binary and a free
// or local model. It is opt-in: build tag `e2e` plus ARVEL_E2E=1. Prerequisite:
// `opencode` on PATH with at least one configured provider (free or local).
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/arvel-dev/arvel-sdk/testkit/mockhost"
)

func TestRealTurnAgainstOpenCode(t *testing.T) {
	if os.Getenv("ARVEL_E2E") != "1" {
		t.Skip("ARVEL_E2E=1 not set; set it and configure a free or local provider in opencode")
	}
	bin := buildPlugin(t)
	t.Run("basic-turn", func(t *testing.T) {
		h := mockhost.Start(t, bin)
		h.RunScenario(t, filepath.Join("..", "conformance", "basic-turn.yaml"))
		h.Shutdown()
	})
}

// buildPlugin compiles the plugin into a temporary executable and returns its path.
func buildPlugin(t *testing.T) string {
	t.Helper()
	name := "arvel-plugin-opencode"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", out,
		"github.com/arvel-dev/arvel-plugin-opencode/cmd/arvel-plugin-opencode")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}
	return out
}
