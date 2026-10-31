// SPDX-License-Identifier: Apache-2.0

package detect

import (
	"context"
	"testing"
)

func TestParseAuthListV2(t *testing.T) {
	t.Parallel()

	got := parseAuthListV2(stdoutOf(t, "v2-auth-list"))
	if len(got) != 1 {
		t.Fatalf("providers = %d, want 1: %+v", len(got), got)
	}
	if got[0].Name != "DeepSeek" || got[0].ID != "DeepSeek" {
		t.Fatalf("provider = %+v, want {Name:DeepSeek ID:DeepSeek}", got[0])
	}
}

func TestParseAuthListV2_MultiWord(t *testing.T) {
	t.Parallel()

	got := parseAuthListV2("GitHub Copilot  github-copilot  stored\n")
	if len(got) != 1 || got[0].Name != "GitHub Copilot" || got[0].ID != "github-copilot" {
		t.Fatalf("provider = %+v, want {Name:GitHub Copilot ID:github-copilot}", got)
	}
}

func TestDetector_Auth_Flavour(t *testing.T) {
	t.Parallel()

	v2 := &Detector{bin: "/bin/true", run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) != 2 || args[0] != "auth" || args[1] != "list" {
			t.Fatalf("unexpected args: %v", args)
		}
		return []byte("Provider  provider  stored\n"), nil
	}}
	got, err := v2.Auth(context.Background(), V2)
	if err != nil || len(got) != 1 || got[0].ID != "provider" {
		t.Fatalf("V2 Auth = %+v, %v", got, err)
	}

	v1 := &Detector{bin: "/bin/true", run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("●  Name api\n└  1 credentials\n"), nil
	}}
	got, err = v1.Auth(context.Background(), V1)
	if err != nil || len(got) != 1 || got[0].Name != "Name" {
		t.Fatalf("V1 Auth = %+v, %v", got, err)
	}
}

func TestParseAuthListV1_Empty(t *testing.T) {
	t.Parallel()

	if got := parseAuthListV1(stdoutOf(t, "v1-auth-list")); len(got) != 0 {
		t.Fatalf("providers = %+v, want none", got)
	}
}

func TestParseAuthListV1_Stored(t *testing.T) {
	t.Parallel()

	got := parseAuthListV1(stdoutOf(t, "v1-auth-list-stored"))
	if len(got) != 1 {
		t.Fatalf("providers = %d, want 1: %+v", len(got), got)
	}
	if got[0].Name != "DeepSeek" || got[0].ID != "DeepSeek" {
		t.Fatalf("provider = %+v, want {Name:DeepSeek ID:DeepSeek}", got[0])
	}
}
