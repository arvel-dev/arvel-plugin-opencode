// SPDX-License-Identifier: Apache-2.0

package detect

import (
	"context"
	"regexp"
	"strings"
)

// Provider is an authenticated provider from `opencode auth list`.
type Provider struct {
	ID   string // provider identifier (v2 second column / v1 name)
	Name string // display name (v2 first column / v1 name)
}

// ansiRE matches SGR colour codes emitted by the v1 box-drawing output.
var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// stripANSI removes SGR escape sequences.
func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// Auth returns the authenticated providers for the given flavour. It re-runs
// the command on every call (no caching at this layer).
func (d *Detector) Auth(ctx context.Context, f Flavour) ([]Provider, error) {
	out, err := d.run(ctx, d.bin, "auth", "list")
	if err != nil {
		return nil, err
	}
	switch f {
	case V1:
		return parseAuthListV1(string(out)), nil
	default:
		return parseAuthListV2(string(out)), nil
	}
}

// parseAuthListV2 parses v2's tabular output: one row per provider with
// "<display name...> <providerID> stored". Empty output means no provider is
// authenticated. Parsing from the right keeps multi-word display names.
func parseAuthListV2(s string) []Provider {
	var out []Provider
	for _, ln := range strings.Split(s, "\n") {
		fields := strings.Fields(ln)
		if len(fields) < 3 || fields[len(fields)-1] != "stored" {
			continue
		}
		id := fields[len(fields)-2]
		name := strings.Join(fields[:len(fields)-2], " ")
		if name == "" || id == "" {
			continue
		}
		out = append(out, Provider{Name: name, ID: id})
	}
	return out
}

// parseAuthListV1 parses v1's box-drawing output. Provider rows start with the
// bullet "●" followed by "<name> <type>"; the trailing "<n> credentials" line
// carries the count but is not needed for the provider list.
func parseAuthListV1(s string) []Provider {
	var out []Provider
	for _, ln := range strings.Split(stripANSI(s), "\n") {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, "●") {
			continue
		}
		fields := strings.Fields(t)
		// fields[0] is "●"; the last field is the credential type.
		if len(fields) < 3 {
			continue
		}
		name := strings.Join(fields[1:len(fields)-1], " ")
		if name == "" {
			continue
		}
		out = append(out, Provider{Name: name, ID: name})
	}
	return out
}
