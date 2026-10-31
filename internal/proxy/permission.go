// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"

	acp "github.com/coder/acp-go-sdk"

	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

// categoryFromKind maps an ACP tool kind to a permission category (PROTOCOL §7.3).
func categoryFromKind(k *acp.ToolKind) protocol.PermissionCategory {
	if k == nil {
		return protocol.PermissionCategoryOther
	}
	switch *k {
	case acp.ToolKindExecute:
		return protocol.PermissionCategoryCommand
	case acp.ToolKindEdit, acp.ToolKindDelete, acp.ToolKindMove:
		return protocol.PermissionCategoryEdit
	case acp.ToolKindRead, acp.ToolKindSearch:
		return protocol.PermissionCategoryRead
	case acp.ToolKindFetch:
		return protocol.PermissionCategoryNetwork
	default:
		return protocol.PermissionCategoryOther
	}
}

// RequestPermission evaluates the upstream permission request against Arvel
// policy (SPEC §6.2): deny/allow are answered locally; ask is forwarded to the
// host with _meta.arvel; the host's "always" is kept session-local and answered
// "once" upstream.
func (o *outputSink) RequestPermission(ctx context.Context, req acp.RequestPermissionRequest) acp.RequestPermissionResponse {
	category := categoryFromKind(req.ToolCall.Kind)
	key := allowKey(category, req.ToolCall)

	if o.s.allow.hit(key) {
		if id, ok := optionByKind(req.Options, acp.PermissionOptionKindAllowOnce); ok {
			return selected(id)
		}
		return cancelled()
	}

	switch o.s.decide(category, req.ToolCall) {
	case protocol.PermissionLevelDeny:
		if id, ok := optionByKind(req.Options, acp.PermissionOptionKindRejectOnce); ok {
			return selected(id)
		}
		return cancelled()
	case protocol.PermissionLevelAllow:
		if id, ok := optionByKind(req.Options, acp.PermissionOptionKindAllowOnce); ok {
			return selected(id)
		}
		return cancelled()
	}

	dec, err := o.out.RequestPermission(ctx, plugin.PermissionRequest{
		ToolCall: req.ToolCall,
		Options:  req.Options,
		Meta:     permissionMeta(category, req.ToolCall),
	})
	if err != nil || dec.Cancelled {
		return cancelled()
	}
	if isKind(req.Options, dec.OptionID, acp.PermissionOptionKindAllowAlways) {
		o.s.allow.add(key)
		if id, ok := optionByKind(req.Options, acp.PermissionOptionKindAllowOnce); ok {
			return selected(id)
		}
		return cancelled()
	}
	return selected(acp.PermissionOptionId(dec.OptionID))
}

func selected(id acp.PermissionOptionId) acp.RequestPermissionResponse {
	return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected(id)}
}

func cancelled() acp.RequestPermissionResponse {
	return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}
}
