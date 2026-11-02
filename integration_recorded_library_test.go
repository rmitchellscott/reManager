//go:build integration

package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reManager/internal/pdfimport"
	"reManager/internal/support"
	"reManager/internal/vellum"
)

func TestRecordedRescanWithoutTheLibrarian(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	if _, ok := modsInstalledVersions(t, app)["librarian"]; ok {
		t.Fatal("this test needs a tablet without the librarian mod")
	}
	// A removed extension stays loaded until xochitl restarts.
	recordedRestartUI(t, app, device, true)
	_, err := runQuietOp(t, app, time.Minute, func(id string) { app.RescanLibrary(id) })
	if err == nil || !strings.Contains(err.Error(), "librarian mod did not answer") {
		t.Errorf("RescanLibrary without the librarian = %v, want the librarian named", err)
	}
}

func TestRecordedImportsWithTheLibrarian(t *testing.T) {
	tablet, app, device := connectRecordedWithCatalog(t)
	startWithMods := modsAnswerHashtable("Start with mods")
	recordedInstall(t, app, device, "librarian", startWithMods)
	t.Cleanup(func() {
		recordedUninstall(t, app, device, "librarian", startWithMods)
		tablet.run(vellum.VellumBin + " del xovi-message-broker")
		recordedRestartUI(t, app, device, true)
	})
	// The import follows the start at once, before xochitl has loaded its library, which the rescan has to wait out.
	recordedRestartUI(t, app, device, true)

	path := filepath.Join(t.TempDir(), "Rescanned.pdf")
	writeLocalFile(t, path, minimalPDF(1))
	importErr := importDocuments(t, app, []ImportDocument{{Path: path, VisibleName: recordedImportName + " rescanned"}}, true, false)
	found := strings.Fields(tablet.run("grep -l '" + recordedImportName + "' " + pdfimport.XochitlPath + "/*.metadata || true"))
	t.Cleanup(func() {
		for _, metadata := range found {
			base := strings.TrimSuffix(metadata, ".metadata")
			tablet.run("rm -rf " + base + " " + base + ".*")
		}
		runQuietOp(t, app, time.Minute, func(id string) { app.RescanLibrary(id) })
	})
	if importErr != nil {
		t.Fatalf("ImportDocuments with a rescan: %v", importErr)
	}
	if len(found) != 1 {
		t.Fatalf("want one imported document named %q, found %v", recordedImportName, found)
	}
}

func TestRecordedImportEpubAndRmdoc(t *testing.T) {
	tablet, app, device := connectRecordedWithCatalog(t)
	modsBefore := recordedModsRunning(t, app)
	local := t.TempDir()
	epub := filepath.Join(local, "Book.epub")
	rmdoc := filepath.Join(local, "Notebook.rmdoc")
	writeLocalFile(t, epub, minimalEPUB(t, "Recorded Book"))
	writeLocalFile(t, rmdoc, zipArchive(t, rmdocEntries("")))
	for path, fileType := range map[string]string{epub: "epub", rmdoc: "rmdoc"} {
		if info, err := app.GetImportFileInfo(path); err != nil || info.FileType != fileType {
			t.Fatalf("GetImportFileInfo(%s) = %+v, %v", filepath.Base(path), info, err)
		}
	}
	documents := []ImportDocument{
		{Path: epub, VisibleName: recordedImportName + " book"},
		{Path: rmdoc, VisibleName: recordedImportName + " notebook"},
	}

	importErr := importDocuments(t, app, documents, false, true)
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
	if len(found) != 2 {
		t.Fatalf("want two imported documents named %q, found %v", recordedImportName, found)
	}
	if status, err := app.GetXochitlStatus(); err != nil || !status.Running {
		t.Errorf("after the import restart, xochitl = %+v, %v", status, err)
	}
}

func TestRecordedDownload(t *testing.T) {
	tablet, app, _ := connectRecordedWithCatalog(t)
	remote := "/home/root/remanager-recorded-downloads"
	tablet.run("rm -rf " + remote + " && mkdir -p " + remote + "/Folder/sub && cd " + remote +
		" && seq 1 4000 > numbers.txt && echo first > Folder/a.txt && echo second > 'Folder/sub/b c.txt'")
	t.Cleanup(func() { tablet.run("rm -rf " + remote) })

	local := t.TempDir()
	manager := app.transferManager()
	manager.enqueue(TransferDownload, "numbers.txt", filepath.Join(local, "numbers.txt"), remote+"/numbers.txt", false)
	manager.enqueue(TransferDownload, "Folder", local, remote+"/Folder", true)
	snapshot := waitForTransfersToSettle(t, app, 2*time.Minute)
	for _, name := range []string{"numbers.txt", "Folder"} {
		if view := transferNamed(t, snapshot, name); view.State != TransferDone || len(view.FailedFiles) > 0 {
			t.Errorf("%s: %s %s, failed %v", name, view.State, view.Error, view.FailedFiles)
		}
	}
	for local, remote := range map[string]string{
		filepath.Join(local, "numbers.txt"):              remote + "/numbers.txt",
		filepath.Join(local, "Folder", "a.txt"):          remote + "/Folder/a.txt",
		filepath.Join(local, "Folder", "sub", "b c.txt"): remote + "/Folder/sub/b c.txt",
	} {
		want, _, _ := strings.Cut(tablet.run("sha256sum '"+remote+"'"), " ")
		if got := localFileHash(t, local); got != want {
			t.Errorf("%s downloaded with sha256 %s, want %s", local, got, want)
		}
	}
}

func TestRecordedSleepScreenSetAndRestore(t *testing.T) {
	tablet, app, device := connectRecordedWithCatalog(t)
	before, err := app.GetSleepScreen()
	if err != nil {
		t.Fatal(err)
	}
	modsBefore := recordedModsRunning(t, app)

	picture := image.NewGray(image.Rect(0, 0, 16, 16))
	picture.Set(8, 8, color.White)
	local := filepath.Join(t.TempDir(), "remanager-recorded-sleep.png")
	file, err := os.Create(local)
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(file, picture)
	file.Close()
	remote := "/home/root/remanager-recorded-sleep.png"
	app.UploadFilesFromPaths([]string{local}, "/home/root")
	waitForTransfersToSettle(t, app, time.Minute)
	t.Cleanup(func() { tablet.run("rm -f " + remote) })

	if _, err := runQuietOp(t, app, time.Minute, func(id string) { app.SetSleepScreen(id, remote) }); err != nil {
		t.Fatalf("SetSleepScreen: %v", err)
	}
	if path, err := app.GetSleepScreen(); err != nil || path != remote {
		t.Errorf("GetSleepScreen after setting = %q, %v", path, err)
	}
	recordedRestartUI(t, app, device, modsBefore)

	restore := func(id string) { app.SetSleepScreen(id, before) }
	if before == "" {
		restore = func(id string) { app.ResetSleepScreen(id) }
	}
	if _, err := runQuietOp(t, app, time.Minute, restore); err != nil {
		t.Fatalf("restore the sleep screen to %q: %v", before, err)
	}
	if path, err := app.GetSleepScreen(); err != nil || path != before {
		t.Errorf("GetSleepScreen after restoring = %q, %v; want %q", path, err, before)
	}
	recordedRestartUI(t, app, device, modsBefore)
}

// The replay fails if the pen lookup or the pen-leave write recorded here is never sent.
func TestRecordedKeepAwakeStartsAndStops(t *testing.T) {
	_, app, _ := connectRecordedWithCatalog(t)
	for _, on := range []bool{true, false} {
		if err := app.UpdateSettings(SettingsPatch{PreventSleep: &on}); err != nil {
			t.Fatalf("UpdateSettings(PreventSleep: %v): %v", on, err)
		}
	}
}

func TestRecordedSupportBundle(t *testing.T) {
	_, app, _ := connectRecordedWithCatalog(t)
	_, _, request := uploadSupportBundle(t, app)
	files := uploadedBundle(t, request)
	if _, ok := files["support-bundle/device/device_info.json"]; !ok {
		t.Error("the uploaded bundle has no device information from the tablet")
	}
	var info support.BundleInfo
	if err := json.Unmarshal(files["support-bundle/BUNDLE_INFO.json"], &info); err != nil || len(info.Errors) > 0 {
		t.Errorf("BUNDLE_INFO = %+v, %v; want no collection errors", info, err)
	}
}

func TestRecordedReenable(t *testing.T) {
	_, app, _ := connectRecordedWithCatalog(t)
	run := runOp(t, app, 5*time.Minute, nil, func(id string) { app.RunReenable(id) })
	if !run.result.OK {
		t.Fatalf("RunReenable: %+v; output:\n%s", run.result, run.output)
	}
	if run.result.Reenable == "" {
		t.Errorf("RunReenable reported no status: %+v", run.result)
	}
}

// xochitl.service's start limit reboots a Paper Pro after five starts inside 10 minutes.
func TestRecordedRestartUIFiveTimesInARow(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	modsBefore := recordedModsRunning(t, app)
	from := emitted.mark()
	for i := range 5 {
		if _, err := runQuietOp(t, app, 2*time.Minute, func(id string) { app.RestartXochitl(id) }); err != nil {
			t.Fatalf("restart %d: %v", i+1, err)
		}
		recordedSettle()
	}
	if lost := emitted.count(from, eventConnectionLost); lost != 0 {
		t.Fatalf("the tablet dropped the connection %d times during five restarts", lost)
	}
	if status, err := app.GetXochitlStatus(); err != nil || !status.Running || status.XoviActive != modsBefore {
		t.Errorf("after five restarts, xochitl = %+v, %v; want running with mods %v as before", status, err, modsBefore)
	}
	if !modsBefore {
		recordedRestartUI(t, app, device, true)
	}
}
