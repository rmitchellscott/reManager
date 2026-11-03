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
