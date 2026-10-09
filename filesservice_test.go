package main

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"reManager/internal/executor"
)

func TestRestoreUIControlStartsAStoppedUI(t *testing.T) {
	cases := []struct {
		name        string
		status      XochitlStatus
		wantRunning bool
		wantStart   string
	}{
		{"stock UI running", XochitlStatus{Running: true}, true, executor.WithXochitlStartReset("systemctl start xochitl")},
		{"UI running with mods", XochitlStatus{Running: true, XoviActive: true}, true, executor.WithXochitlStartReset("/home/root/xovi/start")},
		{"stock UI already stopped", XochitlStatus{Running: false}, false, executor.WithXochitlStartReset("systemctl start xochitl")},
		{"launcher selected", XochitlStatus{ActiveLauncher: "oxide"}, true, executor.WithXochitlStartReset("/home/root/.vellum/bin/launcherctl start-launcher")},
		{"stock launcher", XochitlStatus{Running: true, ActiveLauncher: "none"}, true, executor.WithXochitlStartReset("systemctl start xochitl")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ui := restoreUIControlFor(c.status)
			if ui.running != c.wantRunning {
				t.Errorf("running = %t, want %t", ui.running, c.wantRunning)
			}
			if ui.startCommand != c.wantStart {
				t.Errorf("startCommand = %q, want %q", ui.startCommand, c.wantStart)
			}
		})
	}
}

func TestBackupSlotAllowsOneOperationAtATime(t *testing.T) {
	c := &core{}

	if !c.claimBackupSlot() {
		t.Fatal("first claim refused")
	}
	if c.claimBackupSlot() {
		t.Fatal("second claim allowed while the first holds the slot")
	}

	c.releaseBackupSlot()
	if !c.claimBackupSlot() {
		t.Fatal("claim refused after release")
	}
}

func TestIsSystemPath(t *testing.T) {
	cases := map[string]bool{
		"/home/root":                     false,
		"/home/root/":                    false,
		"/home/root/.local/share":        false,
		"/home/root/../root/x":           false,
		"/home/rootfoo":                  true,
		"/home/root/../../etc/profile.d": true,
		"/etc/xochitl.conf":              true,
		"/":                              true,
	}
	for remotePath, want := range cases {
		if got := isSystemPath(remotePath); got != want {
			t.Errorf("isSystemPath(%q) = %v, want %v", remotePath, got, want)
		}
	}
}

func TestWritableRootReleaseFromAnEndedConnectionIsIgnored(t *testing.T) {
	endedClient, currentClient := &ssh.Client{}, &ssh.Client{}
	c := &core{writableRootClient: currentClient, writableRootCount: 2}

	if err := c.releaseWritableRoot(endedClient); err != nil {
		t.Fatal(err)
	}
	if c.writableRootCount != 2 {
		t.Errorf("count = %d after a stale release, want 2", c.writableRootCount)
	}
	if err := c.releaseWritableRoot(currentClient); err != nil {
		t.Fatal(err)
	}
	if c.writableRootCount != 1 {
		t.Errorf("count = %d after a release, want 1", c.writableRootCount)
	}
}

func TestQueuedTransferDoesNotRunOnALaterConnection(t *testing.T) {
	files := newServices(&core{}).files
	job := &transferJob{kind: TransferUpload, connGen: files.connGen.Load()}
	files.connGen.Add(1)

	if err := files.transferManager().run(job); err == nil || !strings.Contains(err.Error(), "disconnected before this transfer started") {
		t.Errorf("run on a later connection = %v, want the ended-connection error", err)
	}
}

func TestIsProtectedFromDelete(t *testing.T) {
	protected := []string{"/", "", "/etc", "/home", "/home/root", "/home/root/", "/usr/../etc", "etc"}
	for _, p := range protected {
		if !isProtectedFromDelete(p) {
			t.Errorf("%q is deletable", p)
		}
	}
	deletable := []string{"/home/root/notes", "/home/root/.local/share/remarkable/xochitl/x", "/etc/profile.d"}
	for _, p := range deletable {
		if isProtectedFromDelete(p) {
			t.Errorf("%q is protected", p)
		}
	}
}

var testGuideRevisions = []guideRevision{
	{Revision: 4, MinApp: "2.0.0", SHA256: "launch", Epub: "4.epub"},
	{Revision: 5, MinApp: "2.1.0", SHA256: "two-one", Epub: "5.epub"},
	{Revision: 6, MinApp: "2.0.0", SHA256: "backport", Epub: "6.epub"},
	{Revision: 7, MinApp: "2.2.0", SHA256: "future", Epub: "7.epub"},
}

func TestGuideTargetIsTheHighestRankedRevisionTheAppMeets(t *testing.T) {
	cases := []struct {
		appVersion string
		want       int
	}{
		{"1.9.0", 0},
		{"2.0.1", 6},
		{"2.1.0", 5},
		{"2.1.3", 5},
		{"2.2.0", 7},
	}
	for _, c := range cases {
		t.Run(c.appVersion, func(t *testing.T) {
			got := 0
			if target, ok := guideTarget(testGuideRevisions, c.appVersion); ok {
				got = target.Revision
			}
			if got != c.want {
				t.Errorf("target revision = %d, want %d", got, c.want)
			}
		})
	}
}

func TestGuideOutOfDateNeverDowngrades(t *testing.T) {
	backport, _ := guideTarget(testGuideRevisions, "2.0.1")
	cases := []struct {
		name      string
		installed string
		want      bool
	}{
		{"older revision", "launch", true},
		{"1.x copy", "legacy", true},
		{"the target", "backport", false},
		{"a revision for a newer app", "two-one", false},
		{"unknown copy", "edited", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := guideOutOfDate(testGuideRevisions, backport, c.installed); got != c.want {
				t.Errorf("out of date = %t, want %t", got, c.want)
			}
		})
	}
}
