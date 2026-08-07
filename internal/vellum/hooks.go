package vellum

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"reManager/internal/component"
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
				Title:       "No Launchers Installed",
				Message:     "Launchers let apps run on your reMarkable. Install one from the Mods tab to choose between them here.",
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

	message := "Choose which launcher starts on your reMarkable."
	switch {
	case current == "" || current == StockLauncher:
		message = "No launcher is selected. " + message
	case active == current:
		message = fmt.Sprintf("%s is running. %s", current, message)
	case active == "":
		message = fmt.Sprintf("%s is selected but not running. %s", current, message)
	default:
		message = fmt.Sprintf("%s is selected and %s is running. %s", current, active, message)
	}

	return &component.HookExecutionResult{
		DialogConfig: &component.DialogConfig{
			Title:   "Switch Launcher",
			Message: message,
			Note:    "Switching restarts your reMarkable's interface.",
			Actions: actions,
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

func RebuildHashtableDialog(ctx component.CommandContext) (*component.HookExecutionResult, error) {
	current, active := launcherState(ctx)

	script := "/home/root/xovi/rebuild_hashtable"
	postCommand := &component.DialogConfig{
		Title:         "Setup Complete",
		Success:       true,
		PrimaryAction: "start_with_mods",
		Message:       "The hashtable has been rebuilt. The reMarkable interface is currently stopped. How would you like to start it?",
		Actions: []component.DialogAction{
			{
				Id:    "start_with_mods",
				Label: "Start UI with Mods",
				Type:  "run_command",
				Value: "/home/root/xovi/start",
			},
			{
				Id:    "start_without_mods",
				Label: "Start UI without Mods",
				Type:  "run_command",
				Value: "systemctl start xochitl",
			},
		},
	}

	switch {
	case LauncherSelected(active):
		script = fmt.Sprintf("%[1]s stop-launcher; %[2]s; rc=$?; %[1]s start-launcher; exit $rc",
			LauncherctlBin, script)
		postCommand = &component.DialogConfig{
			Title:       "Setup Complete",
			Success:     true,
			InfoOnly:    true,
			ConfirmText: "OK",
			Message:     fmt.Sprintf("The hashtable has been rebuilt and %s restarted.", active),
		}
	case LauncherSelected(current):
		postCommand = &component.DialogConfig{
			Title:         "Setup Complete",
			Success:       true,
			PrimaryAction: "start_launcher",
			Message:       fmt.Sprintf("The hashtable has been rebuilt. The reMarkable interface is currently stopped. Start %s to bring it back.", current),
			Actions: []component.DialogAction{
				{
					Id:    "start_launcher",
					Label: fmt.Sprintf("Start %s", current),
					Type:  "run_command",
					Value: LauncherctlBin + " start-launcher",
				},
			},
		}
	}

	return &component.HookExecutionResult{
		DialogConfig: &component.DialogConfig{
			Title:             "Rebuild Hashtable",
			Message:           "The hashtable is a map mods use to identify interface components. It's separate from system files and required for UI mods.\n\nBuilding takes about a minute and your reMarkable's interface will restart.",
			Note:              "You'll need to rebuild after each OS update.\nYou can rebuild the hashtable from the Maintenance tab.",
			ConfirmText:       "Rebuild",
			CancelText:        "Cancel",
			InProgressMessage: "If you have a passcode, enter it on your reMarkable.",
			PostCommandDialog: postCommand,
		},
		Command: &component.CommandResult{
			Script:      script,
			Description: "Rebuild Qt resource hashtable",
		},
	}, nil
}
