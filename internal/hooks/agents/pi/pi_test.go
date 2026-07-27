package pi

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/mainline-org/mainline/internal/hooks"
)

func TestInstallWritesManagedExtensionAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()

	report, err := (Agent{}).Install(dir, hooks.InstallOptions{BinPath: "/tmp/mainline"})
	if err != nil {
		t.Fatal(err)
	}
	if report.HookCount != 5 {
		t.Fatalf("HookCount = %d, want 5", report.HookCount)
	}
	if report.CommandMode != hooks.CommandModeBin {
		t.Fatalf("CommandMode = %q, want %q", report.CommandMode, hooks.CommandModeBin)
	}

	extPath := filepath.Join(dir, ".pi", "extensions", "mainline.ts")
	raw, err := os.ReadFile(extPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		managedMarker,
		`const MAINLINE_COMMAND_MODE = "bin";`,
		`const MAINLINE_COMMAND = "/tmp/mainline";`,
		`const MAINLINE_COMMAND_CWD: string | undefined = undefined;`,
		`"hooks", "pi"`,
		`pi.on("before_agent_start"`,
		`debug("running " + hookName + " in " + commandCwd);`,
		`debug("exited " + hookName + " with code " + code`,
		`systemPromptAppend`,
		`const HOOK_STDOUT_MAX_CHARS = 256 * 1024;`,
		`const MAINLINE_CONTEXT_MAX_CHARS = 48 * 1024;`,
		`hook context exceeded Pi safety budget`,
		`combined hook context exceeded Pi safety budget`,
		`stdout from " + hookName + " exceeded safety budget`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("extension missing %q:\n%s", want, text)
		}
	}

	again, err := (Agent{}).Install(dir, hooks.InstallOptions{BinPath: "/tmp/mainline"})
	if err != nil {
		t.Fatal(err)
	}
	if !again.AlreadyInstalled {
		t.Fatalf("second install should be idempotent: %#v", again)
	}

	st, err := (Agent{}).InstallationStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed || st.NeedsRepair || st.HookCount != st.ExpectedHookCount {
		t.Fatalf("unexpected healthy install status: %#v", st)
	}
	if st.CommandMode != hooks.CommandModeBin {
		t.Fatalf("status CommandMode = %q, want %q", st.CommandMode, hooks.CommandModeBin)
	}
}

func TestGeneratedExtensionEnforcesContextBudgets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses an executable script as the fake mainline command")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available")
	}
	if err := exec.Command(node, "--experimental-strip-types", "--input-type=module", "--eval", "").Run(); err != nil {
		t.Skip("node does not support TypeScript type stripping")
	}

	dir := t.TempDir()
	fakeMainline := filepath.Join(dir, "fake-mainline")
	fakeSource := `#!/usr/bin/env node
const hook = process.argv.at(-1);
const scenario = process.env.FAKE_MAINLINE_SCENARIO;
let size = 2;
if (scenario === "combined") size = hook === "session-start" ? 40000 : 20000;
if (scenario === "single") size = hook === "user-prompt-submit" ? 50000 : 2;
if (scenario === "stdout") size = hook === "user-prompt-submit" ? 300000 : 2;
process.stdout.write(JSON.stringify({systemPromptAppend: "X".repeat(size)}));
`
	if err := os.WriteFile(fakeMainline, []byte(fakeSource), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (Agent{}).Install(dir, hooks.InstallOptions{BinPath: fakeMainline}); err != nil {
		t.Fatal(err)
	}

	harnessPath := filepath.Join(dir, "probe.mjs")
	harness := `import mainlinePiExtension from "./.pi/extensions/mainline.ts";
const handlers = new Map();
mainlinePiExtension({on: (name, handler) => handlers.set(name, handler)});
const ctx = {cwd: process.cwd(), sessionManager: {getSessionFile: () => "session.jsonl"}};
await handlers.get("session_start")({reason: "new"}, ctx);
const result = await handlers.get("before_agent_start")({prompt: "test", systemPrompt: "BASE"}, ctx);
const prompt = result?.systemPrompt ?? "";
process.stdout.write(JSON.stringify({
  warning: prompt.includes("Mainline hook context omitted"),
  leaked: prompt.includes("XXXXXXXXXX"),
}));
`
	if err := os.WriteFile(harnessPath, []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, scenario := range []string{"single", "combined", "stdout"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(node, "--experimental-strip-types", harnessPath)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "FAKE_MAINLINE_SCENARIO="+scenario)
			raw, err := cmd.Output()
			if err != nil {
				t.Fatalf("run generated extension: %v", err)
			}
			var got struct {
				Warning bool `json:"warning"`
				Leaked  bool `json:"leaked"`
			}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("decode probe output: %v\n%s", err, raw)
			}
			if !got.Warning || got.Leaked {
				t.Fatalf("unexpected bounded context result: %#v", got)
			}
		})
	}
}

func TestDefaultInstallUsesLocalDevInMainlineSourceRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/mainline-org/mainline\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := (Agent{}).Install(dir, hooks.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.CommandMode != hooks.CommandModeLocalDev {
		t.Fatalf("CommandMode = %q, want %q", report.CommandMode, hooks.CommandModeLocalDev)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".pi", "extensions", "mainline.ts"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `const MAINLINE_COMMAND = "go";`) || !strings.Contains(text, `"run", ".", "hooks", "pi"`) {
		t.Fatalf("expected local-dev invocation in extension:\n%s", text)
	}
	if want := `const MAINLINE_COMMAND_CWD: string | undefined = ` + strconv.Quote(dir) + `;`; !strings.Contains(text, want) {
		t.Fatalf("expected local-dev command cwd %q in extension:\n%s", want, text)
	}

	st, err := (Agent{}).InstallationStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.CommandMode != hooks.CommandModeLocalDev {
		t.Fatalf("status CommandMode = %q, want %q", st.CommandMode, hooks.CommandModeLocalDev)
	}
}

func TestInstallRefusesUnmanagedExtensionUnlessForced(t *testing.T) {
	dir := t.TempDir()
	extPath := filepath.Join(dir, ".pi", "extensions", "mainline.ts")
	if err := os.MkdirAll(filepath.Dir(extPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extPath, []byte("export default function userExtension() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := (Agent{}).Install(dir, hooks.InstallOptions{BinPath: "/tmp/mainline"}); err == nil {
		t.Fatal("expected unmanaged extension collision to fail")
	}

	if _, err := (Agent{}).Install(dir, hooks.InstallOptions{BinPath: "/tmp/mainline", Force: true}); err != nil {
		t.Fatalf("force install should replace unmanaged file: %v", err)
	}
	raw, err := os.ReadFile(extPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), managedMarker) {
		t.Fatalf("forced install did not write managed extension:\n%s", raw)
	}
}

func TestInstallationStatusDetectsMissingPiEventAndRepair(t *testing.T) {
	dir := t.TempDir()
	if _, err := (Agent{}).Install(dir, hooks.InstallOptions{BinPath: "/tmp/mainline"}); err != nil {
		t.Fatal(err)
	}
	extPath := filepath.Join(dir, ".pi", "extensions", "mainline.ts")
	raw, err := os.ReadFile(extPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	text = strings.Replace(text, `pi.on("agent_end", async (event, ctx) => {`, `/* missing agent_end */`, 1)
	if err := os.WriteFile(extPath, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := (Agent{}).InstallationStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed || !st.NeedsRepair || st.HookCount != st.ExpectedHookCount-1 || !strings.Contains(strings.Join(st.RepairReasons, "\n"), "agent_end") {
		t.Fatalf("expected missing agent_end hook to need repair: %#v", st)
	}

	if _, err := (Agent{}).Install(dir, hooks.InstallOptions{BinPath: "/tmp/mainline"}); err != nil {
		t.Fatal(err)
	}
	st, err = (Agent{}).InstallationStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.NeedsRepair || st.HookCount != st.ExpectedHookCount {
		t.Fatalf("install should repair missing Pi event: %#v", st)
	}
}

func TestInstallationStatusDetectsLegacyLocalDevExtensionWithoutCommandCWD(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/mainline-org/mainline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (Agent{}).Install(dir, hooks.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	extPath := filepath.Join(dir, ".pi", "extensions", "mainline.ts")
	raw, err := os.ReadFile(extPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	filtered := lines[:0]
	for _, line := range lines {
		if strings.Contains(line, "MAINLINE_COMMAND_CWD") {
			continue
		}
		filtered = append(filtered, line)
	}
	if err := os.WriteFile(extPath, []byte(strings.Join(filtered, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := (Agent{}).InstallationStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed || !st.NeedsRepair || !strings.Contains(strings.Join(st.RepairReasons, "\n"), "command cwd") {
		t.Fatalf("expected legacy local-dev extension to need repair: %#v", st)
	}
}

func TestUninstallRemovesOnlyManagedExtension(t *testing.T) {
	dir := t.TempDir()
	if _, err := (Agent{}).Install(dir, hooks.InstallOptions{BinPath: "/tmp/mainline"}); err != nil {
		t.Fatal(err)
	}
	extPath := filepath.Join(dir, ".pi", "extensions", "mainline.ts")
	if err := (Agent{}).Uninstall(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(extPath); !os.IsNotExist(err) {
		t.Fatalf("managed extension should be removed, stat err=%v", err)
	}

	if err := os.MkdirAll(filepath.Dir(extPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extPath, []byte("export default function keepMe() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (Agent{}).Uninstall(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(extPath); err != nil {
		t.Fatalf("unmanaged extension should be preserved: %v", err)
	}
}

func TestParseEvents(t *testing.T) {
	raw := strings.NewReader(`{"session_id":"sess-1","prompt":"do work","status":"completed","reason":"quit","summary":"done","modified_files":["a.go"]}`)
	ev, err := (Agent{}).ParseEvent(context.Background(), HookUserPromptSubmit, raw)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != hooks.TurnStart || ev.Agent != AgentName || ev.SessionID != "sess-1" || ev.Prompt != "do work" {
		t.Fatalf("unexpected prompt event: %#v", ev)
	}

	ev, err = (Agent{}).ParseEvent(context.Background(), HookStop, strings.NewReader(`{"session_id":"sess-1","status":"completed","summary":"done","modified_files":["a.go"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != hooks.TurnEnd || ev.Summary != "done" || len(ev.ModifiedFiles) != 1 || ev.ModifiedFiles[0] != "a.go" {
		t.Fatalf("unexpected stop event: %#v", ev)
	}

	ev, err = (Agent{}).ParseEvent(context.Background(), HookPreCompact, strings.NewReader(`{"session_id":"sess-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != hooks.Compaction || ev.SessionID != "sess-1" {
		t.Fatalf("unexpected compaction event: %#v", ev)
	}
}

func TestRenderHookOutput(t *testing.T) {
	d := hooks.NewDispatcher(fakeEngine{
		status: map[string]any{
			"branch": "feat/pi-hooks",
			"active_intent": map[string]any{
				"intent_id": "int_test",
				"goal":      "add pi hooks",
			},
		},
		proposals: map[string]any{
			"proposals": []map[string]any{{"intent_id": "int_other", "title": "nearby"}},
		},
	}, nil, hooks.DefaultDispatchSettings())

	out, err := (Agent{}).RenderHookOutput(HookUserPromptSubmit, d, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if !strings.Contains(payload["systemPromptAppend"], "Mainline per-prompt context") || !strings.Contains(payload["systemPromptAppend"], "int_test") {
		t.Fatalf("unexpected hook output:\n%s", payload["systemPromptAppend"])
	}

	out, err = (Agent{}).RenderHookOutput(HookSessionStart, hooks.NewDispatcher(nil, nil, hooks.DefaultDispatchSettings()), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "systemPromptAppend") || !strings.Contains(string(out), "Mainline session-start context") {
		t.Fatalf("unexpected session-start output: %s", out)
	}
}

type fakeEngine struct {
	status    any
	proposals any
}

func (f fakeEngine) Sync() (any, error)            { return nil, nil }
func (f fakeEngine) Status() (any, error)          { return f.status, nil }
func (f fakeEngine) ListProposals() (any, error)   { return f.proposals, nil }
func (f fakeEngine) BinaryStaleness() (any, error) { return nil, nil }
