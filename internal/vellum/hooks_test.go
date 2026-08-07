package vellum

import (
	"fmt"
	"strings"
	"testing"

	"reManager/internal/component"
)

type fakeRunner struct {
	responses map[string]string
	errs      map[string]error
	exitErrs  map[string]error
}

func (f *fakeRunner) ExecuteWithOutput(command string) (string, error) {
	for fragment, err := range f.errs {
		if strings.Contains(command, fragment) {
			return "", err
		}
	}
	for fragment, out := range f.responses {
		if strings.Contains(command, fragment) {
			for errFragment, err := range f.exitErrs {
				if strings.Contains(command, errFragment) {
					return out, err
				}
			}
			return out, nil
		}
	}
	return "", fmt.Errorf("unexpected command: %s", command)
}

func probe(list, current, active string) map[string]string {
	return map[string]string{
		"list-launchers": strings.Join([]string{list, current, active}, "\n"+LauncherProbeMarker+"\n"),
	}
}

func actionValues(actions []component.DialogAction) map[string]string {
	out := make(map[string]string, len(actions))
	for _, a := range actions {
		out[a.Id] = a.Label
	}
	return out
}

func TestLauncherSelectDialog(t *testing.T) {
	ctx := component.CommandContext{Exec: &fakeRunner{responses: probe("appload\nnone\noxide", "oxide", "oxide")}}

	result, err := LauncherSelectDialog(ctx)
	if err != nil {
		t.Fatal(err)
	}

	labels := actionValues(result.DialogConfig.Actions)
	if len(labels) != 2 {
		t.Fatalf("expected appload and stock, got %v", labels)
	}
	if _, ok := labels["oxide"]; ok {
		t.Error("switching to the current launcher is not an option")
	}
	if labels["appload"] != "appload" {
		t.Errorf("appload label = %q", labels["appload"])
	}
	if result.DialogConfig.Actions[len(result.DialogConfig.Actions)-1].Id != StockLauncher {
		t.Errorf("stock should sort last, got %+v", result.DialogConfig.Actions)
	}
	if labels["none"] != "No launcher" {
		t.Errorf("stock label = %q", labels["none"])
	}
	if !strings.Contains(result.DialogConfig.Message, "oxide") {
		t.Errorf("message should name the current launcher, got %q", result.DialogConfig.Message)
	}
	if result.Command != nil {
		t.Error("the chooser runs the action the user picks, not a fixed command")
	}
	for _, a := range result.DialogConfig.Actions {
		if !strings.HasPrefix(a.Value, LauncherctlBin+" switch-launcher ") {
			t.Errorf("action %q value = %q", a.Id, a.Value)
		}
	}
}

func TestLauncherSelectDialogNoCurrent(t *testing.T) {
	ctx := component.CommandContext{Exec: &fakeRunner{responses: probe("appload", "Error: No launcher currently set", "")}}

	result, err := LauncherSelectDialog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	labels := actionValues(result.DialogConfig.Actions)
	if labels["appload"] != "appload" {
		t.Errorf("nothing should be marked current, got %v", labels)
	}
	if strings.Contains(result.DialogConfig.Message, "currently active") {
		t.Errorf("message should not claim an active launcher, got %q", result.DialogConfig.Message)
	}
}

func TestLauncherSelectDialogOnlyStock(t *testing.T) {
	ctx := component.CommandContext{Exec: &fakeRunner{responses: probe("none", "none", "")}}

	result, err := LauncherSelectDialog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.DialogConfig.Actions) != 0 {
		t.Errorf("stock alone is not a choice, got %+v", result.DialogConfig.Actions)
	}
	if !result.DialogConfig.InfoOnly {
		t.Error("expected an informational dialog when nothing is installed")
	}
}

func TestLauncherSelectDialogIgnoresStderrNoise(t *testing.T) {
	ctx := component.CommandContext{Exec: &fakeRunner{responses: probe(
		"[sentry] DEBUG crash-safe logs flush\nappload\n[sentry] crash has been captured\nnone",
		"[sentry] DEBUG crash-safe metrics flush\nappload",
		"appload")}}

	result, err := LauncherSelectDialog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	labels := actionValues(result.DialogConfig.Actions)
	if len(labels) != 1 || labels[StockLauncher] != "No launcher" {
		t.Fatalf("appload is current, so only stock remains, got %v", labels)
	}
}

func TestLauncherSelectDialogToleratesNonZeroExit(t *testing.T) {
	runner := &fakeRunner{
		responses: probe("appload\nnone", "none", ""),
		exitErrs:  map[string]error{"active-launcher": fmt.Errorf("Process exited with status 1")},
	}

	result, err := LauncherSelectDialog(component.CommandContext{Exec: runner})
	if err != nil {
		t.Fatalf("a launcher that is not running must not break the chooser: %v", err)
	}
	if labels := actionValues(result.DialogConfig.Actions); len(labels) != 1 || labels["appload"] != "appload" {
		t.Errorf("actions = %v, want appload", labels)
	}
}

func TestLauncherSelectDialogNeedsDevice(t *testing.T) {
	if _, err := LauncherSelectDialog(component.CommandContext{}); err == nil {
		t.Error("expected an error without an executor")
	}
}

func TestLauncherSelectDialogMessages(t *testing.T) {
	cases := []struct {
		name, current, active, want string
	}{
		{"running", "appload\n", "appload\n", "appload is running."},
		{"selected but stopped", "appload\n", "", "appload is selected but not running."},
		{"nothing selected", "none\n", "", "No launcher is selected."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := component.CommandContext{Exec: &fakeRunner{responses: probe("appload\noxide\nnone", tc.current, tc.active)}}
			result, err := LauncherSelectDialog(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(result.DialogConfig.Message, tc.want) {
				t.Errorf("message = %q, want prefix %q", result.DialogConfig.Message, tc.want)
			}
		})
	}
}

func launcherProbe(current, active string) map[string]string {
	return map[string]string{
		"current-launcher": current + "\n" + LauncherProbeMarker + "\n" + active,
	}
}

func TestRebuildHashtableDialogOffersXoviWithoutLauncher(t *testing.T) {
	for _, current := range []string{"", "none"} {
		exec := &fakeRunner{responses: launcherProbe(current, current)}
		result, err := RebuildHashtableDialog(component.CommandContext{Exec: exec})
		if err != nil {
			t.Fatalf("current=%q: %v", current, err)
		}

		if strings.Contains(result.Command.Script, "launcherctl") {
			t.Errorf("current=%q: no launcher should mean no launcherctl in %q", current, result.Command.Script)
		}
		actions := actionValues(result.DialogConfig.PostCommandDialog.Actions)
		if _, ok := actions["start_with_mods"]; !ok {
			t.Errorf("current=%q: expected xovi start action, got %v", current, actions)
		}
	}
}

func TestRebuildHashtableDialogRestartsRunningLauncher(t *testing.T) {
	exec := &fakeRunner{responses: launcherProbe("oxide", "oxide")}
	result, err := RebuildHashtableDialog(component.CommandContext{Exec: exec})
	if err != nil {
		t.Fatal(err)
	}

	script := result.Command.Script
	for _, want := range []string{"stop-launcher", "rebuild_hashtable", "start-launcher", "exit $rc"} {
		if !strings.Contains(script, want) {
			t.Errorf("script %q missing %q", script, want)
		}
	}
	if strings.Index(script, "stop-launcher") > strings.Index(script, "rebuild_hashtable") {
		t.Errorf("launcher must stop before the rebuild: %q", script)
	}
	if strings.Index(script, "start-launcher") < strings.Index(script, "rebuild_hashtable") {
		t.Errorf("launcher must restart after the rebuild: %q", script)
	}

	post := result.DialogConfig.PostCommandDialog
	if len(post.Actions) != 0 {
		t.Errorf("an auto-restarted launcher needs no start action, got %v", actionValues(post.Actions))
	}
	if !strings.Contains(post.Message, "oxide") {
		t.Errorf("post-command message should name the launcher: %q", post.Message)
	}
}

func TestRebuildHashtableDialogOffersStartForStoppedLauncher(t *testing.T) {
	exec := &fakeRunner{responses: launcherProbe("oxide", "none")}
	result, err := RebuildHashtableDialog(component.CommandContext{Exec: exec})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result.Command.Script, "stop-launcher") {
		t.Errorf("a stopped launcher needs no stop: %q", result.Command.Script)
	}
	actions := actionValues(result.DialogConfig.PostCommandDialog.Actions)
	if label, ok := actions["start_launcher"]; !ok || label != "Start oxide" {
		t.Errorf("got %v, want start_launcher / \"Start oxide\"", actions)
	}
}

func TestRebuildHashtableDialogWithoutDevice(t *testing.T) {
	result, err := RebuildHashtableDialog(component.CommandContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := actionValues(result.DialogConfig.PostCommandDialog.Actions)["start_with_mods"]; !ok {
		t.Error("no exec available should fall back to the xovi actions")
	}
}

func TestLauncherSelected(t *testing.T) {
	for name, want := range map[string]bool{"": false, "none": false, "oxide": true, "appload": true} {
		if got := LauncherSelected(name); got != want {
			t.Errorf("LauncherSelected(%q) = %v, want %v", name, got, want)
		}
	}
}
