// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/arvel-dev/arvel-sdk/plugin"
)

func TestProvider_ModelsPlaceholder(t *testing.T) {
	t.Parallel()

	got, err := New().Models(context.Background(), false)
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("Models result does not validate: %v", err)
	}
	if len(got.Models) != 1 || got.Models[0].ID != StubModelID {
		t.Fatalf("Models = %+v, want the single %q placeholder", got.Models, StubModelID)
	}
	if got.Default != StubModelID {
		t.Fatalf("Default = %q, want %q", got.Default, StubModelID)
	}
}

func TestProvider_NewSessionNotImplemented(t *testing.T) {
	t.Parallel()

	_, err := New().NewSession(context.Background(), plugin.SessionParams{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("NewSession error = %v, want ErrNotImplemented", err)
	}
}

func TestProvider_StatusValid(t *testing.T) {
	t.Parallel()

	st, err := New().Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if err := st.Validate(); err != nil {
		t.Fatalf("Status does not validate: %v", err)
	}
}
