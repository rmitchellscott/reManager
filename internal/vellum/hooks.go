package vellum

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"reManager/internal/component"
	"reManager/internal/executor"
)

type HookFunc func(ctx component.CommandContext) (*component.HookExecutionResult, error)

var hookRegistry = map[string]HookFunc{
	"rebuild_hashtable_dialog": RebuildHashtableDialog,
	"launcher_select":          LauncherSelectDialog,
}

const (
	LauncherctlBin      = VellumRoot + "/bin/launcherctl"
	StockLauncher       = "none"
	LauncherProbeMarker = "@@launcherctl@@"
)

func LauncherSelected(name string) bool {
	return name != "" && name != StockLauncher
}

func LauncherSelectDialog(ctx component.CommandContext) (*component.HookExecutionResult, error) {
	if ctx.Exec == nil {
		return nil, fmt.Errorf("launcher selection needs a connected device")
	}

	probe := fmt.Sprintf("%[1]s list-launchers 2>/dev/null; echo %[2]s; %[1]s current-launcher 2>/dev/null; echo %[2]s; %[1]s active-launcher 2>/dev/null",
		LauncherctlBin, LauncherProbeMarker)
	output, err := ctx.Exec.ExecuteWithOutput(probe)
	if err != nil && strings.TrimSpace(output) == "" {
		return nil, fmt.Errorf("failed to list launchers: %w", err)
	}

	sections := strings.Split(output, LauncherProbeMarker)
	for len(sections) < 3 {
		sections = append(sections, "")
	}

	seen := map[string]bool{}
	var launchers []string
	for _, name := range parseLauncherNames(sections[0]) {
		if seen[name] {
			continue
		}
		seen[name] = true
		launchers = append(launchers, name)
	}

	installed := 0
	for _, name := range launchers {
		if name != StockLauncher {
			installed++
		}
	}
	if installed == 0 {
		return &component.HookExecutionResult{
			DialogConfig: &component.DialogConfig{
				Title:       "No launchers installed",
				Severity:    component.DialogInfo,
				Subtitle:    "Launchers let apps run on your reMarkable. Install one from the Mods tab to choose between them here.",
				InfoOnly:    true,
				ConfirmText: "OK",
			},
		}, nil
	}

	current := ParseLauncherName(sections[1])
	active := ParseLauncherName(sections[2])
	if !seen[StockLauncher] {
		launchers = append(launchers, StockLauncher)
	}

	sort.Slice(launchers, func(i, j int) bool {
		if (launchers[i] == StockLauncher) != (launchers[j] == StockLauncher) {
			return launchers[j] == StockLauncher
		}
		return launchers[i] < launchers[j]
	})

	actions := make([]component.DialogAction, 0, len(launchers))
	for _, launcher := range launchers {
		if launcher == current {
			continue
		}
		actions = append(actions, component.DialogAction{
			Id:    launcher,
			Label: launcherLabel(launcher),
			Type:  "run_command",
			Value: fmt.Sprintf("%s switch-launcher %s --start", LauncherctlBin, launcher),
		})
	}

	var state string
	switch {
	case current == "" || current == StockLauncher:
		state = "No launcher is selected."
	case active == current:
		state = fmt.Sprintf("%s is running.", current)
	case active == "":
		state = fmt.Sprintf("%s is selected but not running.", current)
	default:
		state = fmt.Sprintf("%s is selected and %s is running.", current, active)
	}
	return &component.HookExecutionResult{
		DialogConfig: &component.DialogConfig{
			Title:    "Switch launcher",
			Severity: component.DialogInfo,
			Subtitle: "Restarts the reMarkable UI.",
			Message:  state,
			Actions:  actions,
			Choices:  true,
		},
	}, nil
}

var launcherNameRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

func parseLauncherNames(output string) []string {
	var names []string
	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(line)
		if launcherNameRegex.MatchString(name) {
			names = append(names, name)
		}
	}
	return names
}

func ParseLauncherName(output string) string {
	if names := parseLauncherNames(output); len(names) > 0 {
		return names[0]
	}
	return ""
}

func launcherLabel(name string) string {
	if name == StockLauncher {
		return "No launcher"
	}
	return name
}

func GetHook(name string) HookFunc {
	return hookRegistry[name]
}

func launcherState(ctx component.CommandContext) (current, active string) {
	if ctx.Exec == nil {
		return "", ""
	}
	probe := fmt.Sprintf("[ -x %[1]s ] && %[1]s current-launcher 2>/dev/null; echo %[2]s; [ -x %[1]s ] && %[1]s active-launcher 2>/dev/null",
		LauncherctlBin, LauncherProbeMarker)
	output, err := ctx.Exec.ExecuteWithOutput(probe)
	if err != nil && strings.TrimSpace(output) == "" {
		return "", ""
	}
	sections := strings.Split(output, LauncherProbeMarker)
	for len(sections) < 2 {
		sections = append(sections, "")
	}
	return ParseLauncherName(sections[0]), ParseLauncherName(sections[1])
}

const RebuildHashtableSeconds = 65

func RebuildHashtableDialog(ctx component.CommandContext) (*component.HookExecutionResult, error) {
	current, active := launcherState(ctx)

	script := "/home/root/xovi/rebuild_hashtable"
	postCommand := &component.DialogConfig{
		Title:         "Hashtable rebuilt",
		Severity:      component.DialogSuccess,
		Success:       true,
		PrimaryAction: "start_with_mods",
		CancelText:    "Not now",
		Subtitle:      "The hashtable has been rebuilt.",
		Message:       "The reMarkable UI is stopped. How would you like to start it?",
		Actions: []component.DialogAction{
			{
				Id:    "start_with_mods",
				Label: "Start with mods",
				Type:  "run_command",
				Value: executor.WithXochitlStartReset("/home/root/xovi/start"),
			},
			{
				Id:    "start_without_mods",
				Label: "Start without mods",
				Type:  "run_command",
				Value: executor.WithXochitlStartReset("systemctl start xochitl"),
			},
		},
	}

	switch {
	case LauncherSelected(active):
		script = fmt.Sprintf("%[1]s stop-launcher; %[2]s; rc=$?; %[3]s; exit $rc",
			LauncherctlBin, script, executor.WithXochitlStartReset(LauncherctlBin+" start-launcher"))
		postCommand = &component.DialogConfig{
			Title:       "Hashtable rebuilt",
			Severity:    component.DialogSuccess,
			Success:     true,
			InfoOnly:    true,
			ConfirmText: "OK",
			Subtitle:    fmt.Sprintf("The hashtable has been rebuilt and %s restarted.", active),
		}
	case LauncherSelected(current):
		postCommand = &component.DialogConfig{
			Title:         "Hashtable rebuilt",
			Severity:      component.DialogSuccess,
			Success:       true,
			PrimaryAction: "start_launcher",
			CancelText:    "Not now",
			Subtitle:      "The hashtable has been rebuilt.",
			Message:       fmt.Sprintf("The reMarkable UI is stopped. Start %s to bring it back.", current),
			Actions: []component.DialogAction{
				{
					Id:    "start_launcher",
					Label: fmt.Sprintf("Start %s", current),
					Type:  "run_command",
					Value: executor.WithXochitlStartReset(LauncherctlBin + " start-launcher"),
				},
			},
		}
	}

	return &component.HookExecutionResult{
		DialogConfig: &component.DialogConfig{
			Title:             "Rebuild hashtable",
			Severity:          component.DialogWarning,
			ExpectedSeconds:   RebuildHashtableSeconds,
			Subtitle:          "Takes about a minute and restarts the reMarkable UI.",
			Message:           "The hashtable is a map mods use to identify interface components. It's separate from system files and required for UI mods.",
			Note:              "You'll need to rebuild after each OS update.\nYou can rebuild the hashtable from the System tab.",
			ConfirmText:       "Rebuild",
			CancelText:        "Cancel",
			InProgressMessage: "If you have a passcode, enter it on your reMarkable.",
			PostCommandDialog: postCommand,
		},
		Command: &component.CommandResult{
			Script:      script,
			Description: "Rebuild the Qt resource hashtable (needed after OS updates)",
		},
	}, nil
}
