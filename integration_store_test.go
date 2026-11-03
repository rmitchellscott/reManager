//go:build integration

package main

import (
	"slices"
	"strings"
	"testing"

	"reManager/internal/vellum"
)

func TestStoreFavoritesPersist(t *testing.T) {
	app := newTestApp(t)
	waitForCatalog(t, app)
	name := storeAnyPackage(t, app).Name
	if slices.Contains(app.GetFavorites(), name) {
		t.Fatalf("%s is starred before the test starred it", name)
	}

	starred, err := app.ToggleFavorite(name)
	if err != nil || !starred {
		t.Fatalf("ToggleFavorite(%s) = %v, %v; want starred", name, starred, err)
	}
	t.Cleanup(func() {
		if slices.Contains(app.GetFavorites(), name) {
			app.ToggleFavorite(name)
		}
	})
	if !slices.Contains(newTestApp(t).GetFavorites(), name) {
		t.Errorf("a new App does not see the star on %s", name)
	}

	starred, err = app.ToggleFavorite(name)
	if err != nil || starred {
		t.Fatalf("second ToggleFavorite(%s) = %v, %v; want unstarred", name, starred, err)
	}
	if slices.Contains(newTestApp(t).GetFavorites(), name) {
		t.Errorf("a new App still sees the removed star on %s", name)
	}
}

func TestStoreFollowsPersist(t *testing.T) {
	app := newTestApp(t)
	waitForCatalog(t, app)
	pkg := storeAnyPackage(t, app)
	developer := pkg.UpstreamAuthor
	if storeFollow(app, developer) != nil {
		t.Fatalf("%s is followed before the test followed them", developer)
	}

	followed, err := app.ToggleFollow(developer)
	if err != nil || !followed {
		t.Fatalf("ToggleFollow(%s) = %v, %v; want followed", developer, followed, err)
	}
	t.Cleanup(func() {
		if storeFollow(app, developer) != nil {
			app.ToggleFollow(developer)
		}
	})
	if err := app.SetPackageSeen(developer, pkg.Name, true); err != nil {
		t.Fatalf("SetPackageSeen: %v", err)
	}

	follow := storeFollow(newTestApp(t), developer)
	if follow == nil {
		t.Fatalf("a new App does not see the follow of %s", developer)
	}
	if follow.FollowedAt == "" {
		t.Error("the follow carries no FollowedAt for the feed to baseline against")
	}
	if !slices.Contains(follow.Seen, pkg.Name) {
		t.Errorf("the follow's seen list %v leaves out %s", follow.Seen, pkg.Name)
	}

	followed, err = app.ToggleFollow(developer)
	if err != nil || followed {
		t.Fatalf("second ToggleFollow(%s) = %v, %v; want unfollowed", developer, followed, err)
	}
	if storeFollow(newTestApp(t), developer) != nil {
		t.Errorf("a new App still sees the removed follow of %s", developer)
	}
}

func TestStoreRefreshPackages(t *testing.T) {
	app := newTestApp(t)
	waitForCatalog(t, app)
	from := emitted.mark()
	if err := app.RefreshMetadata(); err != nil {
		t.Fatalf("RefreshMetadata: %v", err)
	}
	if app.CatalogLoadFailed() {
		t.Error("the catalog reports a failed load after a refresh")
	}
	if len(app.GetPackages(paperPro, currentOSVersion, "aarch64")) == 0 {
		t.Error("no packages after a refresh")
	}
	if n := emitted.count(from, "metadata:error"); n != 0 {
		t.Errorf("the refresh emitted %d metadata:error", n)
	}
}

func TestStorePackagesForPaperProOnCurrentOS(t *testing.T) {
	requireDocker(t)
	_, app := vellumTablet(t)
	packages := app.GetPackages(paperPro, currentOSVersion, "aarch64")
	if len(packages) == 0 {
		t.Fatal("GetPackages returned nothing")
	}

	compatible := map[string]bool{}
	for _, pkg := range packages {
		if pkg.Name == "vellum" && !pkg.Hidden {
			t.Errorf("vellum is not marked hidden")
		}
		if !pkg.Compatible {
			if pkg.IncompatibleReason == "" {
				t.Errorf("%s is incompatible with no reason", pkg.Name)
			}
			continue
		}
		compatible[pkg.Name] = true
		if len(pkg.Devices) > 0 && !slices.Contains(pkg.Devices, paperPro) {
			t.Errorf("%s is offered as compatible but lists devices %v", pkg.Name, pkg.Devices)
		}
		for _, dep := range pkg.Depends {
			if strings.HasPrefix(dep, "vellum") || dep == "/bin/sh" {
				t.Errorf("%s shows the framework dependency %s", pkg.Name, dep)
			}
		}
		releases := app.GetPackageReleases(pkg.Name)
		i := slices.IndexFunc(releases, func(r vellum.PackageRelease) bool { return r.Version == pkg.Version })
		if i < 0 || !releases[i].Compatible {
			t.Errorf("%s is offered at %s, which is not a release the tablet can install", pkg.Name, pkg.Version)
		}
	}
	if len(compatible) == 0 {
		t.Fatal("nothing in the catalog is compatible with a Paper Pro on the current OS")
	}

	olderOS := 0
	for _, pkg := range app.GetPackages(paperPro, "3.0.0.0", "aarch64") {
		if compatible[pkg.Name] && !pkg.Compatible && pkg.IncompatibleReason == "os" {
			olderOS++
		}
	}
	if olderOS == 0 {
		t.Error("no package compatible on the current OS is ruled out for OS 3.0, so the firmware filter does nothing")
	}

	otherDevice := 0
	for _, pkg := range app.GetPackages("rm1", "3.20.0.0", "armv7") {
		if compatible[pkg.Name] && !pkg.Compatible && pkg.IncompatibleReason == "device" {
			otherDevice++
		}
	}
	if otherDevice == 0 {
		t.Error("no package compatible with a Paper Pro is ruled out for a reMarkable 1, so the device filter does nothing")
	}
}

func TestStorePackageDetail(t *testing.T) {
	requireDocker(t)
	_, app := vellumTablet(t)
	var withHistory PackageInfo
	for _, pkg := range app.GetPackages(paperPro, currentOSVersion, "aarch64") {
		if pkg.Compatible && pkg.ReadmeURL != nil && len(app.GetPackageReleases(pkg.Name)) > 1 && pkg.Name > withHistory.Name {
			withHistory = pkg
		}
	}
	if withHistory.Name == "" {
		t.Fatal("no compatible package has a readme and more than one release")
	}

	releases := app.GetPackageReleases(withHistory.Name)
	if releases[0].Version != withHistory.Version && releases[0].Compatible {
		t.Errorf("the newest release of %s is %s and compatible, but the catalog offers %s", withHistory.Name, releases[0].Version, withHistory.Version)
	}
	for i, release := range releases {
		if release.Released == "" {
			t.Errorf("%s %s has no release date", withHistory.Name, release.Version)
		}
		if i > 0 && release.Version == releases[i-1].Version {
			t.Errorf("%s lists %s twice", withHistory.Name, release.Version)
		}
	}

	readme, err := app.GetReadmeContent(*withHistory.ReadmeURL)
	if err != nil || strings.TrimSpace(readme) == "" {
		t.Errorf("GetReadmeContent(%s) = %d bytes, %v", *withHistory.ReadmeURL, len(readme), err)
	}
	if _, err := app.GetReadmeContent("file:///etc/passwd"); err == nil {
		t.Error("GetReadmeContent read a file: URL")
	}
	if releases := app.GetPackageReleases("no-such-package-in-the-catalog"); releases != nil {
		t.Errorf("an unknown package has releases %v", releases)
	}
}

func storeAnyPackage(t *testing.T, app *testApp) PackageInfo {
	t.Helper()
	var chosen PackageInfo
	for _, pkg := range app.GetPackages(paperPro, currentOSVersion, "aarch64") {
		if pkg.Compatible && !pkg.Hidden && pkg.UpstreamAuthor != "" && pkg.Name > chosen.Name {
			chosen = pkg
		}
	}
	if chosen.Name == "" {
		t.Fatal("no compatible package in the catalog names its developer")
	}
	return chosen
}

func storeFollow(app *testApp, developer string) *FollowInfo {
	for _, follow := range app.GetFollows() {
		if follow.Name == developer {
			return &follow
		}
	}
	return nil
}
