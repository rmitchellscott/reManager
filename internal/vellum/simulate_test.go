package vellum

import (
	"reflect"
	"strings"
	"testing"

	"reManager/internal/component"
)

type recordingRunner struct {
	commands []string
	output   string
}

func (r *recordingRunner) ExecuteWithOutput(cmd string) (string, error) {
	r.commands = append(r.commands, cmd)
	return r.output, nil
}

func (r *recordingRunner) Execute(commands []component.CommandResult) error { return nil }

func (r *recordingRunner) ExecuteStreaming(cmd string, onOutput func(line string)) error { return nil }

func TestSimulateAddWithoutForbidsRemovalsAndDropsTheirPurge(t *testing.T) {
	r := &recordingRunner{output: strings.Join([]string{
		"(1/2) Purging xovi-tripletap (1.2.0-r1)",
		"(2/2) Installing launcherctl (0.2.0-r0)",
		"OK: 41 MiB in 58 packages",
	}, "\n")}
	c := &Client{executor: r}

	got, err := c.SimulateAddWithout([]string{"launcherctl"}, []string{"xovi-tripletap"})
	if err != nil {
		t.Fatalf("SimulateAddWithout: %v", err)
	}
	if want := []string{"launcherctl"}; !reflect.DeepEqual(got.Packages, want) {
		t.Errorf("packages = %v, want %v", got.Packages, want)
	}
	if len(r.commands) != 1 || !strings.HasSuffix(r.commands[0], "add --simulate 'launcherctl' '!xovi-tripletap'") {
		t.Errorf("command = %v", r.commands)
	}
}

func TestSimulateAddWithoutRemovalsIsAPlainAdd(t *testing.T) {
	r := &recordingRunner{output: "(1/1) Installing launcherctl (0.2.0-r0)\n"}
	c := &Client{executor: r}

	if _, err := c.SimulateAddWithout([]string{"launcherctl"}, nil); err != nil {
		t.Fatalf("SimulateAddWithout: %v", err)
	}
	if len(r.commands) != 1 || strings.Contains(r.commands[0], "!") {
		t.Errorf("command = %v", r.commands)
	}
}

func TestSimulateAddWithoutReportsVersionChangesOfInstalledPackages(t *testing.T) {
	r := &recordingRunner{output: strings.Join([]string{
		"(1/4) Upgrading appload (0.3.0-r0 -> 0.4.0-r0)",
		"(2/4) Downgrading qt-resource-rebuilder (0.2.0-r0 -> 0.1.0-r0)",
		"(3/4) Installing remarkdown (1.0.0-r0)",
		"(4/4) Installing trmnl (1.0.0-r0)",
		"OK: 41 MiB in 58 packages",
	}, "\n")}
	c := &Client{executor: r}

	got, err := c.SimulateAddWithout([]string{"remarkdown", "trmnl"}, nil)
	if err != nil {
		t.Fatalf("SimulateAddWithout: %v", err)
	}
	if want := []string{"appload", "qt-resource-rebuilder", "remarkdown", "trmnl"}; !reflect.DeepEqual(got.Packages, want) {
		t.Errorf("packages = %v, want %v", got.Packages, want)
	}
	want := []VersionChange{
		{Name: "appload", From: "0.3.0-r0", To: "0.4.0-r0"},
		{Name: "qt-resource-rebuilder", From: "0.2.0-r0", To: "0.1.0-r0"},
	}
	if !reflect.DeepEqual(got.VersionChanges, want) {
		t.Errorf("version changes = %v, want %v", got.VersionChanges, want)
	}
}

func TestSimulateAddWithVersionsLeavesOutPurgedPackages(t *testing.T) {
	r := &recordingRunner{output: strings.Join([]string{
		"(1/3) Purging xovi-tripletap (1.2.0-r1)",
		"(2/3) Installing launcherctl (0.2.0-r0)",
		"(3/3) Upgrading xovi (0.9.0-r0 -> 0.10.0-r0)",
		"OK: 41 MiB in 58 packages",
	}, "\n")}
	c := &Client{executor: r}

	got, err := c.SimulateAddWithVersions("launcherctl")
	if err != nil {
		t.Fatalf("SimulateAddWithVersions: %v", err)
	}
	want := []SimulatedPackage{{Name: "launcherctl", Version: "0.2.0-r0"}, {Name: "xovi", Version: "0.10.0-r0"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("packages = %v, want %v", got, want)
	}
}
