// SPDX-License-Identifier: Apache-2.0

// Package provider implements the plugin.Provider for OpenCode. In O0 it is a
// scaffold that returns a valid, fixed status and a single hidden placeholder
// model; detection, the real model list, upstream control and sessions arrive
// in O1–O4.
package provider

import (
	"context"
	"errors"

	"github.com/arvel-dev/arvel-sdk/plugin"
	"github.com/arvel-dev/arvel-sdk/protocol"
)

// ErrNotImplemented marks provider surfaces that are not built until later milestones.
var ErrNotImplemented = errors.New("not implemented until a later milestone")

// StubModelID is the placeholder model reported until detection lands in O1.
// The protocol requires a non-empty model list with a valid default, so the
// scaffold reports exactly one hidden model instead of an empty list.
const StubModelID = "opencode/stub"

// Provider is the O0 scaffold of the OpenCode provider.
type Provider struct{}

// New returns the O0 provider scaffold.
func New() *Provider { return &Provider{} }

// Initialize reports no optional features: O0 does not resume sessions yet.
func (p *Provider) Initialize(context.Context, protocol.InitMeta) (plugin.Features, error) {
	return plugin.Features{LoadSession: false}, nil
}

// Status reports a ready plugin with a single connected OpenCode connection.
func (p *Provider) Status(context.Context) (protocol.Status, error) {
	return protocol.Status{State: protocol.StateReady, Message: "OpenCode provider scaffold (O0)"}, nil
}

// Models reports one hidden placeholder so the protocol shape validates.
func (p *Provider) Models(context.Context, bool) (protocol.ModelsResult, error) {
	return protocol.ModelsResult{
		Models: []protocol.Model{{
			ID:      StubModelID,
			Label:   "OpenCode (not detected yet)",
			Billing: protocol.BillingUnknown,
			Hidden:  true,
		}},
		Default: StubModelID,
	}, nil
}

// Usage reports unknown usage: OpenCode spend tracking arrives in O4.
func (p *Provider) Usage(context.Context) (protocol.Usage, error) {
	return protocol.Usage{Source: protocol.UsageSourceUnknown, Status: protocol.UsageStatusOK}, nil
}

// Action answers every action with "done": O0 announces no actions.
func (p *Provider) Action(context.Context, string) (protocol.ActionResult, error) {
	return protocol.ActionResult{Kind: protocol.ActionKindDone, Message: "not implemented"}, nil
}

// NewSession always fails: session support arrives in O2.
func (p *Provider) NewSession(context.Context, plugin.SessionParams) (plugin.Session, error) {
	return nil, ErrNotImplemented
}

// LoadSession always fails: LoadSession is not announced in O0.
func (p *Provider) LoadSession(context.Context, plugin.LoadParams) (plugin.Session, error) {
	return nil, ErrNotImplemented
}

// Shutdown releases nothing: the scaffold owns no resources.
func (p *Provider) Shutdown(context.Context) error { return nil }
