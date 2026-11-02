//go:build integration

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"reManager/internal/storage"
	"reManager/internal/support"
)

func supportRequestsSince(from int) []supportRequest {
	return supportStub.received()[from:]
}

func readBundleTarball(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	decompressed, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("bundle is not gzip: %v", err)
	}
	files := map[string][]byte{}
	reader := tar.NewReader(decompressed)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return files
		}
		if err != nil {
			t.Fatalf("bundle is not a tarball: %v", err)
		}
		files[header.Name], _ = io.ReadAll(reader)
	}
}

func uploadedBundle(t *testing.T, request supportRequest) map[string][]byte {
	t.Helper()
	_, params, err := mime.ParseMediaType(request.header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("the %s %s request has no multipart content type: %v", request.method, request.path, err)
	}
	reader := multipart.NewReader(bytes.NewReader(request.body), params["boundary"])
	for {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatalf("no file part in the %s %s body: %v", request.method, request.path, err)
		}
		if part.FormName() == "file" {
			data, _ := io.ReadAll(part)
			return readBundleTarball(t, data)
		}
	}
}

func bundleRecord(app *testApp, remoteID string) (storage.BundleRecord, bool) {
	for _, record := range app.GetBundleHistory() {
		if record.RemoteID == remoteID {
			return record, true
		}
	}
	return storage.BundleRecord{}, false
}

func uploadSupportBundle(t *testing.T, app *testApp) (remoteID, url string, request supportRequest) {
	t.Helper()
	requestsFrom := len(supportStub.received())
	run := runOp(t, app, 2*time.Minute, nil, func(id string) { app.UploadSupportBundle(id, "") })
	if !run.result.OK || run.result.SupportBundle == nil {
		t.Fatalf("upload failed: %+v", run.result)
	}
	requests := supportRequestsSince(requestsFrom)
	if len(requests) != 1 || requests[0].method != "POST" || requests[0].path != "/api/bundles" {
		t.Fatalf("support service received %d requests, want one POST /api/bundles: %+v", len(requests), requests)
	}
	return run.result.SupportBundle.RemoteID, run.result.SupportBundle.URL, requests[0]
}

func TestSupportBundleFlow(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app, deviceID := connectSavedTablet(t, tablet, "IT Support Tablet")

	preview := app.GetSupportBundlePreview()
	if len(preview.Included) == 0 || !slices.Contains(preview.Excluded, "Device serial number") {
		t.Errorf("preview: %+v", preview)
	}

	// GenerateSupportBundle asks where to save in a native dialog, so the test runs the generator directly.
	saved := filepath.Join(t.TempDir(), "remanager-support.tar.gz")
	if err := app.buildGenerator(deviceID, true, "").Generate(saved, support.GenerateBundleID()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	data, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	files := readBundleTarball(t, data)
	for _, name := range []string{"support-bundle/BUNDLE_INFO.json", "support-bundle/device/device_info.json", "support-bundle/settings.json", "support-bundle/logs/remanager.log"} {
		if _, ok := files[name]; !ok {
			t.Errorf("saved bundle has no %s", name)
		}
	}
	var info support.BundleInfo
	json.Unmarshal(files["support-bundle/BUNDLE_INFO.json"], &info)
	if info.DeviceID != deviceID || len(info.Errors) > 0 {
		t.Errorf("BUNDLE_INFO: device %q, errors %v; want device %q", info.DeviceID, info.Errors, deviceID)
	}

	remoteID, url, request := uploadSupportBundle(t, app)
	if _, ok := uploadedBundle(t, request)["support-bundle/device/device_info.json"]; !ok {
		t.Error("the uploaded bundle has no device information from the connected tablet")
	}
	record, ok := bundleRecord(app, remoteID)
	if !ok || record.URL != url || record.DeviceID != deviceID || record.DeviceName != "IT Support Tablet" {
		t.Fatalf("history record %+v (found %t), want %s at %s for %s", record, ok, remoteID, url, deviceID)
	}

	requestsFrom := len(supportStub.received())
	if _, err := runQuietOp(t, app, 2*time.Minute, func(id string) { app.AppendSupportBundle(id, remoteID, deviceID) }); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	requests := supportRequestsSince(requestsFrom)
	if len(requests) != 1 || requests[0].method != "POST" || requests[0].path != "/api/bundles/"+remoteID+"/append" {
		t.Fatalf("append sent %+v", requests)
	}
	uploadedBundle(t, requests[0])
	if record, _ := bundleRecord(app, remoteID); record.LastAppended == 0 {
		t.Error("the history does not record the append")
	}

	requestsFrom = len(supportStub.received())
	if err := app.DeleteSupportBundle(remoteID); err != nil {
		t.Fatalf("DeleteSupportBundle: %v", err)
	}
	requests = supportRequestsSince(requestsFrom)
	if len(requests) != 1 || requests[0].method != "DELETE" || requests[0].path != "/api/bundles/"+remoteID {
		t.Fatalf("delete sent %+v", requests)
	}
	if _, ok := bundleRecord(app, remoteID); ok {
		t.Error("the deleted bundle is still in the history")
	}
	if err := app.DeleteSupportBundle(remoteID); err == nil {
		t.Error("deleting a bundle a second time succeeded without its credentials")
	}
}

// xochitl logs the serial and the authorId it derives from it, and a foreground command such as a hashtable rebuild captures both into its command log.
func TestSupportBundleOmitsDeviceIdentifiers(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	const (
		serial    = "RM02A7K3Q9X2M4P"
		authorID  = "3f2c9a1e-7b4d-5e6f-8a9b-0c1d2e3f4a5b"
		machineID = "5e1f1a1d2c3b4a5968778695a4b3c2d1"
	)
	tablet.run("echo " + serial + " > /sys/devices/soc0/serial_number && echo " + machineID + " > /etc/machine-id" +
		` && printf '#!/bin/sh\necho "[xochitl] Device serial: $(cat /sys/devices/soc0/serial_number)"\necho "[xochitl] Setting authorId to ` + authorID + `"\necho Done.\n' > /home/root/xovi/rebuild_hashtable`)
	app, deviceID := connectSavedTablet(t, tablet, "IT Support Tablet")

	run := runOp(t, app, time.Minute, nil, func(id string) {
		app.runOperation(id, "Rebuild hashtable", func(op *Operation) (any, error) {
			return nil, app.runOperationCommand(op, "/home/root/xovi/rebuild_hashtable", false)
		})
	})
	if !run.result.OK || !strings.Contains(run.output, serial) {
		t.Fatalf("the rebuild did not print the serial: %+v\n%s", run.result, run.output)
	}
	commandLogs, _ := filepath.Glob(filepath.Join(app.logger.GetLogDir(), deviceID, "*.log"))
	var logged bool
	for _, path := range commandLogs {
		data, _ := os.ReadFile(path)
		logged = logged || bytes.Contains(data, []byte(serial))
	}
	if !logged {
		t.Fatalf("no command log under %s holds the serial, so the bundle cannot be checked for it", deviceID)
	}

	_, _, request := uploadSupportBundle(t, app)
	var redacted bool
	for name, data := range uploadedBundle(t, request) {
		for _, identifier := range []string{serial, authorID, machineID} {
			if bytes.Contains(data, []byte(identifier)) {
				t.Errorf("%s contains %s", name, identifier)
			}
		}
		redacted = redacted || bytes.Contains(data, []byte("[serial redacted]")) && bytes.Contains(data, []byte("[author id redacted]"))
	}
	if !redacted {
		t.Error("no file in the bundle carries the redaction markers, so the command log was left out")
	}
}

func TestSupportProblemReport(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app, deviceID := connectSavedTablet(t, tablet, "IT Support Tablet")
	_, bundleURL, _ := uploadSupportBundle(t, app)

	requestsFrom := len(supportStub.received())
	if err := app.SubmitProblemReport("The import never finished.", "person@example.com", bundleURL, deviceID); err != nil {
		t.Fatalf("SubmitProblemReport: %v", err)
	}
	requests := supportRequestsSince(requestsFrom)
	if len(requests) != 1 || requests[0].method != "POST" || requests[0].path != "/api/reports" {
		t.Fatalf("report sent %+v", requests)
	}
	var report map[string]string
	if err := json.Unmarshal(requests[0].body, &report); err != nil {
		t.Fatalf("report body is not JSON: %v: %s", err, requests[0].body)
	}
	device := app.GetDeviceInfo()
	want := map[string]string{
		"description":      "The import never finished.",
		"email":            "person@example.com",
		"bundle_url":       bundleURL,
		"app_version":      version,
		"device_model":     device["machine"],
		"firmware_version": device["firmware"],
	}
	for key, value := range want {
		if report[key] != value || value == "" {
			t.Errorf("report %s = %q, want %q", key, report[key], value)
		}
	}
	if report["host_os"] == "" {
		t.Error("report has no host_os")
	}
}

func TestSupportDeleteLogs(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{})
	app, deviceID := connectSavedTablet(t, tablet, "IT Support Tablet")

	run := runOp(t, app, time.Minute, nil, func(id string) {
		app.RunSystemTask(id, "restart-xochitl", string(app.connectedDeviceType))
	})
	if !run.result.OK {
		t.Fatalf("restart-xochitl: %+v", run.result)
	}
	logDir := app.logger.GetLogDir()
	if commandLogs, _ := filepath.Glob(filepath.Join(logDir, deviceID, "*.log")); len(commandLogs) == 0 {
		t.Fatal("the task wrote no command log")
	}

	if err := app.DeleteAllLogs(); err != nil {
		t.Fatalf("DeleteAllLogs: %v", err)
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "remanager.") {
			t.Errorf("%s left in %s", entry.Name(), logDir)
		}
	}
	appLog, err := os.ReadFile(filepath.Join(logDir, "remanager.log"))
	if err != nil || !bytes.Contains(appLog, []byte("all command logs deleted by user")) {
		t.Errorf("the app log does not record the deletion: %v", err)
	}
}
