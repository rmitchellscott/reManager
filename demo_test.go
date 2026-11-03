package main

import "testing"

// The seeded list comes from the live catalog at connect time, so the tests use a fixed stand-in rather than depending on what the index holds today.
func testDemoPackages() map[string]string {
	return map[string]string{
		"xovi":         "0.2.4",
		"appload":      "1.4.0",
		"koreader":     "2026.07.1",
		"vellum-tools": "0.9.2",
	}
}

func TestIsDemoCredential(t *testing.T) {
	cases := []struct {
		name           string
		host, authType string
		secret         string
		want           bool
	}{
		{"demo pair", "demo", "password", "demo", true},
		{"screenshot pair", "demo", "password", "screenshots", true},
		{"OS update pair", "demo", "password", "os-update", true},
		{"real host", "10.11.99.1", "password", "demo", false},
		{"real password", "demo", "password", "hunter2", false},
		{"key auth cannot reach demo", "demo", "key", "demo", false},
		{"agent auth cannot reach demo", "demo", "agent", "demo", false},
		{"empty", "", "password", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isDemoCredential(c.host, c.authType, c.secret); got != c.want {
				t.Errorf("isDemoCredential(%q, %q, %q) = %v, want %v", c.host, c.authType, c.secret, got, c.want)
			}
		})
	}
}

// The transport is nil here, so a demo guard that fails to short-circuit would panic; these assert the fabricated values, not merely a non-empty result.
func TestDemoGuardsBypassTheTransport(t *testing.T) {
	s := newServices(&core{demo: true, demoPackages: testDemoPackages()})

	if !s.connection.isConnected() {
		t.Error("isConnected should report true in demo mode")
	}
	if !s.connection.inDemoMode() {
		t.Error("inDemoMode should report true")
	}
	if got := s.device.GetDeviceArchitecture("rmppure"); got != "aarch64" {
		t.Errorf("GetDeviceArchitecture = %q, want aarch64", got)
	}
	// The header and the saved device both take their name from machine, and store screenshots show it, so it names the model like a real tablet.
	if got := s.device.GetDeviceInfo()["machine"]; got != demoDeviceType.DisplayName() {
		t.Errorf("GetDeviceInfo machine = %q, want %q", got, demoDeviceType.DisplayName())
	}
	if installed, err := s.packages.GetInstalledPackagesWithVersions(); err != nil || len(installed) == 0 {
		t.Errorf("GetInstalledPackagesWithVersions = %v, %v, want fabricated packages", installed, err)
	}
	if got := s.packages.GetPackageConflicts(nil, nil, "rmppure", demoFirmware); len(got) != 0 {
		t.Errorf("GetPackageConflicts = %v, want empty", got)
	}
	sim, err := s.packages.SimulateInstall([]string{"not-installed-here"}, nil, "rmppure")
	if err != nil || sim == nil || len(sim.Packages) != 1 {
		t.Errorf("SimulateInstall = %v, %v", sim, err)
	}
	if sim != nil && len(sim.Requested) != 1 {
		t.Errorf("Requested = %v, want the one requested package", sim.Requested)
	}
}

func TestScreenshotDemoLooksLikeATablet(t *testing.T) {
	review := newServices(&core{demo: true, demoPackages: testDemoPackages()})
	if _, err := review.files.ListDirectory("/home/root"); err == nil {
		t.Error("ListDirectory in the review demo should refuse")
	}
	if got := review.shell.demoPrompt(); got != demoShellPrompt {
		t.Errorf("review demo prompt = %q, want %q", got, demoShellPrompt)
	}

	screenshots := newServices(&core{demo: true, demoScreenshots: true, demoPackages: testDemoPackages()})
	listing, err := screenshots.files.ListDirectory("")
	if err != nil || len(listing) != len(screenshotHomeListing) {
		t.Errorf("ListDirectory(\"\") = %d entries, %v, want the captured /home/root", len(listing), err)
	}
	if listing, err := screenshots.files.ListDirectory("/home/root/xovi"); err != nil || len(listing) != 0 {
		t.Errorf("ListDirectory(xovi) = %v, %v, want an empty folder", listing, err)
	}
	if got := screenshots.shell.demoPrompt(); got != screenshotShellPrompt {
		t.Errorf("screenshot demo prompt = %q, want %q", got, screenshotShellPrompt)
	}

	paths, ok := screenshots.files.screenshotImportPaths()
	if !ok || len(paths) != len(screenshotImportFiles) {
		t.Fatalf("screenshotImportPaths = %v, %v, want the staged documents", paths, ok)
	}
	if info, err := screenshots.files.GetImportFileInfo(paths[1]); err != nil || info.PageCount != 6 {
		t.Errorf("GetImportFileInfo(%s) = %+v, %v, want the canned PDF", paths[1], info, err)
	}
	if _, ok := review.files.screenshotImportPaths(); ok {
		t.Error("the review demo should open the real file picker")
	}
}

// Step mode reads the versions and the installed list from this warning alone.
func TestDemoOSUpdateNamesTheStandbyVersion(t *testing.T) {
	connection := newServices(&core{demo: true, demoPackages: testDemoPackages()}).connection
	change, status := connection.demoOSUpdate()
	if change.PrevVersion != demoFallbackFirmware || change.NewVersion != demoFirmware {
		t.Errorf("change = %+v, want %s to %s", change, demoFallbackFirmware, demoFirmware)
	}
	if len(status.InstalledPackages) != len(testDemoPackages()) {
		t.Errorf("InstalledPackages = %v, want the seeded packages", status.InstalledPackages)
	}
}

func TestDemoInstallChangesReportedState(t *testing.T) {
	connection := newServices(&core{demo: true, demoPackages: testDemoPackages()}).connection

	if _, present := connection.demoInstalled()["nothing-yet"]; present {
		t.Fatal("package should not be installed before the plan runs")
	}

	connection.mu.Lock()
	connection.demoPackages["nothing-yet"] = "1.0.0"
	connection.mu.Unlock()

	if got := connection.demoInstalled()["nothing-yet"]; got != "1.0.0" {
		t.Errorf("installed version = %q, want 1.0.0", got)
	}
}

func TestDemoInstalledIsACopy(t *testing.T) {
	connection := newServices(&core{demo: true, demoPackages: testDemoPackages()}).connection
	connection.demoInstalled()["xovi"] = "tampered"
	if got := connection.demoInstalled()["xovi"]; got != "0.2.4" {
		t.Errorf("xovi = %q, want 0.2.4: the map escaped", got)
	}
}

func TestDemoResolveSkipsInstalled(t *testing.T) {
	packages := newServices(&core{demo: true, demoPackages: testDemoPackages()}).packages
	got := packages.demoResolve([]string{"xovi", "brand-new"})
	if len(got) != 1 || got[0] != "brand-new" {
		t.Errorf("demoResolve = %v, want [brand-new]", got)
	}
}

func TestStopDemoClearsState(t *testing.T) {
	connection := newServices(&core{demo: true, connectedDeviceID: demoMachineID, demoPackages: testDemoPackages()}).connection
	connection.stopDemo()
	if connection.inDemoMode() {
		t.Error("inDemoMode should report false after stopDemo")
	}
	if connection.connectedDeviceID != "" {
		t.Errorf("connectedDeviceID = %q, want empty", connection.connectedDeviceID)
	}
	if connection.demoPackages != nil {
		t.Error("demoPackages should be cleared so a later demo starts fresh")
	}
}

func TestDemoDeviceSurfacesAnswerWithoutTransport(t *testing.T) {
	s := newServices(&core{demo: true, demoPackages: testDemoPackages(), demoAutoUpdates: true, demoTimezone: demoDefaultTimezone})

	status, err := s.device.GetXochitlStatus()
	if err != nil || !status.Running || !status.XoviActive {
		t.Errorf("GetXochitlStatus = %+v, %v, want running with xovi active", status, err)
	}
	info, err := s.os.GetPartitionInfo()
	if err != nil || info.Active.Version != demoFirmware {
		t.Errorf("GetPartitionInfo = %+v, %v, want active slot on %s", info, err, demoFirmware)
	}
	if got, err := s.device.GetTimezoneStatus(); err != nil || got.DeviceTimezone != demoDefaultTimezone {
		t.Errorf("GetTimezoneStatus = %+v, %v, want device timezone %q", got, err, demoDefaultTimezone)
	}
	if got, err := s.packages.CheckOSCompatibility(demoFirmware); err != nil || got.FetchFailed || len(got.Compatible) != len(testDemoPackages()) {
		t.Errorf("CheckOSCompatibility = %+v, %v, want every installed package compatible", got, err)
	}
	if candidates, err := s.packages.GetUpgradeCandidates(); err != nil || len(candidates) != 0 {
		t.Errorf("GetUpgradeCandidates = %v, %v, want none without a catalog", candidates, err)
	}
}

func TestDemoUpdateSwitchReadsBackWhatWasSet(t *testing.T) {
	s := newServices(&core{demo: true, demoAutoUpdates: true})
	s.packages.recordDemoSystemTask("disable-updates")
	if status, err := s.device.GetUpdateServiceStatus(); err != nil || status.Enabled || status.Running {
		t.Errorf("GetUpdateServiceStatus after disable = %+v, %v, want off", status, err)
	}
	s.packages.recordDemoSystemTask("enable-updates")
	if status, err := s.device.GetUpdateServiceStatus(); err != nil || !status.Enabled {
		t.Errorf("GetUpdateServiceStatus after enable = %+v, %v, want on", status, err)
	}
}

func TestDemoHashtableIsCurrent(t *testing.T) {
	device := newServices(&core{demo: true}).device
	if status, err := device.CheckHashtabVersion(); err != nil || !status.Installed || status.NeedsRebuild {
		t.Errorf("CheckHashtabVersion = %+v, %v, want built for the running firmware", status, err)
	}
}

func TestDemoRefusesDeviceWrites(t *testing.T) {
	files := newServices(&core{demo: true}).files
	if _, err := files.ListDirectory("/home/root"); err != errDemoUnavailable {
		t.Errorf("ListDirectory error = %v, want the demo refusal", err)
	}
	for name, start := range map[string]func(osService *OSService, id string){
		"SwitchBootPartition": func(osService *OSService, id string) { osService.SwitchBootPartition(id, 3) },
		"RebootDevice":        func(osService *OSService, id string) { osService.RebootDevice(id) },
	} {
		c, r := coreWithRecorder()
		c.demo = true
		start(newServices(c).os, "op-1")
		if result := r.waitDone(t); result.OK || result.Error != errDemoUnavailable.Error() {
			t.Errorf("%s result = %+v, want the demo refusal", name, result)
		}
	}
}

func TestDemoDependencyNameDropsVersionConstraint(t *testing.T) {
	for dep, want := range map[string]string{"xovi": "xovi", "xovi>=0.2": "xovi", "qt-resource-rebuilder=1.0-r0": "qt-resource-rebuilder"} {
		if got := demoDependencyName(dep); got != want {
			t.Errorf("demoDependencyName(%q) = %q, want %q", dep, got, want)
		}
	}
}

func TestStaleDemoSessionIsNotCurrent(t *testing.T) {
	connection := newServices(&core{demo: true, demoPackages: testDemoPackages()}).connection
	session := connection.currentDemoSession()
	connection.stopDemo()
	if connection.demoSessionCurrent(session) {
		t.Error("a session from before stopDemo should no longer be current")
	}
}

func TestMalformedPackageNamesAndOSVersionsAreRejected(t *testing.T) {
	packages := newServices(&core{demo: true, demoPackages: testDemoPackages()}).packages

	for _, name := range []string{"x; touch /tmp/marker", "$(id)", "-rf", "name with space", ""} {
		if sim, err := packages.SimulateInstall([]string{"xovi", name}, nil, "rmppure"); err == nil {
			t.Errorf("SimulateInstall(%q) = %+v, want an error", name, sim)
		}
		if sim, err := packages.SimulateInstall([]string{"xovi"}, []string{name}, "rmppure"); err == nil {
			t.Errorf("SimulateInstall with removal %q = %+v, want an error", name, sim)
		}
	}
	for _, version := range []string{"3.28; touch /tmp/marker", "latest", "3.20.", ""} {
		if got, err := packages.CheckOSCompatibility(version); err == nil {
			t.Errorf("CheckOSCompatibility(%q) = %+v, want an error", version, got)
		}
	}
}
