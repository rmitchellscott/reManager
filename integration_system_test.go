//go:build integration

package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	rmversion "github.com/rmitchellscott/remarkable-go/version"

	"reManager/internal/vellum"
)

const systemTestDevice = "rmpp"

func declinePrompts(DialogRequest) string { return "cancel" }

func runSystemTask(t *testing.T, app *testApp, taskID string) operationRun {
	t.Helper()
	return runOp(t, app, time.Minute, nil, func(id string) { app.RunSystemTask(id, taskID, systemTestDevice) })
}

func runMaintenanceCommand(t *testing.T, app *testApp, pkg, command string, answer promptAnswerer) operationRun {
	t.Helper()
	return runOp(t, app, 2*time.Minute, answer, func(id string) { app.RunMaintenanceCommand(id, pkg, command, systemTestDevice) })
}

func requireOperationOK(t *testing.T, what string, run operationRun) {
	t.Helper()
	if !run.result.OK {
		t.Fatalf("%s failed: %s\n%s", what, run.result.Error, run.output)
	}
}

func currentXochitlStatus(t *testing.T, app *testApp) XochitlStatus {
	t.Helper()
	status, err := app.GetXochitlStatus()
	if err != nil {
		t.Fatalf("GetXochitlStatus: %v", err)
	}
	return status
}

func xochitlPID(tablet *fakeTablet) string {
	return strings.TrimSpace(tablet.run("pidof xochitl || true"))
}

func succeedsOnTablet(tablet *fakeTablet, script string) bool {
	return exec.Command("docker", "exec", tablet.container, "bash", "-c", script).Run() == nil
}

func waitOnTablet(t *testing.T, tablet *fakeTablet, script string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !succeedsOnTablet(tablet, script) {
		if time.Now().After(deadline) {
			t.Fatalf("on tablet, %q never succeeded within %v", script, timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func installForSystemTest(t *testing.T, app *testApp, packages ...string) operationRun {
	t.Helper()
	return runOp(t, app, 5*time.Minute, declinePrompts, func(id string) {
		app.InstallPackages(id, packages, nil, systemTestDevice)
	})
}

func modsTablet(t *testing.T, extra ...string) (*fakeTablet, *testApp) {
	t.Helper()
	tablet, app := vellumTablet(t)
	requireOperationOK(t, "install", installForSystemTest(t, app, append([]string{"xovi-extensions", "qt-resource-rebuilder"}, extra...)...))
	return tablet, app
}

// launcherctl and oxide-utils' trigger run glibc Qt binaries that the fake's Alpine userland cannot execute, so apk reports their scripts failing.
var unrunnableLauncherScript = regexp.MustCompile(`^lib/apk/exec/(launcherctl|oxide-utils)-[^ ]*\.(post-install|trigger): exited with error 127`)

func launcherTablet(t *testing.T) (*fakeTablet, *testApp) {
	t.Helper()
	tablet, app := modsTablet(t)
	run := installForSystemTest(t, app, "launcherctl-appload", "launcherctl-oxide")
	if !run.result.OK {
		for _, failure := range strings.Split(run.result.Error, "ERROR: ")[1:] {
			if !unrunnableLauncherScript.MatchString(strings.TrimSpace(failure)) {
				t.Fatalf("launcher install: %s\n%s", run.result.Error, run.output)
			}
		}
	}
	tablet.run("install -m 755 /fake/launcherctl /home/root/.vellum/bin/launcherctl && /fake/write-hashtab /home/root/xovi/exthome/qt-resource-rebuilder/hashtab")
	return tablet, app
}

func TestSystemRestartUIRestartsXochitl(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	tasks := app.GetSystemTasksInfo()
	restart := slices.IndexFunc(tasks, func(task SystemTaskInfo) bool { return task.ID == "restart-xochitl" })
	if restart < 0 || tasks[restart].NeedsWritableRoot {
		t.Fatalf("restart-xochitl task: %+v", tasks)
	}

	before := xochitlPID(tablet)
	run := runSystemTask(t, app, "restart-xochitl")
	requireOperationOK(t, "restart", run)
	if !strings.Contains(tablet.run("cat /var/log/fake-systemd.log"), "systemctl restart xochitl") {
		t.Error("xochitl was never restarted through systemctl")
	}
	after := xochitlPID(tablet)
	if after == "" || after == before {
		t.Errorf("xochitl pid went from %q to %q", before, after)
	}
	if !currentXochitlStatus(t, app).Running {
		t.Error("xochitl is not reported running after the restart")
	}
}

func TestSystemStartWithAndWithoutMods(t *testing.T) {
	requireDocker(t)
	_, app := modsTablet(t)
	if currentXochitlStatus(t, app).XoviActive {
		t.Fatal("xovi is active before anything started it")
	}

	requireOperationOK(t, "Start with mods", runMaintenanceCommand(t, app, "xovi-extensions", "start", nil))
	if status := currentXochitlStatus(t, app); !status.Running || !status.XoviActive {
		t.Errorf("after Start with mods: %+v", status)
	}

	requireOperationOK(t, "Start without mods", runMaintenanceCommand(t, app, "xovi-extensions", "stock", nil))
	if status := currentXochitlStatus(t, app); !status.Running || status.XoviActive {
		t.Errorf("after Start without mods: %+v", status)
	}
}

func TestSystemLauncherStartAndStop(t *testing.T) {
	requireDocker(t)
	tablet, app := launcherTablet(t)
	tablet.run("/home/root/.vellum/bin/launcherctl switch-launcher appload --start")
	if status := currentXochitlStatus(t, app); status.CurrentLauncher != "appload" || status.ActiveLauncher != "appload" {
		t.Fatalf("appload is not the running launcher: %+v", status)
	}

	requireOperationOK(t, "stop launcher", runMaintenanceCommand(t, app, "launcherctl", "stop-launcher", nil))
	if status := currentXochitlStatus(t, app); status.ActiveLauncher != "" || status.Running || status.CurrentLauncher != "appload" {
		t.Errorf("after Stop: %+v", status)
	}

	requireOperationOK(t, "start launcher", runMaintenanceCommand(t, app, "launcherctl", "start-launcher", nil))
	if status := currentXochitlStatus(t, app); status.ActiveLauncher != "appload" || !status.Running || !status.XoviActive {
		t.Errorf("after Start: %+v", status)
	}

	before := xochitlPID(tablet)
	requireOperationOK(t, "restart launcher", runMaintenanceCommand(t, app, "launcherctl", "restart-launcher", nil))
	if after := xochitlPID(tablet); after == "" || after == before {
		t.Errorf("restart-launcher left xochitl at pid %q, was %q", after, before)
	}
}

func TestSystemHashtableRebuildEndsInStartChoice(t *testing.T) {
	requireDocker(t)
	tablet, app := modsTablet(t)
	tablet.run(`sed -i 's/=` + currentOSVersion + `$/=3.26.0.68/' /usr/share/remarkable/update.conf && /fake/write-hashtab /home/root/xovi/exthome/qt-resource-rebuilder/hashtab && sed -i 's/=3.26.0.68$/=` + currentOSVersion + `/' /usr/share/remarkable/update.conf`)
	stale, err := app.CheckHashtabVersion()
	if err != nil || !stale.NeedsRebuild {
		t.Fatalf("an old hashtab should need a rebuild: %+v %v", stale, err)
	}

	from := emitted.mark()
	run := runMaintenanceCommand(t, app, "qt-resource-rebuilder", "rebuild_hashtable", func(req DialogRequest) string {
		if req.Title == "Rebuild hashtable" {
			return "confirm"
		}
		return actionLabeled(req, "Start with mods")
	})
	requireOperationOK(t, "rebuild", run)

	titles := make([]string, len(run.prompts))
	for i, prompt := range run.prompts {
		titles[i] = prompt.Title
	}
	if !slices.Equal(titles, []string{"Rebuild hashtable", "Hashtable rebuilt"}) {
		t.Errorf("prompts: %v", titles)
	}
	if len(run.prompts) == 2 && actionLabeled(run.prompts[1], "Start without mods") == "" {
		t.Errorf("the post dialog offers no Start without mods: %+v", run.prompts[1].Actions)
	}

	var hooks []OperationHook
	for _, event := range emitted.since(from) {
		if hook, ok := event.Data.(OperationHook); ok {
			hooks = append(hooks, hook)
		}
	}
	if len(hooks) != 2 || hooks[0].ExpectedSeconds != 65 || hooks[0].Finished || !hooks[1].Finished || hooks[1].Error != "" {
		t.Errorf("op:hook events: %+v", hooks)
	}
	if count := strings.Count(run.output, "$ /home/root/xovi/rebuild_hashtable"); count != 1 {
		t.Errorf("the rebuild command was printed %d times:\n%s", count, run.output)
	}

	status, err := app.CheckHashtabVersion()
	if err != nil || !status.Installed || status.NeedsRebuild || status.HashtabVersion != currentOSVersion || status.FirmwareVersion != currentOSVersion {
		t.Errorf("hashtab after the rebuild: %+v %v", status, err)
	}
	if xochitl := currentXochitlStatus(t, app); !xochitl.Running || !xochitl.XoviActive {
		t.Errorf("Start with mods did not leave xovi running: %+v", xochitl)
	}
}

func TestSystemReenableRestoresSystemFiles(t *testing.T) {
	requireDocker(t)
	tablet, app := modsTablet(t)
	tablet.run("rm -f /etc/vellum/reenabled")
	if status := strings.TrimSpace(tablet.run("/home/root/.vellum/bin/vellum reenable status || true")); status != "needed" {
		t.Fatalf("reenable status before the run: %q", status)
	}

	run := runOp(t, app, 2*time.Minute, nil, func(id string) { app.RunReenable(id) })
	requireOperationOK(t, "re-enable", run)
	if run.result.Reenable != vellum.ReenableOK {
		t.Errorf("re-enable reported status %q", run.result.Reenable)
	}
	for _, want := range []string{"qt-resource-rebuilder", "Re-enable complete."} {
		if !strings.Contains(run.output, want) {
			t.Errorf("output lacks %q:\n%s", want, run.output)
		}
	}
	tablet.run("test -e /etc/vellum/reenabled")

	from := emitted.mark()
	if result := app.Connect(tablet.addr, tablet.password); !result.Success {
		t.Fatalf("reconnect: %s", result.Message)
	}
	event, _ := emitted.waitFor(t, from, time.Minute, "connect:warnings")
	if status := event.Data.(ConnectWarnings).ReenableStatus; status != vellum.ReenableOK {
		t.Errorf("connect:warnings reenableStatus = %v", status)
	}
}

// Connect with a password leaves the device unsaved, which has nowhere to keep a timezone, so the connect page saves it straight after, as this does.
func connectSavedDevice(t *testing.T, tablet *fakeTablet) *testApp {
	t.Helper()
	app := connectApp(t, tablet)
	if _, err := app.SaveDevice("", "Fake reMarkable", tablet.addr, "password", tablet.password, "", "", ""); err != nil {
		t.Fatalf("SaveDevice: %v", err)
	}
	return app
}

func TestSystemTimezoneRelinksLocaltime(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectSavedDevice(t, tablet)

	if err := app.SaveDeviceTimezone("America/Denver"); err != nil {
		t.Fatal(err)
	}
	mismatch, err := app.GetTimezoneStatus()
	if err != nil || mismatch.DeviceTimezone != "UTC" || mismatch.SavedTimezone != "America/Denver" || !mismatch.NeedsUpdate {
		t.Fatalf("timezone before setting it: %+v %v", mismatch, err)
	}

	run := runOp(t, app, time.Minute, nil, func(id string) { app.SetDeviceTimezone(id, "America/Denver", systemTestDevice) })
	requireOperationOK(t, "set timezone", run)
	if run.result.Value != "America/Denver" {
		t.Errorf("result value %q", run.result.Value)
	}
	if link := strings.TrimSpace(tablet.run("readlink /etc/localtime")); link != "/usr/share/zoneinfo/America/Denver" {
		t.Errorf("/etc/localtime -> %s", link)
	}
	status, err := app.GetTimezoneStatus()
	if err != nil || status.DeviceTimezone != "America/Denver" || status.NeedsUpdate {
		t.Errorf("timezone after setting it: %+v %v", status, err)
	}
}

func TestSystemAutomaticUpdatesToggle(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	updateStatus := func() (enabled, running bool) {
		t.Helper()
		status, err := app.GetUpdateServiceStatus()
		if err != nil {
			t.Fatalf("GetUpdateServiceStatus: %v", err)
		}
		return status.Enabled, status.Running
	}
	if enabled, running := updateStatus(); !enabled || !running {
		t.Fatalf("updates start out enabled=%v running=%v", enabled, running)
	}

	requireOperationOK(t, "disable updates", runSystemTask(t, app, "disable-updates"))
	if enabled, running := updateStatus(); enabled || running {
		t.Errorf("after disabling: enabled=%v running=%v", enabled, running)
	}
	tablet.run("grep -qx AutoUpdate=false /home/root/.config/remarkable/xochitl.conf")

	enable := runSystemTask(t, app, "enable-updates")
	requireOperationOK(t, "enable updates", enable)
	if enabled, running := updateStatus(); !enabled || !running {
		t.Errorf("after enabling: enabled=%v running=%v\n%s", enabled, running, enable.output)
	}
	tablet.run("grep -qx AutoUpdate=true /home/root/.config/remarkable/xochitl.conf")
}

func TestSystemAutomaticUpdatesSkipWritableRootFrom328(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{osVersion: "3.28.0.1"})
	app := connectApp(t, tablet)
	for _, task := range app.GetSystemTasksInfo() {
		if task.NeedsWritableRoot {
			t.Errorf("%s needs the writable root on 3.28", task.ID)
		}
	}
	run := runSystemTask(t, app, "disable-updates")
	requireOperationOK(t, "disable updates", run)
	if strings.Contains(run.output, "remount") {
		t.Errorf("the task remounted the root filesystem on 3.28:\n%s", run.output)
	}
}

func TestSystemSleepScreenSetAndReset(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)
	tablet.run(`printf '[General]\nAutoUpdate=false\n' > /home/root/.config/remarkable/xochitl.conf`)

	if supported, err := app.IsSleepScreenSupported(); err != nil || !supported {
		t.Fatalf("IsSleepScreenSupported: %v %v", supported, err)
	}

	picture := image.NewGray(image.Rect(0, 0, 16, 16))
	picture.Set(8, 8, color.White)
	local := filepath.Join(t.TempDir(), "sleep.png")
	file, err := os.Create(local)
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(file, picture)
	file.Close()
	if out, err := exec.Command("docker", "cp", local, tablet.container+":/home/root/sleep.png").CombinedOutput(); err != nil {
		t.Fatalf("docker cp: %v: %s", err, out)
	}

	if _, err := runQuietOp(t, app, time.Minute, func(id string) { app.SetSleepScreen(id, "/home/root/sleep.png") }); err != nil {
		t.Fatalf("SetSleepScreen: %v", err)
	}
	if path, err := app.GetSleepScreen(); err != nil || path != "/home/root/sleep.png" {
		t.Errorf("GetSleepScreen after setting: %q %v", path, err)
	}
	conf := tablet.run("cat /home/root/.config/remarkable/xochitl.conf")
	if !strings.Contains(conf, "SleepScreenPath") || !strings.Contains(conf, "AutoUpdate") {
		t.Errorf("xochitl.conf after setting:\n%s", conf)
	}
	if files := strings.Fields(tablet.run("ls /home/root/.config/remarkable")); !slices.Equal(files, []string{"xochitl.conf"}) {
		t.Errorf("config directory holds %v", files)
	}

	before := xochitlPID(tablet)
	if _, err := runQuietOp(t, app, time.Minute, func(id string) { app.RestartXochitl(id) }); err != nil {
		t.Fatalf("RestartXochitl: %v", err)
	}
	if after := xochitlPID(tablet); after == "" || after == before {
		t.Errorf("RestartXochitl left xochitl at pid %q, was %q", after, before)
	}

	if _, err := runQuietOp(t, app, time.Minute, func(id string) { app.ResetSleepScreen(id) }); err != nil {
		t.Fatalf("ResetSleepScreen: %v", err)
	}
	if path, err := app.GetSleepScreen(); err != nil || path != "" {
		t.Errorf("GetSleepScreen after reset: %q %v", path, err)
	}
	conf = tablet.run("cat /home/root/.config/remarkable/xochitl.conf")
	if strings.Contains(conf, "SleepScreenPath") || !strings.Contains(conf, "AutoUpdate") {
		t.Errorf("xochitl.conf after reset:\n%s", conf)
	}
}

func TestSystemKeepAwakePulsesThePen(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	penHasEvents := "test -s /dev/input/event1"

	// Keep-awake is on by default, so connecting starts it.
	app := connectApp(t, tablet)
	restoreSettingsAfter(t, app)
	setKeepAwake := func(on bool) {
		t.Helper()
		if err := app.UpdateSettings(SettingsPatch{PreventSleep: &on}); err != nil {
			t.Fatalf("UpdateSettings(PreventSleep: %v): %v", on, err)
		}
	}
	waitOnTablet(t, tablet, penHasEvents, 30*time.Second)

	tablet.run(": > /dev/input/event1")
	setKeepAwake(false)
	if !succeedsOnTablet(tablet, penHasEvents) {
		t.Error("turning keep-awake off wrote no pen-leave events")
	}

	tablet.run(": > /dev/input/event1")
	time.Sleep(22 * time.Second)
	if succeedsOnTablet(tablet, penHasEvents) {
		t.Error("the pen was still pulsed after keep-awake was turned off")
	}

	setKeepAwake(true)
	waitOnTablet(t, tablet, penHasEvents, 30*time.Second)
	setKeepAwake(false)
}

func TestSystemAdvancedLauncherCommands(t *testing.T) {
	requireDocker(t)
	tablet, app := launcherTablet(t)

	status := runMaintenanceCommand(t, app, "launcherctl", "launcher-status", nil)
	requireOperationOK(t, "launcherctl status", status)
	if !strings.Contains(status.output, "Launcher: none") {
		t.Errorf("status output:\n%s", status.output)
	}

	run := runMaintenanceCommand(t, app, "launcherctl", "choose-launcher", func(req DialogRequest) string {
		return actionLabeled(req, "oxide")
	})
	requireOperationOK(t, "switch launcher", run)
	if len(run.prompts) != 1 || actionLabeled(run.prompts[0], "appload") == "" || actionLabeled(run.prompts[0], "oxide") == "" {
		t.Fatalf("switch prompt: %+v", run.prompts)
	}
	if !strings.Contains(run.output, "switch-launcher oxide --start") {
		t.Errorf("the switch's output is missing from the operation:\n%s", run.output)
	}
	if xochitl := currentXochitlStatus(t, app); xochitl.CurrentLauncher != "oxide" || xochitl.ActiveLauncher != "oxide" || xochitl.Running {
		t.Errorf("after switching to oxide: %+v", xochitl)
	}
	tablet.run("test -e /run/fake-systemd/tarnish.active")
}

func TestSystemDebugStopsWhenCanceled(t *testing.T) {
	requireDocker(t)
	tablet, app := modsTablet(t)

	debugRunning := `pgrep -f '[x]ovi/debug' && tr '\0' '\n' < /proc/$(pidof -s xochitl)/environ | grep -q xovi.so`
	run := runOp(t, app, time.Minute, nil, func(id string) {
		app.RunMaintenanceCommand(id, "xovi-extensions", "debug", systemTestDevice)
		waitOnTablet(t, tablet, debugRunning, 30*time.Second)
		app.CancelOperation(id)
	})
	if run.result.OK || !run.result.Canceled {
		t.Fatalf("debug after Stop: %+v", run.result)
	}
	if succeedsOnTablet(tablet, "pgrep -f '[x]ovi/debug' || pidof xochitl") {
		t.Errorf("debug is still running on the tablet: %s", tablet.run("ps -o pid,args"))
	}

	requireOperationOK(t, "Start with mods after Debug", runMaintenanceCommand(t, app, "xovi-extensions", "start", nil))
	if xochitl := currentXochitlStatus(t, app); !xochitl.XoviActive {
		t.Errorf("after Start with mods: %+v", xochitl)
	}
}

func TestSystemFailedCommandReportsWhyAndNextRuns(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)
	tablet.run("rm -f /home/root/xovi/stock")

	failed := runMaintenanceCommand(t, app, "xovi-extensions", "stock", nil)
	if failed.result.OK || failed.result.Canceled || failed.result.Error == "" {
		t.Fatalf("a missing script finished as %+v", failed.result)
	}
	if !strings.Contains(failed.output, "No such file or directory") || !strings.HasSuffix(strings.TrimSpace(failed.output), sentence(failed.result.Error)) {
		t.Errorf("the output does not end with why:\n%s", failed.output)
	}

	requireOperationOK(t, "restart after a failure", runSystemTask(t, app, "restart-xochitl"))
}

// On a Paper Pro the timezone runs between a remount of the root filesystem read-write and one back to read-only; a failure in between must still put the root back, or it stays writable until the next reboot.
func TestSystemFailedTimezoneRestoresReadOnlyRoot(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	failed := runOp(t, app, time.Minute, nil, func(id string) { app.SetDeviceTimezone(id, "Mars/Olympus_Mons", systemTestDevice) })
	if failed.result.OK || !strings.Contains(failed.output, "Invalid or not installed time zone") || !strings.Contains(failed.output, "Could not set the timezone") {
		t.Fatalf("an unknown timezone finished as %+v\n%s", failed.result, failed.output)
	}
	if !strings.Contains(failed.output, "Root remounted as ro") && !strings.Contains(tablet.commands(), "remount,ro") {
		t.Errorf("the root filesystem was left read-write after the failure:\n%s", failed.output)
	}

	requireOperationOK(t, "set timezone after a failure", runOp(t, app, time.Minute, nil, func(id string) {
		app.SetDeviceTimezone(id, "Europe/Berlin", systemTestDevice)
	}))
}

func TestSystemRebootReconnects(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	from := emitted.mark()
	if _, err := runQuietOp(t, app, time.Minute, func(id string) { app.RebootDevice(id) }); err != nil {
		t.Fatalf("RebootDevice: %v", err)
	}
	tablet.waitUntilBooted(2)
	if _, err := os.Stat(filepath.Join(tablet.dir, "rebooted")); err != nil {
		t.Fatal("the tablet never ran systemctl reboot")
	}
	_, next := emitted.waitFor(t, from, time.Minute, "connection:lost")
	emitted.waitFor(t, next, 2*time.Minute, "connection:restored")
	if count := emitted.count(from, "connection:failed"); count != 0 {
		t.Errorf("%d connection:failed events during the reboot", count)
	}

	if !app.isConnected() {
		t.Fatal("not connected after connection:restored")
	}
	requireOperationOK(t, "restart after the reboot", runSystemTask(t, app, "restart-xochitl"))
}

// The fake is back up a second after it reboots, before the app notices the drop; a real tablet takes far longer, so this one is held down while the app's first reconnect attempts are refused.
func TestSystemRebootReconnectsAfterSlowBoot(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app := connectApp(t, tablet)

	from := emitted.mark()
	if _, err := runQuietOp(t, app, time.Minute, func(id string) { app.RebootDevice(id) }); err != nil {
		t.Fatalf("RebootDevice: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for _, err := os.Stat(filepath.Join(tablet.dir, "rebooted")); err != nil; _, err = os.Stat(filepath.Join(tablet.dir, "rebooted")) {
		if time.Now().After(deadline) {
			t.Fatal("the tablet never ran systemctl reboot")
		}
		time.Sleep(100 * time.Millisecond)
	}
	tablet.docker("stop")
	_, next := emitted.waitFor(t, from, time.Minute, "connection:lost")
	emitted.waitFor(t, next, 30*time.Second, "connection:reconnecting")
	time.Sleep(10 * time.Second)
	tablet.docker("start")

	event, _ := emitted.waitFor(t, next, 2*time.Minute, "connection:restored", "connection:failed")
	if event.Name != "connection:restored" {
		t.Fatalf("reconnect gave up while the tablet booted: %v; events: %s", event.Data, eventNames(emitted.since(from)))
	}
	requireOperationOK(t, "restart after the reboot", runSystemTask(t, app, "restart-xochitl"))
}

func TestSystemUserGuideInstallsAndRestartsWithMods(t *testing.T) {
	requireDocker(t)
	var manifest map[string]struct {
		SHA256 string `json:"sha256"`
	}
	if err := fetchJSON(guideManifestURL, &manifest); err != nil {
		t.Fatalf("guide manifest: %v", err)
	}
	var guideVersion string
	for candidate := range manifest {
		if guideVersion == "" || rmversion.Compare(candidate, guideVersion) > 0 {
			guideVersion = candidate
		}
	}
	buildVersion := version
	version = guideVersion
	t.Cleanup(func() { version = buildVersion })

	tablet, app := modsTablet(t)
	requireOperationOK(t, "Start with mods", runMaintenanceCommand(t, app, "xovi-extensions", "start", nil))

	if status := app.checkUserGuide(); status.Skipped || status.Installed {
		t.Fatalf("guide before installing: %+v", status)
	}
	if _, err := runQuietOp(t, app, 2*time.Minute, func(id string) { app.InstallUserGuide(id) }); err != nil {
		t.Fatalf("InstallUserGuide: %v", err)
	}
	status := app.checkUserGuide()
	if !status.Installed || status.NeedsUpdate {
		t.Fatalf("guide after installing: %+v", status)
	}
	tablet.run("test -s /home/root/.local/share/remarkable/xochitl/" + status.DocID + ".epub")

	before := xochitlPID(tablet)
	requireOperationOK(t, "restart for the guide", runMaintenanceCommand(t, app, "xovi-extensions", "start", nil))
	if after := xochitlPID(tablet); after == "" || after == before {
		t.Errorf("the restart finished with xochitl at pid %q, was %q", after, before)
	}
	if xochitl := currentXochitlStatus(t, app); !xochitl.XoviActive {
		t.Errorf("mods are not running after the guide restart: %+v", xochitl)
	}
}

func newerOSVersion(t *testing.T, app *testApp) string {
	t.Helper()
	versions, err := app.GetAvailableOSVersions()
	if err != nil || len(versions) == 0 {
		t.Fatalf("GetAvailableOSVersions: %v %v", versions, err)
	}
	for _, candidate := range versions {
		if strings.HasPrefix(candidate.Version, "3.28.") {
			return candidate.Version
		}
	}
	t.Fatalf("the update server offers no 3.28: %+v", versions)
	return ""
}

func TestOSCheckReadsUpdateServerAndVellum(t *testing.T) {
	requireDocker(t)
	_, app := modsTablet(t, "battery-percentage")

	versions, err := app.GetAvailableOSVersions()
	if err != nil || len(versions) == 0 {
		t.Fatalf("GetAvailableOSVersions: %v %v", versions, err)
	}
	channel := app.osChannelStatus()
	if channel.ActiveVersion != currentOSVersion || channel.LatestPublic != versions[0].Version {
		t.Errorf("osChannelStatus: %+v (newest listed %s)", channel, versions[0].Version)
	}

	target := newerOSVersion(t, app)
	compat, err := app.CheckOSCompatibility(target)
	if err != nil || compat.FetchFailed {
		t.Fatalf("CheckOSCompatibility(%s) failed to fetch: %v", target, err)
	}
	if !slices.Contains(compat.Incompatible, "battery-percentage") {
		t.Errorf("battery-percentage supports only 3.27, yet %s reports %+v", target, compat)
	}
	judged := slices.Concat(compat.Compatible, compat.Incompatible, compat.NoConstraint)
	if !slices.Contains(judged, "qt-resource-rebuilder") {
		t.Errorf("qt-resource-rebuilder is not judged for %s: %+v", target, compat)
	}

	if ready := app.CheckOSManagerReady(); !ready.Ready {
		t.Errorf("CheckOSManagerReady: %+v", ready)
	}
}

func TestOSChangeAfterUpgradeSyncsMods(t *testing.T) {
	requireDocker(t)
	tablet, app := modsTablet(t, "battery-percentage")
	target := newerOSVersion(t, app)
	tablet.run("/fake/write-hashtab /home/root/xovi/exthome/qt-resource-rebuilder/hashtab")
	tablet.run("rm -f /etc/vellum/reenabled && sed -i 's/=" + currentOSVersion + "$/=" + target + "/' /usr/share/remarkable/update.conf")

	from := emitted.mark()
	if result := app.Connect(tablet.addr, tablet.password); !result.Success {
		t.Fatalf("reconnect: %s", result.Message)
	}
	event, _ := emitted.waitFor(t, from, 2*time.Minute, "connect:warnings")
	warnings := event.Data.(ConnectWarnings)
	if mismatch := warnings.OSMismatch; mismatch == nil || mismatch.PrevVersion != currentOSVersion || mismatch.NewVersion != target {
		t.Errorf("osMismatch: %+v", warnings.OSMismatch)
	}
	if compat := warnings.CompatibilityStatus; compat == nil || !slices.Contains(compat.IncompatiblePackages, "battery-percentage") {
		t.Errorf("compatibilityStatus: %+v", warnings.CompatibilityStatus)
	}
	if hashtab := warnings.Hashtab; hashtab == nil || !hashtab.NeedsRebuild {
		t.Errorf("hashtab after the OS change: %+v", warnings.Hashtab)
	}
	if warnings.ReenableStatus != vellum.ReenableNeeded {
		t.Errorf("reenableStatus after the OS change: %v", warnings.ReenableStatus)
	}

	status := app.GetPackageCompatibilityStatus()
	if status.FetchFailed || status.CurrentOsVersion != target || status.StoredOsVersion != currentOSVersion || !slices.Contains(status.IncompatiblePackages, "battery-percentage") {
		t.Errorf("GetPackageCompatibilityStatus: %+v", status)
	}

	blocked := runOp(t, app, 3*time.Minute, nil, func(id string) { app.RunUpgrade(id) })
	if blocked.result.OK || blocked.result.Upgrade == nil || blocked.result.Upgrade.Blocked == nil ||
		!slices.Contains(blocked.result.Upgrade.Blocked.Incompatible, "battery-percentage") {
		t.Fatalf("an upgrade with an incompatible mod finished as %+v\n%s", blocked.result, blocked.output)
	}

	requireOperationOK(t, "remove the incompatible mod", runOp(t, app, 3*time.Minute, declinePrompts, func(id string) {
		app.UninstallPackages(id, []string{"battery-percentage"}, systemTestDevice)
	}))
	requireOperationOK(t, "upgrade", runOp(t, app, 5*time.Minute, declinePrompts, func(id string) { app.RunUpgrade(id) }))
	if state := app.getOSVersionState(); state.Mismatch || state.StoredVersion != target {
		t.Errorf("OS state after the upgrade: %+v", state)
	}

	reenable := runOp(t, app, 2*time.Minute, nil, func(id string) { app.RunReenable(id) })
	requireOperationOK(t, "re-enable", reenable)
	if reenable.result.Reenable != vellum.ReenableOK {
		t.Errorf("re-enable reported %q", reenable.result.Reenable)
	}
}
