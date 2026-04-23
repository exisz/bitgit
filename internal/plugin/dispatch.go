package plugin

import (
	"context"
	"errors"
	"fmt"
)

// HookResult is the canonical response shape from a plugin hook.
//
//	allow: false     → veto. bitgit aborts with Reason.
//	mutate: non-nil  → bitgit replaces the matching field of the operation
//	                   with the mutated value (verb-specific schema).
type HookResult struct {
	Allow  bool           `json:"allow"`
	Reason string         `json:"reason,omitempty"`
	Mutate map[string]any `json:"mutate,omitempty"`
}

// VetoError is returned by Dispatch when any plugin returned Allow=false.
type VetoError struct {
	Plugin string
	Hook   string
	Reason string
}

func (e *VetoError) Error() string {
	return fmt.Sprintf("plugin %q vetoed %s: %s", e.Plugin, e.Hook, e.Reason)
}

// Dispatch fires a hook across every matching plugin sequentially.
//
// Sequential by design: plugin A's mutation must be visible to plugin B.
// First veto wins — remaining plugins are not consulted. Each plugin's
// mutation is shallow-merged into payload before the next call.
//
// payload is mutated in place. Callers should pass a copy if they need the
// original.
func Dispatch(
	ctx context.Context,
	manifests []Manifest,
	hookCtx Context,
	payload map[string]any,
) error {
	for _, m := range manifests {
		if !contains(m.Hooks, hookCtx.Hook) {
			continue
		}
		if !m.Matches(hookCtx) {
			continue
		}
		client, err := Spawn(ctx, m)
		if err != nil {
			return fmt.Errorf("spawn %s: %w", m.Name, err)
		}
		var res HookResult
		err = client.Call(ctx, hookCtx.Hook, map[string]any{
			"context": hookCtx,
			"payload": payload,
		}, &res)
		_ = client.Close()
		if err != nil {
			return fmt.Errorf("plugin %s hook %s: %w", m.Name, hookCtx.Hook, err)
		}
		if !res.Allow {
			return &VetoError{Plugin: m.Name, Hook: hookCtx.Hook, Reason: res.Reason}
		}
		for k, v := range res.Mutate {
			payload[k] = v
		}
	}
	return nil
}

// IsVeto reports whether err is a plugin veto.
func IsVeto(err error) bool {
	var v *VetoError
	return errors.As(err, &v)
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
