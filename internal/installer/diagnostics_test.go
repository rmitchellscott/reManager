package installer

import (
	"fmt"
	"strings"
	"testing"
)

func TestDiagnosticsPrefersApkErrors(t *testing.T) {
	var d vellumDiagnostics
	for _, line := range []string{
		"(1/8) Installing oxide (3.1-r0)",
		"WARNING: The repository tag for world dependency 'fbinfo@oxide' does not exist",
		"ERROR: Not committing changes due to missing repository tags.",
		"/home/root/.vellum/bin/vellum failed with exit status 1.",
	} {
		d.observe(line)
	}

	got := d.explain(fmt.Errorf("Process exited with status 1"))
	if !strings.Contains(got, "missing repository tags") {
		t.Errorf("explain() = %q, want the apk error", got)
	}
	if strings.Contains(got, "failed with exit status") {
		t.Errorf("explain() = %q, should not report the wrapper epilogue", got)
	}
}

func TestDiagnosticsFallsBackToLastLine(t *testing.T) {
	var d vellumDiagnostics
	d.observe("doing something")
	d.observe("  ")
	d.observe("it went wrong somehow")

	got := d.explain(fmt.Errorf("exit 1"))
	if !strings.Contains(got, "it went wrong somehow") {
		t.Errorf("explain() = %q, want the last meaningful line", got)
	}
}

func TestDiagnosticsWithoutOutput(t *testing.T) {
	var d vellumDiagnostics
	if got := d.explain(fmt.Errorf("not connected")); got != "not connected" {
		t.Errorf("explain() = %q, want the bare error", got)
	}
}

func TestDiagnosticsCapsLines(t *testing.T) {
	var d vellumDiagnostics
	for i := 0; i < maxDiagnosticLines*2; i++ {
		d.observe(fmt.Sprintf("ERROR: problem %d", i))
	}

	got := d.explain(fmt.Errorf("exit 1"))
	if lines := strings.Count(got, "\n") + 1; lines != maxDiagnosticLines {
		t.Errorf("kept %d lines, want %d", lines, maxDiagnosticLines)
	}
	if !strings.Contains(got, fmt.Sprintf("problem %d", maxDiagnosticLines*2-1)) {
		t.Errorf("explain() = %q, want the most recent errors", got)
	}
}

func TestParsePurgedPackage(t *testing.T) {
	cases := map[string]string{
		"(1/4) Purging launcherctl-oxide (3.1-r0)": "launcherctl-oxide",
		"(3/4) Purging jq (1.8.1-r2)":              "jq",
		"( 2/10) Removing oxide-utils (3.1-r0)":    "oxide-utils",
		"  Executing launcherctl-3.1-r0.pre-deinstall": "",
		"OK: 7020 KiB in 5 packages":                   "",
		"1 error; 18.4 MiB in 9 packages":              "",
	}

	for line, want := range cases {
		if got := parsePurgedPackage(line); got != want {
			t.Errorf("parsePurgedPackage(%q) = %q, want %q", line, got, want)
		}
	}
}
