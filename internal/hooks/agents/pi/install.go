package pi

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/mainline-org/mainline/internal/hooks"
)

const (
	piDirName          = ".pi"
	extensionsDirName  = "extensions"
	extensionFileName  = "mainline.ts"
	managedMarker      = "mainline-managed-pi-extension"
	commandModeVarName = "MAINLINE_COMMAND_MODE"
)

//go:embed templates/mainline.ts.tmpl
var extensionTemplateSource string

var extensionTemplate = template.Must(template.New("mainline.ts").Parse(extensionTemplateSource))

func (Agent) Install(repoRoot string, opts hooks.InstallOptions) (hooks.InstallReport, error) {
	opts = hooks.ResolveInstallOptions(repoRoot, opts)
	report := hooks.InstallReport{
		Scope:           "repo-local",
		RestartRequired: true,
		CommandMode:     hooks.InstallCommandMode(opts),
	}
	extensionPath := extensionPath(repoRoot)
	desired, err := extensionSource(opts)
	if err != nil {
		return report, err
	}

	prev, err := os.ReadFile(extensionPath)
	fileExisted := true
	if err != nil {
		if os.IsNotExist(err) {
			fileExisted = false
		} else {
			return report, fmt.Errorf("read %s: %w", extensionPath, err)
		}
	}

	if fileExisted {
		text := string(prev)
		if !isManagedExtension(text) && !opts.Force {
			return report, fmt.Errorf("refusing to overwrite unmanaged Pi extension at %s (pass --force to replace it)", extensionPath)
		}
		if text == desired {
			report.AlreadyInstalled = true
			report.Files = []string{extensionPath}
			report.HookCount = len((Agent{}).HookNames())
			return report, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(extensionPath), 0o755); err != nil {
		return report, fmt.Errorf("create .pi/extensions dir: %w", err)
	}
	if err := os.WriteFile(extensionPath, []byte(desired), 0o644); err != nil {
		return report, fmt.Errorf("write %s: %w", extensionPath, err)
	}
	report.Files = []string{extensionPath}
	if !fileExisted {
		report.CreatedFiles = []string{extensionPath}
	}
	report.HookCount = len((Agent{}).HookNames())
	return report, nil
}

func (Agent) Uninstall(repoRoot string) error {
	extensionPath := extensionPath(repoRoot)
	data, err := os.ReadFile(extensionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", extensionPath, err)
	}
	if !isManagedExtension(string(data)) {
		return nil
	}
	if err := os.Remove(extensionPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", extensionPath, err)
	}
	// Best effort cleanup: only succeeds when the directories are empty.
	_ = os.Remove(filepath.Dir(extensionPath))
	_ = os.Remove(filepath.Join(repoRoot, piDirName))
	return nil
}

func (Agent) IsInstalled(repoRoot string) (bool, error) {
	st, err := (Agent{}).InstallationStatus(repoRoot)
	return st.Installed, err
}

func (Agent) InstallationStatus(repoRoot string) (hooks.InstallationStatus, error) {
	extensionPath := extensionPath(repoRoot)
	st := hooks.InstallationStatus{
		Scope:             "repo-local",
		Files:             []string{extensionPath},
		ExpectedHookCount: len((Agent{}).HookNames()),
	}
	data, err := os.ReadFile(extensionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, fmt.Errorf("read %s: %w", extensionPath, err)
	}
	text := string(data)
	if !isManagedExtension(text) {
		st.RepairReasons = []string{".pi/extensions/mainline.ts exists but is not managed by Mainline"}
		st.NeedsRepair = true
		return st, nil
	}
	st.Installed = true
	st.RestartRequired = true
	st.HookCount = st.ExpectedHookCount
	st.CommandMode = extensionCommandMode(text)
	if st.CommandMode == "" || st.CommandMode == hooks.CommandModeUnknown {
		st.RepairReasons = append(st.RepairReasons, "could not determine Pi extension command mode")
	}
	if reason := hooks.RuntimeRepairReason(st.CommandMode); reason != "" {
		st.RepairReasons = append(st.RepairReasons, reason)
	}
	st.NeedsRepair = len(st.RepairReasons) > 0
	return st, nil
}

func extensionPath(repoRoot string) string {
	return filepath.Join(repoRoot, piDirName, extensionsDirName, extensionFileName)
}

func isManagedExtension(text string) bool {
	return strings.Contains(text, managedMarker)
}

func extensionCommandMode(text string) string {
	needle := "const " + commandModeVarName + " = "
	idx := strings.Index(text, needle)
	if idx < 0 {
		return hooks.CommandModeUnknown
	}
	rest := text[idx+len(needle):]
	end := strings.Index(rest, ";")
	if end < 0 {
		return hooks.CommandModeUnknown
	}
	value, err := strconv.Unquote(strings.TrimSpace(rest[:end]))
	if err != nil {
		return hooks.CommandModeUnknown
	}
	switch value {
	case hooks.CommandModePath, hooks.CommandModeLocalDev, hooks.CommandModeBin:
		return value
	default:
		return hooks.CommandModeUnknown
	}
}

func extensionSource(opts hooks.InstallOptions) (string, error) {
	command, argsPrefix := piInvocation(opts)
	data := struct {
		ManagedMarker string
		CommandMode   string
		Command       string
		ArgsPrefix    string
	}{
		ManagedMarker: managedMarker,
		CommandMode:   quoteTS(hooks.InstallCommandMode(opts)),
		Command:       quoteTS(command),
		ArgsPrefix:    quoteTSArray(argsPrefix),
	}
	var out bytes.Buffer
	if err := extensionTemplate.Execute(&out, data); err != nil {
		return "", fmt.Errorf("render Pi extension template: %w", err)
	}
	return out.String(), nil
}

func piInvocation(opts hooks.InstallOptions) (string, []string) {
	switch {
	case opts.BinPath != "":
		return opts.BinPath, []string{"hooks", AgentName}
	case opts.LocalDev:
		return "go", []string{"run", ".", "hooks", AgentName}
	default:
		return "mainline", []string{"hooks", AgentName}
	}
}

func quoteTS(s string) string {
	return strconv.Quote(s)
}

func quoteTSArray(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, strconv.Quote(v))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
