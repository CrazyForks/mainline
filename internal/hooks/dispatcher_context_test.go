package hooks

import (
	"strings"
	"testing"
)

func TestRenderSessionStartContextBoundsLargeState(t *testing.T) {
	turns := make([]any, 500)
	uncovered := make([]any, 500)
	changes := make([]any, 500)
	for i := range turns {
		turns[i] = map[string]any{
			"description":   strings.Repeat("historical-diff-secret ", 400),
			"files_changed": []string{strings.Repeat("large/path/", 100)},
		}
		uncovered[i] = map[string]any{"subject": strings.Repeat("uncovered-secret ", 400)}
		changes[i] = map[string]any{"path": strings.Repeat("sync-secret/", 400)}
	}
	status := map[string]any{
		"initialized": true,
		"branch":      "feat/pi-hooks",
		"active_intent": map[string]any{
			"intent_id": "int_large",
			"status":    "drafting",
			"thread":    "feat/pi-hooks",
			"goal":      "keep the Pi hook context bounded",
			"turns":     turns,
		},
		"turn_count":     len(turns),
		"proposed_count": 7,
		"coverage": map[string]any{
			"window_size":     500,
			"covered_count":   0,
			"skipped_count":   0,
			"uncovered_count": len(uncovered),
			"uncovered":       uncovered,
		},
		"agent_authority": map[string]any{
			"effective": map[string]any{"autonomy": "handoff", "stop_line": "proposed_intent"},
			"current":   map[string]any{"allowed_boundary": "proposed_intent"},
		},
	}
	syncResult := map[string]any{"changes": changes}

	d := NewDispatcher(nil, nil, DefaultDispatchSettings())
	got := d.RenderSessionStartContext(syncResult, status)
	if len(got) > sessionStartContextMaxBytes {
		t.Fatalf("session context = %d bytes, budget = %d", len(got), sessionStartContextMaxBytes)
	}
	for _, want := range []string{
		"int_large",
		"keep the Pi hook context bounded",
		"proposed_intent",
		`"turns_omitted": 500`,
		`"uncovered_details_omitted": 500`,
		`"omitted": 495`,
		"mainline status --json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("bounded context missing %q:\n%s", want, got)
		}
	}
	for _, secret := range []string{"historical-diff-secret", "uncovered-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("bounded context leaked omitted detail %q", secret)
		}
	}
}

func TestRenderTurnStartContextBoundsLargeStrings(t *testing.T) {
	proposals := make([]map[string]any, 30)
	for i := range proposals {
		proposals[i] = map[string]any{
			"intent_id": "int_other",
			"title":     strings.Repeat("proposal-title ", 1000),
		}
	}
	status := map[string]any{
		"branch": "feat/pi-hooks",
		"active_intent": map[string]any{
			"intent_id": "int_large",
			"goal":      strings.Repeat("goal ", 10000),
		},
	}

	d := NewDispatcher(nil, nil, DefaultDispatchSettings())
	got := d.RenderTurnStartContext(status, map[string]any{"proposals": proposals}, nil, nil)
	if len(got) > turnStartContextMaxBytes {
		t.Fatalf("turn context = %d bytes, budget = %d", len(got), turnStartContextMaxBytes)
	}
	for _, want := range []string{"int_large", "[truncated]", `"omitted": 25`, "mainline context"} {
		if !strings.Contains(got, want) {
			t.Fatalf("bounded turn context missing %q:\n%s", want, got)
		}
	}
}
