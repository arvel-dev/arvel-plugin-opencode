// SPDX-License-Identifier: Apache-2.0

package detect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stdoutOf reads the stdout block of a recorded CLI fixture.
func stdoutOf(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "fixtures", name, "output.txt")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	_, after, ok := strings.Cut(string(b), "--- stdout ---")
	if !ok {
		t.Fatalf("fixture %s has no stdout block", name)
	}
	out, _, _ := strings.Cut(after, "--- stderr ---")
	return out
}

// existingFile creates an absolute regular file and returns its path.
func existingFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return p
}

func TestParseVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw          string
		major, minor int
		patch        int
		flavour      Flavour
		outdated     bool
		wantErr      error
	}{
		{"opencode v2.2.5", 2, 2, 5, V2, false, nil},
		{"1.19.5", 1, 19, 5, V1, false, nil},
		{"1.3.16", 1, 3, 16, V1, false, nil},
		{"v1.4.0\n", 1, 4, 0, V1, false, nil},
		{"1.3.15", 1, 3, 15, V1, true, nil},
		{"no version here", 0, 0, 0, V1, true, ErrNoVersion},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			v, err := ParseVersion(c.raw)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("ParseVersion(%q) error = %v, want %v", c.raw, err, c.wantErr)
			}
			if c.wantErr != nil {
				return
			}
			if v.Major != c.major || v.Minor != c.minor || v.Patch != c.patch {
				t.Fatalf("ParseVersion(%q) = %d.%d.%d, want %d.%d.%d", c.raw, v.Major, v.Minor, v.Patch, c.major, c.minor, c.patch)
			}
			if got := v.Flavour(); got != c.flavour {
				t.Fatalf("Flavour() = %q, want %q", got, c.flavour)
			}
			if got := v.IsOutdated(); got != c.outdated {
				t.Fatalf("IsOutdated() = %v, want %v", got, c.outdated)
			}
		})
	}
}

func TestVersionAtLeast(t *testing.T) {
	t.Parallel()

	cases := []struct {
		a, b Version
		want bool
	}{
		{Version{Major: 1, Minor: 3, Patch: 16}, Version{Major: 1, Minor: 3, Patch: 15}, true},
		{Version{Major: 2, Minor: 0, Patch: 0}, Version{Major: 1, Minor: 3, Patch: 16}, true},
		{Version{Major: 1, Minor: 2, Patch: 0}, Version{Major: 1, Minor: 3, Patch: 16}, false},
		{Version{Major: 1, Minor: 3, Patch: 16}, Version{Major: 1, Minor: 3, Patch: 16}, true},
	}
	for _, c := range cases {
		if got := c.a.AtLeast(c.b); got != c.want {
			t.Fatalf("%v.AtLeast(%v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestResolveBin(t *testing.T) {
	t.Run("explicit path", func(t *testing.T) {
		want := existingFile(t)
		got, err := ResolveBin(want)
		if err != nil || got != want {
			t.Fatalf("ResolveBin(%q) = %q, %v", want, got, err)
		}
	})
	t.Run("relative path rejected", func(t *testing.T) {
		if _, err := ResolveBin("bin/opencode"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("ResolveBin(relative) error = %v, want ErrNotFound", err)
		}
	})
	t.Run("missing path rejected", func(t *testing.T) {
		if _, err := ResolveBin(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrNotFound) {
			t.Fatalf("ResolveBin(missing) error = %v, want ErrNotFound", err)
		}
	})
	t.Run("dev env override", func(t *testing.T) {
		t.Setenv("ARVEL_DEV", "1")
		t.Setenv("ARVEL_OPENCODE_BIN", "/from/env/opencode")
		got, err := ResolveBin("")
		if err != nil || got != "/from/env/opencode" {
			t.Fatalf("ResolveBin(\"\") = %q, %v", got, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		t.Setenv("ARVEL_DEV", "")
		t.Setenv("ARVEL_OPENCODE_BIN", "")
		t.Setenv("PATH", t.TempDir())
		if _, err := ResolveBin(""); !errors.Is(err, ErrNotFound) {
			t.Fatalf("ResolveBin(\"\") error = %v, want ErrNotFound", err)
		}
	})
}

func TestDetector_Detect(t *testing.T) {
	t.Parallel()

	bin := existingFile(t)
	d := &Detector{bin: bin, run: func(_ context.Context, gotBin string, args ...string) ([]byte, error) {
		if gotBin != bin || len(args) != 1 || args[0] != "--version" {
			t.Fatalf("unexpected call: bin=%q args=%v", gotBin, args)
		}
		return []byte("opencode v2.2.5\n"), nil
	}}
	info, err := d.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Bin != bin || info.Version.Flavour() != V2 || info.Version.Major != 2 {
		t.Fatalf("Detect = %+v, want bin=%q flavour=v2 major=2", info, bin)
	}
	if d.bin != bin {
		t.Fatalf("Detect did not store the resolved bin: %q", d.bin)
	}
}

func TestDetector_DetectNotFound(t *testing.T) {
	t.Setenv("ARVEL_DEV", "")
	t.Setenv("ARVEL_OPENCODE_BIN", "")
	t.Setenv("PATH", t.TempDir())

	d := &Detector{bin: "", run: func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("run must not be called when the binary is missing")
		return nil, nil
	}}
	if _, err := d.Detect(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Detect error = %v, want ErrNotFound", err)
	}
}
