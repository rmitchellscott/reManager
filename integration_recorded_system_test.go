//go:build integration

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"reManager/internal/pdfimport"
)

func TestRecordedSystemStatus(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)

	timezone, err := app.GetTimezoneStatus()
	if err != nil || timezone.DeviceTimezone == "" {
		t.Errorf("GetTimezoneStatus = %+v, %v", timezone, err)
	}
	if supported, err := app.IsSleepScreenSupported(); err != nil || !supported {
		t.Errorf("IsSleepScreenSupported = %v, %v", supported, err)
	}
	if _, err := app.GetSleepScreen(); err != nil {
		t.Errorf("GetSleepScreen: %v", err)
	}
	tasks := app.GetSystemTasksInfo()
	if !slices.ContainsFunc(tasks, func(task SystemTaskInfo) bool { return strings.HasSuffix(task.ID, "-updates") }) {
		t.Errorf("GetSystemTasksInfo offers no update toggle: %+v", tasks)
	}

	versions, err := app.GetAvailableOSVersions()
	if err != nil || len(versions) == 0 {
		t.Fatalf("GetAvailableOSVersions = %v, %v", versions, err)
	}
	if channel := app.osChannelStatus(); channel.ActiveVersion != device.firmware || channel.LatestPublic == "" {
		t.Errorf("osChannelStatus = %+v, want active %s and a latest public version", channel, device.firmware)
	}
	var target string
	for _, version := range versions {
		if version.Version != device.firmware {
			target = version.Version
			break
		}
	}
	if target == "" {
		t.Fatalf("no OS version other than %s is listed: %+v", device.firmware, versions)
	}
	compat, err := app.CheckOSCompatibility(target)
	if err != nil || compat.FetchFailed {
		t.Fatalf("CheckOSCompatibility(%s) = %+v, %v", target, compat, err)
	}
	if judged := slices.Concat(compat.Compatible, compat.Incompatible, compat.NoConstraint); !slices.Contains(judged, "qt-resource-rebuilder") {
		t.Errorf("qt-resource-rebuilder is not judged for %s: %+v", target, compat)
	}
	if ready := app.CheckOSManagerReady(); !ready.Ready {
		t.Errorf("CheckOSManagerReady = %+v", ready)
	}
}

func TestRecordedTimezoneRoundTrip(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	before, err := app.GetTimezoneStatus()
	if err != nil || before.DeviceTimezone == "" {
		t.Fatalf("GetTimezoneStatus = %+v, %v", before, err)
	}
	other := "America/Denver"
	if before.DeviceTimezone == other {
		other = "Europe/Paris"
	}

	for _, timezone := range []string{other, before.DeviceTimezone} {
		run := runOp(t, app, time.Minute, nil, func(id string) { app.SetDeviceTimezone(id, timezone, device.deviceType) })
		if !run.result.OK {
			t.Fatalf("SetDeviceTimezone(%s) = %+v; output:\n%s", timezone, run.result, run.output)
		}
		if status, err := app.GetTimezoneStatus(); err != nil || status.DeviceTimezone != timezone {
			t.Errorf("after setting %s, GetTimezoneStatus = %+v, %v", timezone, status, err)
		}
	}
}

func TestRecordedAutomaticUpdatesRoundTrip(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	before, err := app.GetUpdateServiceStatus()
	if err != nil {
		t.Fatal(err)
	}
	order := []string{"disable-updates", "enable-updates"}
	if !before.Enabled {
		order = []string{"enable-updates", "disable-updates"}
	}
	for _, task := range order {
		run := runOp(t, app, time.Minute, nil, func(id string) { app.RunSystemTask(id, task, device.deviceType) })
		if !run.result.OK {
			t.Fatalf("%s: %+v; output:\n%s", task, run.result, run.output)
		}
		status, err := app.GetUpdateServiceStatus()
		if want := task == "enable-updates"; err != nil || status.Enabled != want {
			t.Errorf("after %s, GetUpdateServiceStatus = %+v, %v; want enabled=%v", task, status, err, want)
		}
	}
}

const recordedImportName = "reManager recorded import"

// Rescanning needs the librarian mod, so the import restarts the UI instead.
func TestRecordedImportPDF(t *testing.T) {
	tablet, app, device := connectRecordedWithCatalog(t)
	modsBefore := recordedModsRunning(t, app)
	path := filepath.Join(t.TempDir(), "Recorded.pdf")
	if err := os.WriteFile(path, minimalPDF(2), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := app.GetImportFileInfo(path)
	if err != nil || info.FileType != "pdf" || info.PageCount != 2 {
		t.Fatalf("GetImportFileInfo = %+v, %v", info, err)
	}

	importErr := importDocuments(t, app, []ImportDocument{{Path: path, VisibleName: recordedImportName}}, false, true)
	recordedSettle()
	found := strings.Fields(tablet.run("grep -l '" + recordedImportName + "' " + pdfimport.XochitlPath + "/*.metadata || true"))
	t.Cleanup(func() {
		for _, metadata := range found {
			base := strings.TrimSuffix(metadata, ".metadata")
			tablet.run("rm -rf " + base + " " + base + ".*")
		}
		recordedRestartUI(t, app, device, modsBefore)
	})
	if importErr != nil {
		t.Fatalf("ImportDocuments: %v", importErr)
	}
	if len(found) != 1 {
		t.Fatalf("want one imported document named %q, found %v", recordedImportName, found)
	}
	base := strings.TrimSuffix(found[0], ".metadata")
	for _, suffix := range []string{".pdf", ".content", ".pagedata"} {
		tablet.run("test -s " + base + suffix)
	}
	if status, err := app.GetXochitlStatus(); err != nil || !status.Running || status.XoviActive != modsBefore {
		t.Errorf("after the import restart, xochitl = %+v, %v; want running with mods %v as before", status, err, modsBefore)
	}
}
