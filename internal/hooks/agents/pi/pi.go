// Package pi wires the Pi coding agent into mainline's hooks subsystem.
package pi

import (
	"encoding/json"

	"github.com/mainline-org/mainline/internal/hooks"
)

const AgentName = "pi"

const DisplayName = "Pi"

const (
	HookSessionStart     = "session-start"
	HookUserPromptSubmit = "user-prompt-submit"
	HookStop             = "stop"
	HookPreCompact       = "pre-compact"
	HookSessionEnd       = "session-end"
)

// Agent is the Pi implementation of hooks.Agent. Pi does not have a native
// hooks.json format; Install writes a repo-local .pi/extensions/mainline.ts
// extension that subscribes to Pi lifecycle events and invokes
// `mainline hooks pi <event>`.
type Agent struct{}

func (Agent) Name() string { return AgentName }

func (Agent) DisplayName() string { return DisplayName }

func (Agent) HookNames() []string {
	return []string{
		HookSessionStart,
		HookUserPromptSubmit,
		HookStop,
		HookPreCompact,
		HookSessionEnd,
	}
}

func (Agent) RenderHookOutput(hookName string, d *hooks.Dispatcher, _ *hooks.Event, _ any) ([]byte, error) {
	if d == nil || !d.Settings.Enabled {
		return nil, nil
	}

	var md string
	switch hookName {
	case HookSessionStart:
		md = d.RenderSessionStartContext(d.LastSync(), d.LastStatus())
	case HookUserPromptSubmit:
		if d.Engine == nil {
			return nil, nil
		}
		status, statusErr := d.Engine.Status()
		proposals, proposalsErr := d.Engine.ListProposals()
		md = d.RenderTurnStartContext(status, proposals, statusErr, proposalsErr)
	default:
		return nil, nil
	}

	if md == "" {
		return nil, nil
	}
	return json.Marshal(map[string]any{
		"systemPromptAppend": md,
	})
}

func init() {
	hooks.Register(Agent{})
}
