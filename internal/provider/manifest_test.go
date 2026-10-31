// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"path/filepath"
	"testing"

	"github.com/arvel-dev/arvel-sdk/manifest"
)

// TestManifest validates the checked-in arvel.json against the SDK schema and
// semantic rules. It is the offline guard for the manifest.
func TestManifest(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "arvel.json")
	m, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	if problems := m.Validate(); len(problems) != 0 {
		for _, p := range problems {
			t.Errorf("manifest problem: %s", p)
		}
		t.Fatal("arvel.json is not valid")
	}
	if m.ID != "opencode" {
		t.Fatalf("manifest id = %q, want %q", m.ID, "opencode")
	}
	if m.Type != manifest.TypeProvider {
		t.Fatalf("manifest type = %q, want %q", m.Type, manifest.TypeProvider)
	}
}
