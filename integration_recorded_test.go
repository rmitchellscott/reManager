//go:build integration

package main

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"reManager/internal/tablettest"
)

// TestRecorded* tests replay what a real tablet answered; re-record with REMANAGER_RECORD_HOST set to a tablet reachable as `ssh <host>`.
const recordHostEnv = "REMANAGER_RECORD_HOST"

// Installed once, before any app starts, because an app's background catalog fetch reads http.DefaultTransport and may outlive the test that started it.
var recordedHTTP = func() *switchableTransport {
	transport := &switchableTransport{base: http.DefaultTransport}
	http.DefaultTransport = transport
	return transport
}()

type switchableTransport struct {
	base    http.RoundTripper
	current atomic.Pointer[http.RoundTripper]
}

func (s *switchableTransport) use(transport http.RoundTripper) {
	if transport == nil {
		s.current.Store(nil)
		return
	}
	s.current.Store(&transport)
}

func (s *switchableTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if current := s.current.Load(); current != nil {
		return (*current).RoundTrip(req)
	}
	return s.base.RoundTrip(req)
}

type recordedTablet struct {
	t        *testing.T
	addr     string
	password string
}

func startRecordedTablet(t *testing.T) *recordedTablet {
	t.Helper()
	host := os.Getenv(recordHostEnv)
	cassettePath := filepath.Join("testdata", "tablet", t.Name()+".json")

	recordedHTTP.use(tablettest.NewHTTPCassette(filepath.Join("testdata", "tablet", "http"), host != "", recordedHTTP.base, func(url string) {
		t.Errorf("no recorded response for %s; record again with %s set", url, recordHostEnv)
	}))
	t.Cleanup(func() { recordedHTTP.use(nil) })

	var server *tablettest.Server
	if host != "" {
		cassette := &tablettest.Cassette{}
		var err error
		server, err = tablettest.NewRecordServer([]string{host}, cassette)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			server.Close()
			if t.Failed() {
				return
			}
			if err := cassette.Save(cassettePath); err != nil {
				t.Errorf("save %s: %v", cassettePath, err)
			}
		})
	} else {
		cassette, err := tablettest.LoadCassette(cassettePath)
		if errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("no recording at %s; record one with %s set to a tablet's ssh host", cassettePath, recordHostEnv)
		}
		if err != nil {
			t.Fatal(err)
		}
		server, err = tablettest.NewReplayServer(cassette, func(command string) {
			t.Errorf("no recorded answer for %q; record again with %s set", command, recordHostEnv)
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			server.Close()
			if t.Failed() {
				return
			}
			for _, unasked := range server.Unasked() {
				t.Errorf("recorded %q was never asked for; the app skipped a step, or record again with %s set", unasked, recordHostEnv)
			}
		})
	}
	return &recordedTablet{t: t, addr: server.Addr(), password: tablettest.Password}
}

// run goes through the same server as the app, so its answer is recorded and replayed like any other.
func (r *recordedTablet) run(command string) string {
	r.t.Helper()
	client, err := ssh.Dial("tcp", r.addr, &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password(r.password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		r.t.Fatal(err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		r.t.Fatal(err)
	}
	defer session.Close()
	out, err := session.CombinedOutput(command)
	if err != nil {
		r.t.Fatalf("on tablet %q: %v: %s", command, err, out)
	}
	return string(out)
}

type recordedDevice struct {
	deviceType string
	firmware   string
	arch       string
}

func connectRecordedWithCatalog(t *testing.T) (*recordedTablet, *testApp, recordedDevice) {
	t.Helper()
	tablet, app, device := connectTablet(t, startRecordedTablet(t))
	// Proxy mode downloads through httputil's streaming client, which the HTTP cassette cannot reach, so the tablet fetches packages itself.
	modsProxyMode(t, app, false)
	return tablet, app, device
}

// Tablet-only tests need the real tablet and live downloads, so they run while recording and skip in a replay; nothing they do is saved.
func connectTabletOnly(t *testing.T) (*recordedTablet, *testApp, recordedDevice) {
	t.Helper()
	host := os.Getenv(recordHostEnv)
	if host == "" {
		t.Skipf("needs a real tablet: set %s", recordHostEnv)
	}
	server, err := tablettest.NewRecordServer([]string{host}, &tablettest.Cassette{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	return connectTablet(t, &recordedTablet{t: t, addr: server.Addr(), password: tablettest.Password})
}

func connectTablet(t *testing.T, tablet *recordedTablet) (*recordedTablet, *testApp, recordedDevice) {
	t.Helper()
	app := newTestApp(t)
	// Keep-awake pulses the pen on a 20 s timer, so the pulse count depends on how long a test runs, which a replay cannot match.
	restoreSettingsAfter(t, app)
	keepAwake := false
	if err := app.UpdateSettings(SettingsPatch{PreventSleep: &keepAwake}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	from := emitted.mark()
	result := app.Connect(tablet.addr, tablet.password)
	if !result.Success {
		t.Fatalf("Connect: %s (%s)", result.Message, result.Code)
	}
	// The connect checks keep running after Connect returns; waiting for them keeps their commands inside the test in a recording and its replay.
	emitted.waitFor(t, from, time.Minute, eventConnectWarnings)
	waitForCatalog(t, app)
	device := recordedDevice{deviceType: result.Device, arch: app.GetDeviceArchitecture(result.Device)}
	app.mu.Lock()
	device.firmware = app.connectedFirmware
	app.mu.Unlock()
	return tablet, app, device
}

// xochitl can segfault on SIGTERM mid-render and reboot a Paper Pro, so a recording waits out its multi-second render after every start before any restart; replay has no tablet to protect.
func recordedSettle() {
	if os.Getenv(recordHostEnv) != "" {
		time.Sleep(20 * time.Second)
	}
}

func recordedModsRunning(t *testing.T, app *testApp) bool {
	t.Helper()
	status, err := app.GetXochitlStatus()
	if err != nil {
		t.Fatalf("GetXochitlStatus: %v", err)
	}
	return status.XoviActive
}

// Restart UI alone brings xochitl back without xovi, so a restart that should keep mods goes through xovi's start script, as Start with mods does.
func recordedRestartUI(t *testing.T, app *testApp, device recordedDevice, withMods bool) {
	t.Helper()
	start := func(id string) { app.RestartXochitl(id) }
	if withMods {
		start = func(id string) { app.RunMaintenanceCommand(id, "xovi-extensions", "start", device.deviceType) }
	}
	if _, err := runQuietOp(t, app, 2*time.Minute, start); err != nil {
		t.Fatalf("restart the UI (with mods %v): %v", withMods, err)
	}
	recordedSettle()
	if running := recordedModsRunning(t, app); running != withMods {
		t.Fatalf("after restarting the UI with mods %v, mods running = %v", withMods, running)
	}
}

func recordedCatalogPick(t *testing.T, app *testApp, device recordedDevice, match func(PackageInfo) bool) string {
	t.Helper()
	installed := modsInstalledVersions(t, app)
	var names []string
	for _, pkg := range app.GetPackages(device.deviceType, device.firmware, device.arch) {
		if _, ok := installed[pkg.Name]; !ok && pkg.Compatible && !pkg.Hidden && match(pkg) {
			names = append(names, pkg.Name)
		}
	}
	if len(names) == 0 {
		t.Fatal("no compatible, uninstalled package in the catalog fits this test")
	}
	slices.Sort(names)
	return names[0]
}

func recordedInstall(t *testing.T, app *testApp, device recordedDevice, mod string, answer promptAnswerer) operationRun {
	t.Helper()
	sim, err := app.SimulateInstall([]string{mod}, nil, device.deviceType)
	if err != nil || sim.Error != "" {
		t.Fatalf("SimulateInstall(%s) = %+v, %v", mod, sim, err)
	}
	if !slices.Contains(sim.Packages, mod) {
		t.Fatalf("the plan for %s leaves it out: %v", mod, sim.Packages)
	}
	run := runOp(t, app, 5*time.Minute, answer, func(id string) {
		app.InstallPackages(id, []string{mod}, sim.Packages, device.deviceType)
	})
	if !run.result.OK || run.result.Install == nil || !run.result.Install.Success {
		t.Fatalf("install of %s: %+v; output:\n%s", mod, run.result, run.output)
	}
	if _, ok := modsInstalledVersions(t, app)[mod]; !ok {
		t.Fatalf("%s missing from the installed list after its install", mod)
	}
	return run
}

func recordedUninstall(t *testing.T, app *testApp, device recordedDevice, mod string, answer promptAnswerer) operationRun {
	t.Helper()
	sim, err := app.SimulateUninstall([]string{mod}, nil, device.deviceType)
	if err != nil || !slices.Contains(sim.Packages, mod) {
		t.Fatalf("SimulateUninstall(%s) = %+v, %v", mod, sim, err)
	}
	run := runOp(t, app, 5*time.Minute, answer, func(id string) {
		app.UninstallPackages(id, sim.Packages, device.deviceType)
	})
	if !run.result.OK {
		t.Fatalf("removal of %s: %+v; output:\n%s", mod, run.result, run.output)
	}
	if _, ok := modsInstalledVersions(t, app)[mod]; ok {
		t.Fatalf("%s still installed after its removal", mod)
	}
	return run
}

func TestRecordedConnect(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	if !app.isConnected() || app.inDemoMode() {
		t.Fatalf("connected = %v, demo = %v; want a live, real connection", app.isConnected(), app.inDemoMode())
	}
	if device.deviceType == "" || device.firmware == "" || device.arch == "" {
		t.Errorf("device facts incomplete: %+v", device)
	}
	info := app.GetDeviceInfo()
	for _, key := range []string{"machine", "firmware"} {
		if info[key] == "" {
			t.Errorf("GetDeviceInfo()[%q] is empty: %v", key, info)
		}
	}
	installed := modsInstalledVersions(t, app)
	if len(installed) == 0 {
		t.Error("no installed packages read from a tablet that has Vellum")
	}
	status, err := app.GetXochitlStatus()
	if err != nil || !status.Running {
		t.Errorf("GetXochitlStatus = %+v, %v; want the UI running", status, err)
	}
	if _, err := app.CheckHashtabVersion(); err != nil {
		t.Errorf("CheckHashtabVersion: %v", err)
	}
	if _, err := app.GetUpdateServiceStatus(); err != nil {
		t.Errorf("GetUpdateServiceStatus: %v", err)
	}
	if _, err := app.GetUpgradeCandidates(); err != nil {
		t.Errorf("GetUpgradeCandidates: %v", err)
	}
	if !slices.ContainsFunc(app.GetPackages(device.deviceType, device.firmware, device.arch), func(p PackageInfo) bool { return p.Compatible }) {
		t.Error("the catalog lists nothing compatible with the tablet")
	}
}

func TestRecordedSmallModInstallAndRemove(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	mod := recordedCatalogPick(t, app, device, modsIsSmallMod)

	install := recordedInstall(t, app, device, mod, nil)
	if log := modsProgressLog(modsProgress(install.result.ID)); !strings.Contains(log, mod) {
		t.Errorf("the install log never names %s:\n%s", mod, log)
	}
	recordedUninstall(t, app, device, mod, nil)
}

// qt-resource-rebuilder is already on the tablet, so the install builds no hashtable; its post-install hook asks to build one, driven here as a user would after an OS update.
func TestRecordedHashtableModInstallRebuildAndRemove(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	mod := recordedCatalogPick(t, app, device, modsNeedsOnlyTheHashtable)

	install := recordedInstall(t, app, device, mod, nil)
	if len(install.prompts) != 0 {
		t.Errorf("installing %s with qt-resource-rebuilder present prompted %q", mod, modsPromptTitles(install.prompts))
	}

	from := emitted.mark()
	rebuild := runOp(t, app, 5*time.Minute, func(request DialogRequest) string {
		if request.Title == "Rebuild hashtable" {
			return "confirm"
		}
		return actionLabeled(request, "Start with mods")
	}, func(id string) {
		app.RunMaintenanceCommand(id, "qt-resource-rebuilder", "rebuild_hashtable", device.deviceType)
	})
	recordedSettle()
	if !rebuild.result.OK {
		t.Fatalf("rebuild: %+v; output:\n%s", rebuild.result, rebuild.output)
	}
	if titles := modsPromptTitles(rebuild.prompts); !slices.Equal(titles, []string{"Rebuild hashtable", "Hashtable rebuilt"}) {
		t.Errorf("rebuild prompts = %q, want the rebuild and its result", titles)
	}
	var hooks []OperationHook
	for _, event := range emitted.since(from) {
		if hook, ok := event.Data.(OperationHook); ok && hook.ID == rebuild.result.ID {
			hooks = append(hooks, hook)
		}
	}
	if len(hooks) != 2 || hooks[0].Finished || !hooks[1].Finished || hooks[1].Error != "" {
		t.Errorf("want a hook start and a clean finish, got %+v", hooks)
	}

	status, err := app.GetXochitlStatus()
	if err != nil || !status.Running || !status.XoviActive {
		t.Errorf("after Start with mods, xochitl = %+v, %v; want running with xovi active", status, err)
	}
	if hashtab, err := app.CheckHashtabVersion(); err != nil || !hashtab.Installed || hashtab.NeedsRebuild {
		t.Errorf("after the rebuild, hashtab = %+v, %v; want present and current", hashtab, err)
	}

	recordedUninstall(t, app, device, mod, nil)
}

func TestRecordedUnknownPackageFailsToResolve(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	sim, err := app.SimulateInstall([]string{"remanager-no-such-package"}, nil, device.deviceType)
	if err == nil && sim.Error == "" {
		t.Fatalf("SimulateInstall of a package that does not exist = %+v, want an error", sim)
	}
}
