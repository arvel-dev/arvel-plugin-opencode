// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/arvel-dev/arvel-sdk/protocol"
)

// rpcErr builds the wire shape OpenCode returns: a JSON-RPC error whose data is
// the vendor discriminator (FINDINGS V6).
func rpcErr(code int, msg string, data string) *protocol.RPCError {
	e := &protocol.RPCError{Code: code, Message: msg}
	if data != "" {
		e.Data = json.RawMessage(data)
	}
	return e
}

func TestMapError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		in         *protocol.RPCError
		wantCode   protocol.ErrorCode
		wantModel  string
		wantVendor string
	}{
		{
			name:     "free limit is rate_limited",
			in:       rpcErr(-32603, "Internal error: Free usage limit reached", `{"errorName":"provider.free-limit","service":"session"}`),
			wantCode: protocol.ErrRateLimited,
		},
		{
			name:     "http 429 message is rate_limited",
			in:       rpcErr(-32603, "Internal error: 429 Too Many Requests", `{"errorName":"provider.transport","service":"session"}`),
			wantCode: protocol.ErrRateLimited,
		},
		{
			name:      "model not found is model_unavailable",
			in:        rpcErr(-32602, "Invalid params: model not found: nonexistent/nonexistent", `{"modelId":"nonexistent/nonexistent","providerId":"nonexistent"}`),
			wantCode:  protocol.ErrModelUnavailable,
			wantModel: "nonexistent/nonexistent",
		},
		{
			name:       "v2 transport is provider_error",
			in:         rpcErr(-32603, "Internal error: ECONNREFUSED", `{"errorName":"provider.transport","service":"session"}`),
			wantCode:   protocol.ErrProviderError,
			wantVendor: "provider.transport",
		},
		{
			name:       "v1 transport (APIError) is provider_error",
			in:         rpcErr(-32603, "Internal error: Cannot connect to API", `{"errorName":"APIError","service":"session"}`),
			wantCode:   protocol.ErrProviderError,
			wantVendor: "APIError",
		},
		{
			name:     "auth heuristic by errorName",
			in:       rpcErr(-32603, "Internal error: sign-in required", `{"errorName":"provider.auth"}`),
			wantCode: protocol.ErrAuthRequired,
		},
		{
			name:     "auth heuristic by message",
			in:       rpcErr(-32603, "Unauthorized: invalid API key", `{}`),
			wantCode: protocol.ErrAuthRequired,
		},
		{
			name:       "unknown vendor code is provider_error",
			in:         rpcErr(-32603, "Internal error: boom", `{"errorName":"something.else"}`),
			wantCode:   protocol.ErrProviderError,
			wantVendor: "something.else",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := MapError(tc.in)
			var re *protocol.RPCError
			if !errors.As(out, &re) {
				t.Fatalf("MapError returned %T, want *protocol.RPCError", out)
			}
			d, ok := re.Arvel()
			if !ok {
				t.Fatalf("mapped error has no data.arvel: %+v", re)
			}
			if d.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", d.Code, tc.wantCode)
			}
			if tc.wantModel != "" && d.Model != tc.wantModel {
				t.Fatalf("model = %q, want %q", d.Model, tc.wantModel)
			}
			if tc.wantVendor != "" && d.VendorCode != tc.wantVendor {
				t.Fatalf("vendorCode = %q, want %q", d.VendorCode, tc.wantVendor)
			}
			if err := d.Validate(); err != nil {
				t.Fatalf("mapped error does not validate: %v", err)
			}
		})
	}
}

func TestMapError_NilAndForeign(t *testing.T) {
	t.Parallel()

	if got := MapError(nil); got != nil {
		t.Fatalf("MapError(nil) = %v, want nil", got)
	}
	foreign := context.Canceled
	if got := MapError(foreign); !errors.Is(got, foreign) {
		t.Fatalf("MapError(foreign) = %v, want the same error", got)
	}
}

func TestMapError_RateLimitedHasNoReset(t *testing.T) {
	t.Parallel()

	d, ok := mustArvel(t, MapError(rpcErr(-32603, "limit", `{"errorName":"provider.free-limit"}`)))
	if !ok {
		t.Fatal("no arvel data")
	}
	if !d.ResetsAt.IsZero() || d.WindowID != "" {
		t.Fatalf("rate_limited carried a reset: %+v", d)
	}
	if !d.Retryable {
		t.Fatal("rate_limited should be retryable")
	}
}

func mustArvel(t *testing.T, err error) (protocol.ErrorData, bool) {
	t.Helper()
	var re *protocol.RPCError
	if !errors.As(err, &re) {
		t.Fatalf("error = %T, want *protocol.RPCError", err)
	}
	return re.Arvel()
}
