// SPDX-License-Identifier: Apache-2.0

// Package detect locates the opencode binary and reads its version, flavour,
// authenticated providers and model catalogue. All parsing is pure (string in,
// struct out) so it can be tested against recorded fixtures; the only side
// effects are the exec seams on Detector.
package detect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Flavour identifies which OpenCode major line a binary belongs to.
type Flavour string

const (
	// V1 is the opencode-ai package (major version 1).
	V1 Flavour = "v1"
	// V2 is the @opencode/cli package (major version >= 2).
	V2 Flavour = "v2"
)

// Version is a parsed opencode version.
type Version struct {
	Major, Minor, Patch int
	Original            string // the raw --version stdout, trimmed
}

// MinVersion is the oldest supportable opencode: the first v1 that returns
// configOptions from session/new (FINDINGS.md V1).
const MinVersion = "1.3.16"

// minVersion is MinVersion as a Version.
var minVersion = Version{Major: 1, Minor: 3, Patch: 16, Original: MinVersion}

// semverRE matches the first semver token in a --version string.
var semverRE = regexp.MustCompile(`\d+\.\d+\.\d+`)

// ErrNoVersion is returned when a --version string contains no semver token.
var ErrNoVersion = errors.New("detect: no semver token in version output")

// ErrNotFound is returned when the opencode binary cannot be located.
var ErrNotFound = errors.New("detect: opencode binary not found")

// commandTimeout bounds one opencode invocation. It is shorter than the host's
// per-request deadline so Status/Models can answer even when the vendor CLI
// hangs (for example v2's managed service failing to start).
const commandTimeout = 5 * time.Second

// ParseVersion extracts the first MAJOR.MINOR.PATCH token from raw version
// output. "opencode v2.2.5" and "1.19.5" both parse.
func ParseVersion(raw string) (Version, error) {
	token := semverRE.FindString(raw)
	if token == "" {
		return Version{}, ErrNoVersion
	}
	parts := strings.Split(token, ".")
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])
	return Version{Major: major, Minor: minor, Patch: patch, Original: strings.TrimSpace(raw)}, nil
}

// Flavour returns V2 when Major >= 2, otherwise V1.
func (v Version) Flavour() Flavour {
	if v.Major >= 2 {
		return V2
	}
	return V1
}

// canonical returns the vX.Y.Z form golang.org/x/mod/semver requires.
func (v Version) canonical() string {
	return "v" + strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
}

// String returns the canonical MAJOR.MINOR.PATCH form (safe for display; never
// the raw vendor stdout).
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// AtLeast reports whether v >= other, comparing only Major.Minor.Patch.
func (v Version) AtLeast(other Version) bool {
	return semver.Compare(v.canonical(), other.canonical()) >= 0
}

// IsOutdated reports whether v is below MinVersion.
func (v Version) IsOutdated() bool {
	return !v.AtLeast(minVersion)
}

// Info is a located binary and its version/flavour.
type Info struct {
	Bin     string
	Version Version
}

// Flavour reports the flavour of Info's version.
func (i Info) Flavour() Flavour { return i.Version.Flavour() }

// runner executes the binary; replaced in tests.
type runner func(ctx context.Context, bin string, args ...string) ([]byte, error)

// Detector locates and interrogates the opencode binary.
type Detector struct {
	bin string
	run runner
}

// New returns a Detector for a configured binary path. path may be empty, in
// which case ResolveBin is used (ARVEL_OPENCODE_BIN under ARVEL_DEV=1, then
// exec.LookPath("opencode")). Auth/Models* use d.bin, so callers must either
// pass a resolved path or call Detect first (Detect resolves and stores d.bin).
func New(path string) *Detector {
	return &Detector{bin: path, run: runCommand}
}

// ResolveBin picks the binary: a non-empty path must be an absolute path to a
// regular file (and is returned verbatim); an empty path falls back to
// ARVEL_OPENCODE_BIN (with ARVEL_DEV=1) and then to exec.LookPath("opencode").
func ResolveBin(path string) (string, error) {
	if path != "" {
		if !filepath.IsAbs(path) {
			return "", fmt.Errorf("%w: %q is not an absolute path", ErrNotFound, path)
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return "", fmt.Errorf("%w: %q is not a regular file", ErrNotFound, path)
		}
		return path, nil
	}
	if v := os.Getenv("ARVEL_OPENCODE_BIN"); v != "" && os.Getenv("ARVEL_DEV") == "1" {
		return v, nil
	}
	b, err := exec.LookPath("opencode")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return b, nil
}

// runCommand is the default exec seam. Each invocation is bounded by
// commandTimeout so a hung vendor CLI cannot block an RPC call for longer than
// its own deadline.
func runCommand(ctx context.Context, bin string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("detect: run %s %v: %w", bin, args, err)
	}
	return out, nil
}

// Detect locates the binary and returns its version and flavour. It stores the
// resolved path back into d.bin so Auth/Models* can reuse it.
func (d *Detector) Detect(ctx context.Context) (Info, error) {
	bin, err := ResolveBin(d.bin)
	if err != nil {
		return Info{}, err
	}
	d.bin = bin
	out, err := d.run(ctx, d.bin, "--version")
	if err != nil {
		return Info{}, err
	}
	v, err := ParseVersion(string(out))
	if err != nil {
		return Info{}, err
	}
	return Info{Bin: d.bin, Version: v}, nil
}
