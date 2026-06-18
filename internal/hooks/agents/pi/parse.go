package pi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mainline-org/mainline/internal/hooks"
)

type hookPayload struct {
	SessionID           string   `json:"session_id"`
	Prompt              string   `json:"prompt"`
	Summary             string   `json:"summary"`
	ModifiedFiles       []string `json:"modified_files"`
	Status              string   `json:"status"`
	Reason              string   `json:"reason"`
	PreviousSessionFile string   `json:"previous_session_file"`
	TargetSessionFile   string   `json:"target_session_file"`
}

func (Agent) ParseEvent(_ context.Context, hookName string, stdin io.Reader) (*hooks.Event, error) {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read pi hook stdin: %w", err)
	}
	payload := decodePayload(raw)

	newEvent := func(t hooks.EventType) *hooks.Event {
		ev := hooks.NewEvent(t, AgentName)
		ev.Raw = json.RawMessage(raw)
		hydrate(payload, ev)
		return ev
	}

	switch hookName {
	case HookSessionStart:
		return newEvent(hooks.SessionStart), nil

	case HookUserPromptSubmit:
		ev := newEvent(hooks.TurnStart)
		ev.Prompt = payload.Prompt
		return ev, nil

	case HookStop:
		ev := newEvent(hooks.TurnEnd)
		ev.Summary = payload.Summary
		ev.ModifiedFiles = payload.ModifiedFiles
		return ev, nil

	case HookPreCompact:
		return newEvent(hooks.Compaction), nil

	case HookSessionEnd:
		ev := newEvent(hooks.SessionEnd)
		if ev.Reason == "" && payload.TargetSessionFile != "" {
			ev.Reason = "switch"
		}
		if ev.Reason == "" && payload.PreviousSessionFile != "" {
			ev.Reason = "new-session"
		}
		return ev, nil

	default:
		return &hooks.Event{Agent: AgentName, Raw: json.RawMessage(raw)}, nil
	}
}

func decodePayload(raw []byte) hookPayload {
	if len(raw) == 0 {
		return hookPayload{}
	}
	var payload hookPayload
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func hydrate(payload hookPayload, ev *hooks.Event) {
	ev.SessionID = payload.SessionID
	ev.Status = payload.Status
	ev.Reason = payload.Reason
}
