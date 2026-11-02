//go:build integration

package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"reManager/internal/vellum"
)

func TestRecordedConflictingModsDoNotResolve(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	installed := modsInstalledVersions(t, app)
	compatible := map[string]bool{}
	packages := app.GetPackages(device.deviceType, device.firmware, device.arch)
	for _, pkg := range packages {
		if _, ok := installed[pkg.Name]; !ok && pkg.Compatible && !pkg.Hidden {
			compatible[pkg.Name] = true
		}
	}
	var pair []string
	for _, pkg := range packages {
		if !compatible[pkg.Name] {
			continue
		}
		for _, other := range pkg.Conflicts {
			if compatible[other] && (pair == nil || slices.Compare([]string{pkg.Name, other}, pair) < 0) {
				pair = []string{pkg.Name, other}
			}
		}
	}
	if pair == nil {
		t.Skip("no two compatible, uninstalled packages in the catalog conflict")
	}

	sim, err := app.SimulateInstall(pair, nil, device.deviceType)
	if err == nil && sim.Error == "" && len(sim.Conflicts) == 0 {
		t.Fatalf("SimulateInstall(%v) of two conflicting packages = %+v, want a conflict", pair, sim)
	}
}

func TestRecordedInstalledLauncherSatisfiesAnApp(t *testing.T) {
	_, app, device := connectRecordedWithCatalog(t)
	launcherApp := recordedCatalogPick(t, app, device, func(pkg PackageInfo) bool {
		return slices.Equal(pkg.Depends, []string{"launcher"})
	})
	if choices := app.GetInstallChoices([]string{launcherApp}, device.deviceType); len(choices) != 0 {
		t.Errorf("with a launcher installed, %s still asks for one: %+v", launcherApp, choices)
	}
	sim, err := app.SimulateInstall([]string{launcherApp}, nil, device.deviceType)
	if err != nil || sim.Error != "" || len(sim.UnresolvedVirtuals) != 0 {
		t.Fatalf("SimulateInstall(%s) = %+v, %v", launcherApp, sim, err)
	}
	if !slices.Equal(sim.Packages, []string{launcherApp}) {
		t.Errorf("SimulateInstall(%s) = %v, want only %s", launcherApp, sim.Packages, launcherApp)
	}
}

func TestRecordedUpgradeFromAnOlderRelease(t *testing.T) {
	tablet, app, device := connectRecordedWithCatalog(t)
	installed := modsInstalledVersions(t, app)
	var name, older string
	for _, pkg := range app.GetPackages(device.deviceType, device.firmware, device.arch) {
		if _, ok := installed[pkg.Name]; ok || !pkg.Compatible || pkg.Hidden || len(pkg.Depends) != 0 || len(pkg.Provides) != 0 {
			continue
		}
		var releases []string
		for _, release := range app.GetPackageReleases(pkg.Name) {
			if release.Compatible {
				releases = append(releases, release.Version)
			}
		}
		if len(releases) >= 2 && (name == "" || pkg.Name < name) {
			name, older = pkg.Name, releases[1]
		}
	}
	if name == "" {
		t.Fatal("no compatible package in the catalog has two installable releases")
	}

	// Installing a version pins it in the world file, where apk would then refuse to upgrade it, so the pin is dropped to leave a plain add.
	tablet.run(fmt.Sprintf("%s add %s=%s 2>&1 && sed -i 's/^%s=.*/%s/' %s/etc/apk/world", vellum.VellumBin, name, older, name, name, vellum.VellumRoot))
	if version := modsInstalledVersions(t, app)[name]; version != older {
		t.Fatalf("arranged %s at %s, the tablet has %q", name, older, version)
	}

	candidates, err := app.GetUpgradeCandidates()
	if err != nil {
		t.Fatal(err)
	}
	candidate, ok := candidates[name]
	if !ok || candidate.From != older || candidate.To == older || candidate.To == "" {
		t.Fatalf("upgrade candidate for %s at %s = %+v (listed: %v)", name, older, candidate, ok)
	}

	run := runOp(t, app, 5*time.Minute, nil, func(id string) { app.RunPackageUpgrade(id, []string{name}) })
	if !run.result.OK || run.result.Upgrade == nil {
		t.Fatalf("upgrade of %s: %+v; output:\n%s", name, run.result, run.output)
	}
	if !strings.Contains(run.output, "Upgrade complete.") {
		t.Errorf("the upgrade output does not finish:\n%s", run.output)
	}
	if version := modsInstalledVersions(t, app)[name]; version != candidate.To {
		t.Errorf("%s is at %s after the upgrade, want %s", name, version, candidate.To)
	}
	after, err := app.GetUpgradeCandidates()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after[name]; ok {
		t.Errorf("%s is still an upgrade candidate after its upgrade", name)
	}

	recordedUninstall(t, app, device, name, nil)
}
