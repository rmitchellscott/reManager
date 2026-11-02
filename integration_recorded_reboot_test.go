//go:build integration

package main

import (
	"testing"
	"time"
)

// A tablet takes about a minute to boot, which the app's reconnect has to outlast; replay compresses the outage to a few seconds.
const recordedRebootTimeout = 5 * time.Minute

func recordedReboot(t *testing.T, app *testApp) {
	t.Helper()
	from := emitted.mark()
	if _, err := runQuietOp(t, app, time.Minute, func(id string) { app.RebootDevice(id) }); err != nil {
		t.Fatalf("RebootDevice: %v", err)
	}
	_, next := emitted.waitFor(t, from, time.Minute, eventConnectionLost)
	event, _ := emitted.waitFor(t, next, recordedRebootTimeout, eventConnectionRestored, eventConnectionFailed)
	if event.Name != eventConnectionRestored {
		t.Fatalf("reconnect after the reboot ended with %s: %+v; events: %s", event.Name, event.Data, eventNames(emitted.since(from)))
	}
	emitted.waitFor(t, next, time.Minute, eventConnectWarnings)
	recordedSettle()
	if !app.isConnected() {
		t.Fatal("not connected after connection:restored")
	}
}

// The tablet comes back up stock, as every reboot leaves it, so mods are started again for the tests that follow.
func TestRecordedRebootReconnects(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	recordedReboot(t, app)
	if recordedModsRunning(t, app) {
		t.Error("mods are running right after a reboot, which xovi never does on its own")
	}
	if _, err := app.GetInstalledPackagesWithVersions(); err != nil {
		t.Errorf("the restored connection cannot run commands: %v", err)
	}
	recordedRestartUI(t, app, device, true)
}

func recordedSwitchAndReboot(t *testing.T, app *testApp, partition int, wantVersion string) {
	t.Helper()
	if _, err := runQuietOp(t, app, 2*time.Minute, func(id string) { app.SwitchBootPartition(id, partition) }); err != nil {
		t.Fatalf("SwitchBootPartition(%d): %v", partition, err)
	}
	if info, err := app.GetPartitionInfo(); err != nil || (info.Active.Number == partition) == info.Fallback.IsNextBoot {
		t.Fatalf("after switching to %d, partitions = %+v, %v", partition, info, err)
	}
	recordedReboot(t, app)
	info, err := app.GetPartitionInfo()
	if err != nil || info.Active.Number != partition || info.Active.Version != wantVersion {
		t.Fatalf("after rebooting into %d, partitions = %+v, %v; want %s active", partition, info, err, wantVersion)
	}
	if firmware := app.GetDeviceInfo()["firmware"]; firmware != wantVersion {
		t.Errorf("after rebooting into %d, the app reports firmware %q, want %s", partition, firmware, wantVersion)
	}
}

func TestRecordedPartitionSwitchAndBack(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	before, err := app.GetPartitionInfo()
	if err != nil || before.Fallback.Version == "" || before.UpdatePending {
		t.Fatalf("GetPartitionInfo = %+v, %v; want an idle fallback with an OS on it", before, err)
	}

	recordedSwitchAndReboot(t, app, before.Fallback.Number, before.Fallback.Version)
	recordedSwitchAndReboot(t, app, before.Active.Number, before.Active.Version)
	recordedRestartUI(t, app, device, true)
}
