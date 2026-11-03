package executor

import (
	"os/exec"
	"strings"
	"testing"
)

func TestWithDevicePathExportsForEveryCommandInAScript(t *testing.T) {
	script := WithDevicePath(`echo "$PATH" && sh -c 'echo "$PATH"'`)

	out, err := exec.Command("sh", "-c", script).Output()
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %q", out)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "/home/root/.vellum/bin:") {
			t.Errorf("PATH %q does not start with the Vellum bin directory", line)
		}
	}
}

func TestWithDevicePathKeepsExistingPath(t *testing.T) {
	out, err := exec.Command("sh", "-c", WithDevicePath(`echo "$PATH"`)).Output()
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !strings.HasSuffix(strings.TrimSpace(string(out)), ":/sbin:"+currentPath(t)) {
		t.Errorf("existing PATH not appended: %q", out)
	}
}

func TestWithDevicePathPassesExitCodeThrough(t *testing.T) {
	err := exec.Command("sh", "-c", WithDevicePath("exit 3")).Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 3 {
		t.Errorf("expected exit code 3, got %v", err)
	}
}

func currentPath(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", `echo "$PATH"`).Output()
	if err != nil {
		t.Fatalf("reading PATH: %v", err)
	}
	return strings.TrimSpace(string(out))
}
