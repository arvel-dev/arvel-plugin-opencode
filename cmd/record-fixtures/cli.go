// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runCLI runs "opencode <args...>" and writes a sanitized, human-readable
// output.txt. A non-zero exit is evidence, so it is recorded instead of failing
// the capture.
func runCLI(ctx context.Context, r recordEnv, args ...string) (Metadata, error) {
	if r.CLIStandalone {
		args = append([]string{"--standalone"}, args...)
	}
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	if r.CWD != "" {
		cmd.Dir = r.CWD
	}
	cmd.Env = append(os.Environ(), "OPENCODE_LOG_LEVEL=error")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	exit := 0
	if runErr != nil {
		exit = cmd.ProcessState.ExitCode()
	}
	text := fmt.Sprintf("command: opencode %s\nexit: %d\n\n--- stdout ---\n%s\n--- stderr ---\n%s\n",
		strings.Join(args, " "), exit, sanitizeText(stdout.String()), sanitizeText(stderr.String()))
	if err := os.WriteFile(filepath.Join(r.Dir, "output.txt"), []byte(text), 0o644); err != nil {
		return Metadata{}, err
	}
	return Metadata{}, nil
}

func scenarioVersion(ctx context.Context, r recordEnv) (Metadata, error) {
	return runCLI(ctx, r, "--version")
}

func scenarioModels(ctx context.Context, r recordEnv) (Metadata, error) {
	return runCLI(ctx, r, append([]string{"models"}, r.Args...)...)
}

func scenarioAuthList(ctx context.Context, r recordEnv) (Metadata, error) {
	return runCLI(ctx, r, append([]string{"auth", "list"}, r.Args...)...)
}

func scenarioDebugPaths(ctx context.Context, r recordEnv) (Metadata, error) {
	return runCLI(ctx, r, "debug", "paths")
}

func scenarioDebugConfig(ctx context.Context, r recordEnv) (Metadata, error) {
	return runCLI(ctx, r, "debug", "config")
}

func scenarioDebugAgents(ctx context.Context, r recordEnv) (Metadata, error) {
	return runCLI(ctx, r, "debug", "agents")
}

// scenarioDebugSkill probes the documented "debug skill" subcommand; on
// versions that do not implement it the help text and error are the evidence.
func scenarioDebugSkill(ctx context.Context, r recordEnv) (Metadata, error) {
	return runCLI(ctx, r, "debug", "skill")
}

// scenarioDebugHelp records the debug subcommand list. It is evidence for
// whether a documented debug subcommand (for example "debug skill") exists
// without dumping skill bodies, which can contain user content.
func scenarioDebugHelp(ctx context.Context, r recordEnv) (Metadata, error) {
	return runCLI(ctx, r, "debug", "--help")
}
