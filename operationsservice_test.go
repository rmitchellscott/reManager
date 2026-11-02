package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"

	"reManager/internal/commands"
	"reManager/internal/component"
	apperrors "reManager/internal/errors"
)

type recordedEvent struct {
	name string
	data any
}

type recorder struct {
	mu     sync.Mutex
	events []recordedEvent
	done   chan OperationResult
	prompt chan OperationPrompt
}

func newRecorder() *recorder {
	return &recorder{done: make(chan OperationResult, 4), prompt: make(chan OperationPrompt, 4)}
}

func (r *recorder) emit(name string, data any) {
	r.mu.Lock()
	r.events = append(r.events, recordedEvent{name, data})
	r.mu.Unlock()
	switch name {
	case eventOperationDone:
		r.done <- data.(OperationResult)
	case eventOperationPrompt:
		r.prompt <- data.(OperationPrompt)
	}
}

func (r *recorder) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e.name == name {
			n++
		}
	}
	return n
}

func (r *recorder) output() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out strings.Builder
	for _, e := range r.events {
		if e.name == eventOperationOutput {
			out.WriteString(e.data.(OperationOutput).Text)
		}
	}
	return out.String()
}

func (r *recorder) waitDone(t *testing.T) OperationResult {
	t.Helper()
	select {
	case result := <-r.done:
		// A second op:done would arrive right behind the first.
		time.Sleep(20 * time.Millisecond)
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("operation never finished")
		return OperationResult{}
	}
}

func (r *recorder) waitPrompt(t *testing.T) OperationPrompt {
	t.Helper()
	select {
	case prompt := <-r.prompt:
		return prompt
	case <-time.After(5 * time.Second):
		t.Fatal("operation never prompted")
		return OperationPrompt{}
	}
}

func coreWithRecorder() (*core, *recorder) {
	r := newRecorder()
	return &core{operationSink: r.emit}, r
}

func operationsWithRecorder() (*OperationsService, *recorder) {
	c, r := coreWithRecorder()
	return &OperationsService{core: c}, r
}

func TestOperationFinishesOnceWithItsError(t *testing.T) {
	c, r := coreWithRecorder()
	c.runOperation("op-1", "", func(op *Operation) (any, error) {
		return nil, errors.New("the tablet said no")
	})

	result := r.waitDone(t)
	if result.ID != "op-1" || result.OK || result.Canceled {
		t.Fatalf("result = %+v, want a failure for op-1", result)
	}
	if got := r.count(eventOperationDone); got != 1 {
		t.Errorf("op:done emitted %d times, want 1", got)
	}
	if !strings.Contains(r.output(), "The tablet said no.") {
		t.Errorf("output %q does not end with the reason", r.output())
	}
}

func TestUserErrorCodeReachesTheResult(t *testing.T) {
	c, r := coreWithRecorder()
	c.runOperation("op-1", "", func(op *Operation) (any, error) {
		return nil, apperrors.New(apperrors.ErrLockDirMissing, lockDirMissingMessage, nil, false)
	})

	result := r.waitDone(t)
	if result.Code != apperrors.ErrLockDirMissing || result.Error != lockDirMissingMessage {
		t.Errorf("result = %+v, want the UserError's code and message", result)
	}
}

func TestOperationFinishesWhenItsBodyPanics(t *testing.T) {
	c, r := coreWithRecorder()
	c.runOperation("op-1", "", func(op *Operation) (any, error) {
		panic("boom")
	})

	result := r.waitDone(t)
	if result.OK || !strings.Contains(result.Error, "boom") {
		t.Fatalf("result = %+v, want a failure naming the panic", result)
	}
}

func TestCancellingAnswersAnOpenPrompt(t *testing.T) {
	operations, r := operationsWithRecorder()
	answered := make(chan string, 1)
	operations.runOperation("op-1", "", func(op *Operation) (any, error) {
		answered <- op.Prompt(DialogRequest{Title: "Proceed?"})
		return nil, nil
	})

	r.waitPrompt(t)
	operations.CancelOperation("op-1")

	if got := <-answered; got != "cancel" {
		t.Errorf("prompt answered %q, want cancel", got)
	}
	if result := r.waitDone(t); !result.Canceled {
		t.Errorf("result = %+v, want canceled", result)
	}
}

func TestStopReportsCanceledWhenTheBodyFails(t *testing.T) {
	operations, r := operationsWithRecorder()
	started := make(chan struct{})
	operations.runOperation("op-1", "", func(op *Operation) (any, error) {
		close(started)
		<-op.ctx.Done()
		return InstallResult{}, errors.New("Installation failed: Process exited with status 143 from signal TERM")
	})

	<-started
	operations.CancelOperation("op-1")

	result := r.waitDone(t)
	if result.OK || !result.Canceled {
		t.Fatalf("result = %+v, want canceled", result)
	}
	if strings.Contains(r.output(), "signal TERM") {
		t.Errorf("output %q reports the stop as a failure", r.output())
	}
}

func TestResponseReachesOnlyItsOwnOperation(t *testing.T) {
	operations, r := operationsWithRecorder()
	answered := make(chan string, 1)
	operations.runOperation("op-1", "", func(op *Operation) (any, error) {
		answered <- op.Prompt(DialogRequest{Title: "Proceed?"})
		return nil, nil
	})

	r.waitPrompt(t)
	operations.RespondToOperation("some-other-run", "confirm")
	select {
	case got := <-answered:
		t.Fatalf("prompt answered %q by another operation's response", got)
	case <-time.After(50 * time.Millisecond):
	}

	operations.RespondToOperation("op-1", "confirm")
	if got := <-answered; got != "confirm" {
		t.Errorf("prompt answered %q, want confirm", got)
	}
	r.waitDone(t)
}

// The frontend loses an open prompt when the connection drops, so after the reconnect it asks again, and the answer still reaches the waiting operation.
func TestOpenPromptIsOfferedAgainAndAnswered(t *testing.T) {
	operations, r := operationsWithRecorder()
	answered := make(chan string, 1)
	operations.runOperation("op-1", "", func(op *Operation) (any, error) {
		answered <- op.Prompt(DialogRequest{Title: "Hashtable rebuilt"})
		return nil, nil
	})
	r.waitPrompt(t)

	pending := operations.PendingOperationPrompt()
	if pending == nil || pending.ID != "op-1" || pending.Request.Title != "Hashtable rebuilt" {
		t.Fatalf("PendingOperationPrompt() = %+v, want op-1's prompt", pending)
	}
	operations.RespondToOperation(pending.ID, "start_with_mods")
	if got := <-answered; got != "start_with_mods" {
		t.Errorf("prompt answered %q, want start_with_mods", got)
	}
	r.waitDone(t)
	if pending := operations.PendingOperationPrompt(); pending != nil {
		t.Errorf("PendingOperationPrompt() = %+v after the answer, want none", pending)
	}
}

func TestPromptWithNoSessionLeftIsCanceled(t *testing.T) {
	c, r := coreWithRecorder()
	c.runOperation("op-1", "", func(op *Operation) (any, error) {
		if response := op.Prompt(DialogRequest{Title: "Hashtable rebuilt"}); response == "cancel" {
			return nil, errOperationCanceled
		}
		return nil, nil
	})
	r.waitPrompt(t)

	c.cancelOperationsWaitingOnPrompts()
	if result := r.waitDone(t); !result.Canceled {
		t.Errorf("result = %+v, want canceled", result)
	}
}

func TestDeclinedHookCancelsWithoutRunning(t *testing.T) {
	operations, r := operationsWithRecorder()
	operations.runOperation("op-1", "", func(op *Operation) (any, error) {
		hook := &component.HookExecutionResult{
			DialogConfig: &component.DialogConfig{Title: "Rebuild hashtable", ConfirmText: "Rebuild"},
			Command:      &component.CommandResult{Script: "rebuild"},
		}
		return nil, operations.runHook(op, hook, &wailsExecutor{core: operations.core, op: op})
	})

	prompt := r.waitPrompt(t)
	operations.RespondToOperation(prompt.ID, "cancel")

	if result := r.waitDone(t); !result.Canceled || result.OK {
		t.Errorf("result = %+v, want canceled", result)
	}
	if got := r.count(eventOperationHook); got != 0 {
		t.Errorf("a declined hook started a run: %d op:hook events", got)
	}
}

// "No launchers installed" is a notice, so acknowledging it must not start a run that ends in a green success.
func TestAcknowledgedNoticeStartsNoRun(t *testing.T) {
	operations, r := operationsWithRecorder()
	operations.runOperation("op-1", "", func(op *Operation) (any, error) {
		hook := &component.HookExecutionResult{
			DialogConfig: &component.DialogConfig{Title: "No launchers installed", InfoOnly: true},
		}
		return nil, operations.runHook(op, hook, &wailsExecutor{core: operations.core, op: op})
	})

	prompt := r.waitPrompt(t)
	operations.RespondToOperation(prompt.ID, "confirm")

	if result := r.waitDone(t); !result.OK {
		t.Errorf("result = %+v, want a plain finish", result)
	}
	if got := r.count(eventOperationHook); got != 0 {
		t.Errorf("an acknowledgement started a run: %d op:hook events", got)
	}
}

// A flow that cannot start still fails its operation, so the modal it opened always gets an end.
func TestFlowsWithoutAConnectionStillFinish(t *testing.T) {
	flows := map[string]func(s *services, id string){
		"os-change upgrade": func(s *services, id string) { s.packages.RunUpgrade(id) },
		"package upgrade":   func(s *services, id string) { s.packages.RunPackageUpgrade(id, []string{"xovi"}) },
		"re-enable":         func(s *services, id string) { s.packages.RunReenable(id) },
		"install":           func(s *services, id string) { s.packages.InstallPackages(id, []string{"xovi"}, nil, "rmppure") },
		"uninstall":         func(s *services, id string) { s.packages.UninstallPackages(id, []string{"xovi"}, "rmppure") },
		"timezone":          func(s *services, id string) { s.device.SetDeviceTimezone(id, "UTC", "rmppure") },
		"reboot":            func(s *services, id string) { s.os.RebootDevice(id) },
		"partition switch":  func(s *services, id string) { s.os.SwitchBootPartition(id, 3) },
		"config save":       func(s *services, id string) { s.config.WriteConfigFile(id, "[General]\n") },
		"config backup":     func(s *services, id string) { s.config.BackupConfigFile(id) },
		"config restore":    func(s *services, id string) { s.config.RestoreConfigBackup(id, "xochitl.conf.backup-1") },
		"sleep screen set":  func(s *services, id string) { s.config.SetSleepScreen(id, "/home/root/sleep.png") },
		"sleep reset":       func(s *services, id string) { s.config.ResetSleepScreen(id) },
		"ui restart":        func(s *services, id string) { s.config.RestartXochitl(id) },
		"library rescan":    func(s *services, id string) { s.config.RescanLibrary(id) },
		"import": func(s *services, id string) {
			s.files.ImportDocuments(id, []ImportDocument{{Path: "/doc.pdf"}}, false, true)
		},
		"vellum install":   func(s *services, id string) { s.packages.BootstrapVellum(id) },
		"vellum uninstall": func(s *services, id string) { s.packages.UninstallVellum(id, false) },
		"vellum cleanup":   func(s *services, id string) { s.packages.CleanupBrokenVellum(id) },
		"backup":           func(s *services, id string) { s.files.CreateDeviceBackup(id, "/backup.tar.zst") },
		"restore":          func(s *services, id string) { s.files.RestoreDeviceBackup(id, "/backup.tar.zst") },
		"os install":       func(s *services, id string) { s.os.InstallOSVersion(id, "3.20.0.92") },
	}
	for name, start := range flows {
		t.Run(name, func(t *testing.T) {
			c, r := coreWithRecorder()
			start(newServices(c), "op-1")
			result := r.waitDone(t)
			if result.OK || result.Canceled {
				t.Errorf("result = %+v, want a failure", result)
			}
			if !strings.Contains(result.Error, "not connected") {
				t.Errorf("error = %q, want it to say the tablet is not connected", result.Error)
			}
		})
	}
}

func TestDemoFailureSwitchFailsTheNextRunOnly(t *testing.T) {
	c, r := coreWithRecorder()
	c.demo = true
	c.demoFailure = "next"

	c.runOperation("op-1", "", func(op *Operation) (any, error) {
		return nil, c.runOperationCommand(op, "true", false)
	})
	if result := r.waitDone(t); result.OK {
		t.Fatalf("armed run finished ok: %+v", result)
	}

	c.runOperation("op-2", "", func(op *Operation) (any, error) {
		return nil, c.runOperationCommand(op, "true", false)
	})
	if result := r.waitDone(t); !result.OK {
		t.Errorf("run after the armed one failed: %+v", result)
	}
}

func TestDemoFailureStopsAnInstallHalfway(t *testing.T) {
	c, r := coreWithRecorder()
	c.demo = true
	c.demoPackages = testDemoPackages()
	c.demoFailure = "next"
	packages := newServices(c).packages

	packages.UninstallPackages("op-1", []string{"xovi", "appload"}, "rmppure")
	result := r.waitDone(t)
	if result.OK {
		t.Fatalf("armed uninstall finished ok: %+v", result)
	}
	if _, stillThere := packages.demoInstalled()["xovi"]; !stillThere {
		t.Error("a failed demo uninstall still removed its packages")
	}
}

func TestFailedImportLeavesTheOtherDocumentsImported(t *testing.T) {
	c, r := coreWithRecorder()
	c.demo = true
	c.demoFailure = "next"
	files := newServices(c).files

	dir := t.TempDir()
	var documents []ImportDocument
	for _, name := range []string{"first", "second"} {
		documentPath := filepath.Join(dir, name+".epub")
		if err := os.WriteFile(documentPath, []byte("epub"), 0o600); err != nil {
			t.Fatal(err)
		}
		documents = append(documents, ImportDocument{Path: documentPath, VisibleName: name})
	}

	files.ImportDocuments("op-1", documents, false, false)
	result := r.waitDone(t)
	if result.OK || !strings.Contains(result.Error, "1 of 2") {
		t.Fatalf("result = %+v, want a failure naming one of two documents", result)
	}

	finalStatus := map[int]InstallStatus{}
	r.mu.Lock()
	for _, e := range r.events {
		if e.name == eventOperationProgress {
			progress := e.data.(OperationProgress).Progress
			finalStatus[progress.Index] = progress.Status
		}
	}
	r.mu.Unlock()
	if finalStatus[0] != InstallError || finalStatus[1] != InstallCompleted {
		t.Errorf("final statuses = %v, want the first failed and the second completed", finalStatus)
	}
}

func TestDemoFailureReachesVellumAndSupportBundles(t *testing.T) {
	flows := map[string]func(s *services, id string){
		"vellum install":        func(s *services, id string) { s.packages.BootstrapVellum(id) },
		"vellum uninstall":      func(s *services, id string) { s.packages.UninstallVellum(id, false) },
		"support bundle save":   func(s *services, id string) { s.support.GenerateSupportBundle(id, "") },
		"support bundle upload": func(s *services, id string) { s.support.UploadSupportBundle(id, "") },
		"support bundle update": func(s *services, id string) { s.support.AppendSupportBundle(id, "remote-1", "") },
	}
	for name, start := range flows {
		t.Run(name, func(t *testing.T) {
			c, r := coreWithRecorder()
			c.demo = true
			c.demoFailure = "next"
			start(newServices(c), "op-1")
			if result := r.waitDone(t); result.OK || result.Error != demoFailureMessage {
				t.Errorf("result = %+v, want the demo failure", result)
			}
		})
	}
}

// The slot is held until the run returns, so a second backup started while the first is still stopping is refused rather than writing alongside it.
func TestBackupRefusedWhileAnotherHoldsTheSlot(t *testing.T) {
	c, r := coreWithRecorder()
	c.claimBackupSlot()
	newServices(c).files.CreateDeviceBackup("op-1", "/backup.tar.zst")
	if result := r.waitDone(t); result.OK || result.Error != errBackupBusy.Error() {
		t.Errorf("result = %+v, want the busy refusal", result)
	}
}

func TestDemoShellArmsAndClearsFailures(t *testing.T) {
	shell := &ShellService{core: &core{demo: true}}
	if reply := shell.demoShellCommand("fail always"); !strings.Contains(reply, "always") {
		t.Errorf("reply = %q", reply)
	}
	firstRunFails, secondRunFails := shell.takeDemoFailure(), shell.takeDemoFailure()
	if !firstRunFails || !secondRunFails {
		t.Error("fail always should fail every run")
	}
	shell.demoShellCommand("fail off")
	if shell.takeDemoFailure() {
		t.Error("fail off should stop failures")
	}
}

func assertWritableRootRestored(t *testing.T, r *recorder) {
	t.Helper()
	_, restore := commands.WritableRootSteps(rmdevice.RMPPure)
	for _, step := range restore {
		if !strings.Contains(r.output(), step.Script) {
			t.Errorf("restore step %q did not run", step.Description)
		}
	}
}

func TestWritableRootIsRestoredWhenAStepFails(t *testing.T) {
	c := &core{demo: true, demoFailure: "next"}
	r := newRecorder()
	op := newOperation(context.Background(), "op-1", r.emit)
	task := []component.CommandResult{{Script: "timedatectl set-timezone UTC"}}

	if err := c.runOperationCommandsWithWritableRoot(op, rmdevice.RMPPure, task); err == nil {
		t.Fatal("a run with a failing step reported success")
	}
	assertWritableRootRestored(t, r)
}

func TestWritableRootIsRestoredWhenTheOperationIsCanceled(t *testing.T) {
	c := &core{demo: true}
	r := newRecorder()
	op := newOperation(context.Background(), "op-1", r.emit)
	op.cancel()
	task := []component.CommandResult{{Script: "timedatectl set-timezone UTC"}}

	if err := c.runOperationCommandsWithWritableRoot(op, rmdevice.RMPPure, task); !errors.Is(err, errOperationCanceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
	assertWritableRootRestored(t, r)
}

func TestStoppedRunReportsWhatApkChanged(t *testing.T) {
	before := map[string]string{"xovi": "1.0-r0", "appload": "0.3-r0", "koreader": "2024.11-r0"}
	after := map[string]string{"xovi": "1.1-r0", "appload": "0.3-r0", "zig": "0.14-r0"}
	got := describePackageChanges(before, after)
	want := "\nStopped, but apk had already made changes.\nInstalled: zig 0.14-r0.\nUpgraded: xovi 1.0-r0 to 1.1-r0.\nRemoved: koreader.\n"
	if got != want {
		t.Errorf("describePackageChanges = %q, want %q", got, want)
	}
	if got := describePackageChanges(before, before); got != "\nStopped before any package changed.\n" {
		t.Errorf("unchanged packages reported as %q", got)
	}
}
