// SPDX-License-Identifier: Apache-2.0
// Command check-conformance builds the plugin and the fake opencode vendor,
// generates a ready --test-env, and runs arvel-plugin-check --level release
// once for OpenCode v1 and once for v2. Exit 0 iff both flavours pass.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type flavour struct {
	name    string // v1 | v2
	version string // FAKE_OPENCODE_VERSION
}

var flavours = []flavour{
	{name: "v1", version: "1.19.5"},
	{name: "v2", version: "opencode v2.2.5"},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "check-conformance: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "check-conformance-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	plugin := filepath.Join(dir, "arvel-plugin-opencode")
	fake := filepath.Join(dir, "fakeopencode")
	if err := goBuild(root, "./cmd/arvel-plugin-opencode", plugin); err != nil {
		return err
	}
	if err := goBuild(root, "./testdata/fakeagent", fake); err != nil {
		return err
	}
	checker, err := exec.LookPath("arvel-plugin-check")
	if err != nil {
		return fmt.Errorf("arvel-plugin-check not found on PATH: %w", err)
	}

	for _, f := range flavours {
		envPath := filepath.Join(dir, "conformance-"+f.name+".yaml")
		if err := writeTestEnv(envPath, fake, f); err != nil {
			return err
		}
		if err := runCheck(checker, root, envPath, plugin, f.name); err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
	}
	return nil
}

// goBuild runs `go build -o out pkg` with dir as the working directory.
func goBuild(dir, pkg, out string) error {
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = dir
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build %s: %w\n%s", pkg, err, b)
	}
	return nil
}

// runCheck runs arvel-plugin-check --level release and returns an error only
// when the checker reports a failure (non-zero exit).
func runCheck(checker, root, envPath, plugin, name string) error {
	cmd := exec.Command(checker,
		"--level", "release",
		"--manifest", filepath.Join(root, "arvel.json"),
		"--scenarios", filepath.Join(root, "conformance"),
		"--test-env", envPath,
		"--", plugin,
	)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("check-conformance: running %s\n", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("arvel-plugin-check failed: %w", err)
	}
	return nil
}

// writeTestEnv renders the ready --test-env for one flavour.
func writeTestEnv(path, fake string, f flavour) error {
	content := fmt.Sprintf(`initialize:
  connection:
    kind: opencode
    config:
      opencodePath: %q
plugin:
  env:
    FAKE_OPENCODE_VERSION: %q
    FAKE_OPENCODE_FLAVOUR: %q
checks:
  cancel:
    prompt:
      - { type: text, text: "HOLD" }
  busy:
    prompt:
      - { type: text, text: "HOLD" }
`, fake, f.version, f.name)
	return os.WriteFile(path, []byte(content), 0o644)
}
