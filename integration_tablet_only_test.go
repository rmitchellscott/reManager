//go:build integration

package main

import (
	"maps"
	"slices"
	"testing"
	"time"

	"reManager/internal/swupdate"
)

func TestTabletVellumResetAndReinstall(t *testing.T) {
	_, app, device := connectTabletOnly(t)
	before := modsInstalledVersions(t, app)
	// The bootstrap installs these itself, and the OS and model packages are virtual ones Vellum keeps in step with the tablet.
	reinstall := slices.Sorted(maps.Keys(before))
	reinstall = slices.DeleteFunc(reinstall, func(name string) bool {
		return slices.Contains([]string{"vellum", "mount-utils", "vellum-bash-completion", "remarkable-os", "rmppmove", "rmpp", "rm1", "rm2", "rmppure"}, name)
	})

	if run := runOp(t, app, 5*time.Minute, nil, func(id string) { app.UninstallVellum(id, true) }); !run.result.OK {
		t.Fatalf("UninstallVellum(--all): %+v; output:\n%s", run.result, run.output)
	}
	if vellumInstalled(t, app) {
		t.Fatal("Vellum still installed after the uninstall")
	}

	if run := runOp(t, app, 10*time.Minute, nil, func(id string) { app.BootstrapVellum(id) }); !run.result.OK {
		t.Fatalf("BootstrapVellum: %+v; output:\n%s", run.result, run.output)
	}
	if !vellumInstalled(t, app) {
		t.Fatal("Vellum not installed after the bootstrap")
	}
	waitForCatalog(t, app)

	sim, err := app.SimulateInstall(reinstall, nil, device.deviceType)
	if err != nil || sim.Error != "" || len(sim.UnresolvedVirtuals) != 0 {
		t.Fatalf("SimulateInstall(%v) = %+v, %v", reinstall, sim, err)
	}
	run := runOp(t, app, 10*time.Minute, modsAnswerHashtable("Start with mods"), func(id string) {
		app.InstallPackages(id, reinstall, sim.Packages, device.deviceType)
	})
	if !run.result.OK || run.result.Install == nil || !run.result.Install.Success {
		t.Fatalf("reinstall of %v: %+v; output:\n%s", reinstall, run.result, run.output)
	}
	recordedSettle()
	after := modsInstalledVersions(t, app)
	if !slices.Equal(slices.Sorted(maps.Keys(after)), slices.Sorted(maps.Keys(before))) {
		t.Errorf("installed after the reset = %v, before = %v", slices.Sorted(maps.Keys(after)), slices.Sorted(maps.Keys(before)))
	}
	if !recordedModsRunning(t, app) {
		recordedRestartUI(t, app, device, true)
	}
}

// The version already on the other slot is written to it again, so the slots end as they began.
func TestTabletOSInstallRebootAndBack(t *testing.T) {
	_, app, device := connectTabletOnly(t)
	before, err := app.GetPartitionInfo()
	if err != nil || before.Fallback.Version == "" || before.UpdatePending {
		t.Fatalf("GetPartitionInfo = %+v, %v; want an idle fallback with an OS on it", before, err)
	}
	versions, err := app.GetAvailableOSVersions()
	if err != nil {
		t.Fatal(err)
	}
	target := before.Fallback.Version
	if !slices.ContainsFunc(versions, func(v swupdate.OSVersionInfo) bool { return v.Version == target }) {
		t.Skipf("%s, on the other slot, is not offered for this tablet", target)
	}

	run := runOp(t, app, 45*time.Minute, nil, func(id string) { app.InstallOSVersion(id, target) })
	if !run.result.OK {
		t.Fatalf("InstallOSVersion(%s): %+v; output:\n%s", target, run.result, run.output)
	}
	if info, err := app.GetPartitionInfo(); err != nil || !info.Fallback.IsNextBoot || info.Fallback.Version != target {
		t.Fatalf("after the install, partitions = %+v, %v; want %s on the other slot as the next boot", info, err, target)
	}
	recordedReboot(t, app)
	if info, err := app.GetPartitionInfo(); err != nil || info.Active.Number != before.Fallback.Number || info.Active.Version != target {
		t.Fatalf("after rebooting into the install, partitions = %+v, %v", info, err)
	}

	recordedSwitchAndReboot(t, app, before.Active.Number, before.Active.Version)
	recordedRestartUI(t, app, device, true)
}
