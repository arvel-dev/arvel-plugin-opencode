// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/arvel-dev/arvel-sdk/protocol"
)

// vendorError is the decoded `error.data` of an upstream OpenCode error
// (FINDINGS V6): {errorName?, service?, providerId?, modelId?}.
type vendorError struct {
	ErrorName  string `json:"errorName"`
	Service    string `json:"service"`
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
}

// rateLimitRe spots an HTTP 429 / rate-limit provider message. SPEC §8 maps a
// provider message with a status code to rate_limited; FINDINGS V6 observed
// only `provider.free-limit` in O0, so the numeric/message path is the
// documented fallback for the 429s it could not reproduce.
var rateLimitRe = regexp.MustCompile(`(?i)\b429\b|too many requests|rate limit`)

// authStatusRe spots an HTTP 401 status code as a bounded token (so "1429" or
// "4012" do not match). FINDINGS V6 records no auth error fixture; the
// errorName and message checks are a conservative heuristic (re-checked in O5).
var authStatusRe = regexp.MustCompile(`(?i)\b401\b`)

// MapError converts an upstream JSON-RPC error into an Arvel typed error
// (SPEC §8, PROTOCOL §9, FINDINGS V6). A non-*protocol.RPCError (ErrClosed,
// context cancellation) is returned unchanged.
func MapError(err error) error {
	if err == nil {
		return nil
	}
	var re *protocol.RPCError
	if !errors.As(err, &re) {
		return err
	}
	var v vendorError
	if len(re.Data) > 0 {
		_ = json.Unmarshal(re.Data, &v)
	}

	switch {
	case v.ErrorName == "provider.free-limit" || rateLimitRe.MatchString(re.Message):
		return protocol.NewError(protocol.ErrRateLimited,
			errMessage(re.Message, "OpenCode usage limit reached"),
			map[string]any{"retryable": true})
	case re.Code == protocol.CodeInvalidParams && v.ModelID != "":
		return protocol.NewError(protocol.ErrModelUnavailable,
			"model is not available for this account", map[string]any{"model": v.ModelID})
	case isAuthError(v.ErrorName, re.Message):
		return protocol.NewError(protocol.ErrAuthRequired, "OpenCode authentication required", nil)
	case v.ErrorName == "provider.transport" || v.ErrorName == "APIError":
		return protocol.NewError(protocol.ErrProviderError,
			errMessage(re.Message, "OpenCode provider error"),
			map[string]any{"vendorCode": v.ErrorName})
	default:
		code := v.ErrorName
		if code == "" {
			code = "unclassified"
		}
		return protocol.NewError(protocol.ErrProviderError,
			errMessage(re.Message, "OpenCode provider error"),
			map[string]any{"vendorCode": code})
	}
}

// isAuthError is a conservative heuristic for auth failures: an errorName
// containing "auth", a bounded 401 status, or an explicit auth phrase.
func isAuthError(errorName, message string) bool {
	if strings.Contains(strings.ToLower(errorName), "auth") {
		return true
	}
	if authStatusRe.MatchString(message) {
		return true
	}
	m := strings.ToLower(message)
	for _, marker := range []string{"unauthorized", "invalid api key", "not authenticated"} {
		if strings.Contains(m, marker) {
			return true
		}
	}
	return false
}

// errMessage returns vendor text when present, otherwise a fallback.
func errMessage(vendor, fallback string) string {
	if strings.TrimSpace(vendor) == "" {
		return fallback
	}
	return vendor
}
