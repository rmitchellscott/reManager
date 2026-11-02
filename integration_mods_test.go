//go:build integration

package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reManager/internal/vellum"
)

const paperPro = "rmpp"

func TestVellumBootstrapVerifiesDownloads(t *testing.T) {
	requireDocker(t)
	t.Setenv("VELLUM_BOOTSTRAP_DIR", "")
	tablet := startFakeTablet(t, fakeTabletOptions{realVellum: true, osVersion: currentOSVersion})
	from := emitted.mark()
	app := connectApp(t, tablet)
	emitted.waitFor(t, from, time.Minute, "vellum:missing")
	if vellumInstalled(t, app) {
		t.Fatal("Vellum reported installed before the bootstrap")
	}

	run := runOp(t, app, 5*time.Minute, nil, func(id string) { app.BootstrapVellum(id) })
	if !run.result.OK {
		t.Fatalf("bootstrap: %+v; output:\n%s", run.result, run.output)
	}
	if run.output == "" {
		t.Error("the bootstrap streamed no output")
	}

	bootstrapRun := modsCommandsContaining(tablet.commands(), "sh bootstrap.sh")
	if len(bootstrapRun) != 1 {
		t.Fatalf("want one bootstrap.sh run, got %q", bootstrapRun)
	}
	if strings.Contains(bootstrapRun[0], "--no-verify") {
		t.Errorf("the bootstrap of downloaded files skipped verification: %s", bootstrapRun[0])
	}

	if !vellumInstalled(t, app) {
		t.Error("Vellum not installed after the bootstrap")
	}
	installed := modsInstalledPackages(t, tablet)
	for _, name := range []string{"vellum", "mount-utils", "vellum-bash-completion"} {
		if _, ok := installed[name]; !ok {
			t.Errorf("%s not installed after the bootstrap; installed: %v", name, installed)
		}
	}

	from = emitted.mark()
	connectApp(t, tablet)
	emitted.waitFor(t, from, time.Minute, "vellum:ready")
	if n := emitted.count(from, "vellum:missing") + emitted.count(from, "vellum:broken-install"); n != 0 {
		t.Errorf("a reconnect after the bootstrap still found Vellum missing or broken: %s", eventNames(emitted.since(from)))
	}
}

func TestVellumBrokenInstallCleanup(t *testing.T) {
	requireDocker(t)
	tablet, _ := vellumTablet(t)
	tablet.run(`db=/home/root/.vellum/lib/apk/db/installed; awk 'BEGIN{RS=""; ORS="\n\n"} !/(^|\n)P:vellum\n/' $db > $db.new && mv $db.new $db`)

	from := emitted.mark()
	app := connectApp(t, tablet)
	event, _ := emitted.waitFor(t, from, time.Minute, "vellum:broken-install")
	missing, _ := event.Data.([]string)
	if !slices.Equal(missing, []string{"vellum"}) {
		t.Errorf("vellum:broken-install named %v, want [vellum]", event.Data)
	}

	if run := runOp(t, app, time.Minute, nil, func(id string) { app.CleanupBrokenVellum(id) }); !run.result.OK {
		t.Fatalf("cleanup: %+v; output:\n%s", run.result, run.output)
	}
	if leftover := tablet.run("ls -A /home/root/.vellum 2>/dev/null || true"); strings.TrimSpace(leftover) != "" {
		t.Errorf("the cleanup left files behind: %s", leftover)
	}
	if vellumInstalled(t, app) {
		t.Error("Vellum still reported installed after the cleanup")
	}
	app.mu.Lock()
	state := app.connectState
	app.mu.Unlock()
	if !state.vellumChecked || state.vellumFound {
		t.Errorf("after the cleanup the connect state records Vellum checked=%v found=%v, want missing", state.vellumChecked, state.vellumFound)
	}
}

func TestVellumUninstall(t *testing.T) {
	requireDocker(t)
	tablet, app := vellumTablet(t)
	mod := modsSmallMod(t, app)

	for _, removeAllPackages := range []bool{true, false} {
		t.Run(fmt.Sprintf("removeAllPackages=%v", removeAllPackages), func(t *testing.T) {
			if !vellumInstalled(t, app) {
				modsBootstrapAgain(t, app)
			}
			modsVellum(t, tablet, "add "+mod)

			run := runOp(t, app, 2*time.Minute, nil, func(id string) { app.UninstallVellum(id, removeAllPackages) })
			if !run.result.OK {
				t.Fatalf("uninstall: %+v; output:\n%s", run.result, run.output)
			}
			if vellumInstalled(t, app) {
				t.Error("Vellum still reported installed after the uninstall")
			}
			if out := tablet.run("test -e /home/root/.vellum/bin/vellum && echo present || echo gone"); strings.TrimSpace(out) != "gone" {
				t.Errorf("the vellum binary is still on the tablet after the uninstall")
			}
		})
	}
}

// Asks the tablet rather than the app's Vellum client, which an uninstall or cleanup drops.
func vellumInstalled(t *testing.T, app *testApp) bool {
	t.Helper()
	installed, err := vellum.NewClient(&wailsExecutor{core: app.core}).IsInstalled()
	if err != nil {
		t.Fatalf("check for Vellum: %v", err)
	}
	return installed
}

func TestModsPlanInstall(t *testing.T) {
	requireDocker(t)
	tablet, app := vellumTablet(t)
	mod := modsHashtableMod(t, app)

	sim, err := app.SimulateInstall([]string{mod}, nil, paperPro)
	if err != nil || sim.Error != "" {
		t.Fatalf("SimulateInstall(%s) = %+v, %v", mod, sim, err)
	}
	for _, name := range []string{mod, "qt-resource-rebuilder", "xovi"} {
		if !slices.Contains(sim.Packages, name) {
			t.Errorf("the plan for %s leaves out %s: %v", mod, name, sim.Packages)
		}
	}

	var opID string
	run := runOp(t, app, 2*time.Minute, modsAnswerHashtable("Start without mods"), func(id string) {
		opID = id
		app.InstallPackages(id, []string{mod}, []string{mod}, paperPro)
	})
	if !run.result.OK || run.result.Install == nil || !run.result.Install.Success {
		t.Fatalf("install of %s: %+v; output:\n%s", mod, run.result, run.output)
	}

	progress := modsProgress(opID)
	if !slices.ContainsFunc(progress, func(p InstallProgress) bool {
		return p.Component == mod && p.Status == "completed"
	}) {
		t.Errorf("no completed progress for %s: %+v", mod, progress)
	}
	if log := modsProgressLog(progress); !strings.Contains(log, "Installing "+mod) {
		t.Errorf("the install log never names %s:\n%s", mod, log)
	}

	if titles := modsPromptTitles(run.prompts); !slices.Equal(titles, []string{"Build hashtable", "Hashtable rebuilt"}) {
		t.Fatalf("prompts = %q, want the hashtable build and its result", titles)
	}
	rebuilt := run.prompts[1]
	for _, label := range []string{"Start with mods", "Start without mods"} {
		if actionLabeled(rebuilt, label) == "" {
			t.Errorf("Hashtable rebuilt offers no %q: %+v", label, rebuilt.Actions)
		}
	}
	if rebuilt.CancelText != "Not now" {
		t.Errorf("Hashtable rebuilt cancel = %q, want Not now", rebuilt.CancelText)
	}

	if _, ok := modsInstalledVersions(t, app)[mod]; !ok {
		t.Errorf("%s missing from the installed list after its install", mod)
	}
	status, err := app.GetXochitlStatus()
	if err != nil || !status.Running || status.XoviActive {
		t.Errorf("after Start without mods, xochitl = %+v, %v; want running without xovi", status, err)
	}
	if out := tablet.run("test -s /home/root/xovi/exthome/qt-resource-rebuilder/hashtab && echo built || echo missing"); strings.TrimSpace(out) != "built" {
		t.Error("no hashtab on the tablet after the rebuild")
	}
}

func TestModsHookRebuild(t *testing.T) {
	requireDocker(t)
	_, app := vellumTablet(t)
	mod := modsHashtableMod(t, app)

	var opID string
	run := runOp(t, app, 2*time.Minute, modsAnswerHashtable("Start with mods"), func(id string) {
		opID = id
		app.InstallPackages(id, []string{mod}, []string{mod}, paperPro)
	})
	if !run.result.OK {
		t.Fatalf("install of %s: %+v; output:\n%s", mod, run.result, run.output)
	}

	var hooks []OperationHook
	var hookOutput strings.Builder
	for _, event := range emitted.since(0) {
		switch data := event.Data.(type) {
		case OperationHook:
			if data.ID == opID {
				hooks = append(hooks, data)
			}
		case OperationOutput:
			if data.ID == opID && data.Hook {
				hookOutput.WriteString(data.Text)
			}
		}
	}
	if len(hooks) != 2 {
		t.Fatalf("want a hook start and finish, got %+v", hooks)
	}
	if hooks[0].Finished || hooks[0].ExpectedSeconds != 65 {
		t.Errorf("hook start = %+v, want unfinished with ExpectedSeconds 65", hooks[0])
	}
	if !hooks[1].Finished || hooks[1].Error != "" {
		t.Errorf("hook finish = %+v, want finished without an error", hooks[1])
	}
	if hooks[0].Title != hooks[1].Title {
		t.Errorf("hook start %q and finish %q carry different titles", hooks[0].Title, hooks[1].Title)
	}
	if strings.Count(hookOutput.String(), "$ /home/root/xovi/rebuild_hashtable") != 1 {
		t.Errorf("want the rebuild command printed once in the hook output:\n%s", hookOutput.String())
	}

	status, err := app.GetXochitlStatus()
	if err != nil || !status.Running || !status.XoviActive {
		t.Errorf("after Start with mods, xochitl = %+v, %v; want running with xovi active", status, err)
	}
}

func TestModsPlanRemove(t *testing.T) {
	requireDocker(t)
	tablet, app := vellumTablet(t)
	mod := modsSmallMod(t, app)
	modsVellum(t, tablet, "add "+mod)
	app.vellumClient.InvalidateInstalledCache()

	sim, err := app.SimulateUninstall([]string{mod}, nil, paperPro)
	if err != nil || !slices.Equal(sim.Packages, []string{mod}) || sim.InstallFirst {
		t.Fatalf("SimulateUninstall(%s) = %+v, %v; want %s alone", mod, sim, err, mod)
	}

	var opID string
	run := runOp(t, app, 2*time.Minute, nil, func(id string) {
		opID = id
		app.UninstallPackages(id, []string{mod}, paperPro)
	})
	if !run.result.OK {
		t.Fatalf("removal of %s: %+v; output:\n%s", mod, run.result, run.output)
	}
	if !slices.ContainsFunc(modsProgress(opID), func(p InstallProgress) bool {
		return p.Component == mod && p.Status == "completed"
	}) {
		t.Errorf("no completed progress for %s: %+v", mod, modsProgress(opID))
	}
	if _, ok := modsInstalledVersions(t, app)[mod]; ok {
		t.Errorf("%s still installed after its removal", mod)
	}
}

func TestModsPlanMixedRunsRemoveUpgradeInstall(t *testing.T) {
	requireDocker(t)
	tablet, app := vellumTablet(t)
	upgradable, older := modsUpgradableMod(t, app)
	removal := modsSmallMod(t, app, upgradable)
	install := modsSmallMod(t, app, upgradable, removal)
	modsVellum(t, tablet, "add "+removal)
	modsInstallOlder(t, tablet, app, upgradable, older)

	plan, err := app.SimulateUninstall([]string{removal}, []string{install}, paperPro)
	if err != nil || plan.InstallFirst {
		t.Fatalf("SimulateUninstall = %+v, %v; want the removal to run first", plan, err)
	}
	candidates, err := app.GetUpgradeCandidates()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := candidates[upgradable]; !ok {
		t.Fatalf("%s is not an upgrade candidate: %v", upgradable, candidates)
	}

	phases := []struct {
		name  string
		start func(id string)
	}{
		{"remove", func(id string) { app.UninstallPackages(id, []string{removal}, paperPro) }},
		{"upgrade", func(id string) { app.RunPackageUpgrade(id, []string{upgradable}) }},
		{"install", func(id string) { app.InstallPackages(id, []string{install}, []string{install}, paperPro) }},
	}
	for _, phase := range phases {
		if run := runOp(t, app, 2*time.Minute, nil, phase.start); !run.result.OK {
			t.Fatalf("%s phase: %+v; output:\n%s", phase.name, run.result, run.output)
		}
	}

	installed := modsInstalledVersions(t, app)
	if _, ok := installed[removal]; ok {
		t.Errorf("%s still installed", removal)
	}
	if installed[upgradable] == older {
		t.Errorf("%s still at %s", upgradable, older)
	}
	if _, ok := installed[install]; !ok {
		t.Errorf("%s not installed", install)
	}
}

func TestModsPlanInstallsFirstWhenARemovalWouldPurgeWhatItNeeds(t *testing.T) {
	requireDocker(t)
	tablet, app := vellumTablet(t)
	hashtableMods := modsCatalogPicks(app, modsNeedsOnlyTheHashtable)
	if len(hashtableMods) < 2 {
		t.Fatalf("want two compatible mods needing only qt-resource-rebuilder, the catalog has %v", hashtableMods)
	}
	removal, install := hashtableMods[0], hashtableMods[1]
	modsVellum(t, tablet, "add "+removal)
	app.vellumClient.InvalidateInstalledCache()

	alone, err := app.SimulateUninstall([]string{removal}, nil, paperPro)
	if err != nil || !slices.Contains(alone.Packages, "qt-resource-rebuilder") {
		t.Fatalf("removing %s alone = %+v, %v; want it to purge qt-resource-rebuilder", removal, alone, err)
	}
	plan, err := app.SimulateUninstall([]string{removal}, []string{install}, paperPro)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.InstallFirst {
		t.Errorf("removing %s while installing %s should install first, since both need qt-resource-rebuilder: %+v", removal, install, plan)
	}
	if !slices.Equal(plan.Packages, []string{removal}) {
		t.Errorf("the combined plan removes %v, want %s alone", plan.Packages, removal)
	}

	phases := []func(id string){
		func(id string) { app.InstallPackages(id, []string{install}, []string{install}, paperPro) },
		func(id string) { app.UninstallPackages(id, []string{removal}, paperPro) },
	}
	for _, start := range phases {
		if run := runOp(t, app, 2*time.Minute, modsDeclineHashtable, start); !run.result.OK {
			t.Fatalf("%+v; output:\n%s", run.result, run.output)
		}
	}
	installed := modsInstalledVersions(t, app)
	if _, ok := installed[removal]; ok {
		t.Errorf("%s still installed", removal)
	}
	for _, name := range []string{install, "qt-resource-rebuilder", "xovi"} {
		if _, ok := installed[name]; !ok {
			t.Errorf("%s not installed after installing %s and removing %s", name, install, removal)
		}
	}
	if removals := modsCommandsContaining(tablet.commands(), "vellum del '"+removal+"'"); len(removals) != 1 {
		t.Errorf("want one removal on the tablet, got %q", removals)
	}
}

func TestModsLauncherChoice(t *testing.T) {
	requireDocker(t)
	_, app := vellumTablet(t)
	launcherApp := modsLauncherApp(t, app)

	choices := app.GetInstallChoices([]string{launcherApp}, paperPro)
	if len(choices) != 1 || choices[0].Virtual != "launcher" {
		t.Fatalf("GetInstallChoices(%s) = %+v, want one launcher choice", launcherApp, choices)
	}
	choice := choices[0]
	if !slices.Contains(choice.RequiredBy, launcherApp) {
		t.Errorf("the launcher choice is required by %v, want %s", choice.RequiredBy, launcherApp)
	}
	var providers []string
	for _, provider := range choice.Providers {
		providers = append(providers, provider.Name)
	}
	for _, want := range []string{"appload", "oxide"} {
		if !slices.Contains(providers, want) {
			t.Errorf("the launcher providers %v leave out %s", providers, want)
		}
	}
	if choice.Default != "appload" {
		t.Errorf("default launcher = %q, want appload", choice.Default)
	}

	unanswered, err := app.SimulateInstall([]string{launcherApp}, nil, paperPro)
	if err != nil || unanswered.Error != "" || !slices.Contains(unanswered.Packages, choice.PreferredProvider()) {
		t.Errorf("SimulateInstall with no pick = %+v, %v; want the preferred provider %s solved in", unanswered, err, choice.PreferredProvider())
	}

	picked, err := app.SimulateInstall([]string{"oxide", launcherApp}, nil, paperPro)
	if err != nil || picked.Error != "" {
		t.Fatalf("SimulateInstall with oxide picked = %+v, %v", picked, err)
	}
	if !slices.Contains(picked.Packages, "oxide") || slices.Contains(picked.Packages, "appload") {
		t.Errorf("with oxide picked the plan is %v, want oxide and not appload", picked.Packages)
	}
}

func TestModsInstalledProviderSatisfiesTheVirtual(t *testing.T) {
	requireDocker(t)
	_, app := vellumTablet(t)
	launcherApps := modsCatalogPicks(app, func(pkg PackageInfo) bool { return slices.Equal(pkg.Depends, []string{"launcher"}) })
	if len(launcherApps) < 2 {
		t.Skipf("the catalog has fewer than two compatible apps needing only a launcher: %v", launcherApps)
	}
	first, second := launcherApps[0], launcherApps[1]

	provider := app.GetInstallChoices([]string{first}, paperPro)[0].PreferredProvider()
	request := []string{provider, first}
	if run := runOp(t, app, 2*time.Minute, modsDeclineHashtable, func(id string) { app.InstallPackages(id, request, request, paperPro) }); !run.result.OK {
		t.Fatalf("install of %v: %+v; output:\n%s", request, run.result, run.output)
	}

	if choices := app.GetInstallChoices([]string{second}, paperPro); len(choices) != 0 {
		t.Errorf("with %s installed, %s still asks for a launcher: %+v", provider, second, choices)
	}
	sim, err := app.SimulateInstall([]string{second}, nil, paperPro)
	if err != nil || sim.Error != "" || len(sim.UnresolvedVirtuals) != 0 {
		t.Fatalf("SimulateInstall(%s) with %s installed = %+v, %v", second, provider, sim, err)
	}
	if !slices.Equal(sim.Packages, []string{second}) {
		t.Errorf("SimulateInstall(%s) = %v, want only %s", second, sim.Packages, second)
	}
	if run := runOp(t, app, 2*time.Minute, nil, func(id string) { app.InstallPackages(id, []string{second}, []string{second}, paperPro) }); !run.result.OK {
		t.Fatalf("install of %s: %+v; output:\n%s", second, run.result, run.output)
	}
}

func TestModsDependentUninstall(t *testing.T) {
	requireDocker(t)
	tablet, app := vellumTablet(t)
	apploadAddOn := modsCatalogPick(t, app, func(pkg PackageInfo) bool { return slices.Equal(pkg.Depends, []string{"appload"}) })
	modsVellum(t, tablet, "add "+apploadAddOn)
	app.vellumClient.InvalidateInstalledCache()

	sim, err := app.SimulateUninstall([]string{"appload"}, nil, paperPro)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"appload", apploadAddOn} {
		if !slices.Contains(sim.Packages, name) {
			t.Errorf("removing appload lists %v, which leaves out %s", sim.Packages, name)
		}
	}

	run := runOp(t, app, 2*time.Minute, nil, func(id string) { app.UninstallPackages(id, []string{"appload"}, paperPro) })
	if !run.result.OK {
		t.Fatalf("removal of appload: %+v; output:\n%s", run.result, run.output)
	}
	installed := modsInstalledVersions(t, app)
	for _, name := range []string{"appload", apploadAddOn} {
		if _, ok := installed[name]; ok {
			t.Errorf("%s still installed after removing appload", name)
		}
	}
}

func TestModsDependentUninstallOfAVersionPinnedPackage(t *testing.T) {
	requireDocker(t)
	tablet, app := vellumTablet(t)
	apploadAddOn := modsCatalogPick(t, app, func(pkg PackageInfo) bool { return slices.Equal(pkg.Depends, []string{"appload"}) })
	modsVellum(t, tablet, "add "+apploadAddOn)
	pinned := "appload=" + modsInstalledVersions(t, app)["appload"]
	modsVellum(t, tablet, "add "+pinned)
	if world := tablet.run("cat " + vellum.VellumRoot + "/etc/apk/world"); !strings.Contains(world, pinned) {
		t.Fatalf("arranged a world entry %s, the world file has:\n%s", pinned, world)
	}
	app.vellumClient.InvalidateInstalledCache()

	sim, err := app.SimulateUninstall([]string{"appload"}, nil, paperPro)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"appload", apploadAddOn} {
		if !slices.Contains(sim.Packages, name) {
			t.Errorf("removing %s lists %v, which leaves out %s", pinned, sim.Packages, name)
		}
	}

	run := runOp(t, app, 2*time.Minute, nil, func(id string) { app.UninstallPackages(id, []string{"appload"}, paperPro) })
	if !run.result.OK {
		t.Fatalf("removal of %s: %+v; output:\n%s", pinned, run.result, run.output)
	}
	installed := modsInstalledVersions(t, app)
	for _, name := range []string{"appload", apploadAddOn} {
		if _, ok := installed[name]; ok {
			t.Errorf("%s still installed after removing %s", name, pinned)
		}
	}
}

func TestModsUpgradesView(t *testing.T) {
	requireDocker(t)
	tablet, app := vellumTablet(t)
	upgradable, older := modsUpgradableMod(t, app)
	modsInstallOlder(t, tablet, app, upgradable, older)

	candidates, err := app.GetUpgradeCandidates()
	if err != nil {
		t.Fatal(err)
	}
	candidate, ok := candidates[upgradable]
	if !ok {
		t.Fatalf("%s at %s is not an upgrade candidate: %v", upgradable, older, candidates)
	}
	if candidate.From != older || candidate.To == older || candidate.To == "" {
		t.Errorf("candidate %s = %+v, want from %s to a newer version", upgradable, candidate, older)
	}

	var opID string
	run := runOp(t, app, 2*time.Minute, nil, func(id string) {
		opID = id
		app.RunPackageUpgrade(id, []string{upgradable})
	})
	if !run.result.OK || run.result.Upgrade == nil {
		t.Fatalf("upgrade of %s: %+v; output:\n%s", upgradable, run.result, run.output)
	}
	if !strings.Contains(run.output, "Upgrading "+upgradable) || !strings.Contains(run.output, "Upgrade complete.") {
		t.Errorf("the upgrade output does not show %s upgrading to completion:\n%s", upgradable, run.output)
	}
	if !slices.ContainsFunc(modsProgress(opID), func(p InstallProgress) bool {
		return p.Component == upgradable && p.Status == "completed"
	}) {
		t.Errorf("no completed progress for %s: %+v", upgradable, modsProgress(opID))
	}
	if version := modsInstalledVersions(t, app)[upgradable]; version != candidate.To {
		t.Errorf("%s is at %s after the upgrade, want %s", upgradable, version, candidate.To)
	}
	after, err := app.GetUpgradeCandidates()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after[upgradable]; ok {
		t.Errorf("%s is still an upgrade candidate after its upgrade: %v", upgradable, after)
	}
	if world := tablet.run("cat /home/root/.vellum/etc/apk/world"); strings.Contains(world, upgradable+"=") {
		t.Errorf("the upgrade pinned %s in the world file:\n%s", upgradable, world)
	}
}

func TestModsPlanStopKeepsWhatLanded(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{stepSeconds: 2})
	app := connectApp(t, tablet)
	modsProxyMode(t, app, false)
	packages := []string{"stop-one", "stop-two", "stop-three", "stop-four"}

	var opID string
	var stopped atomic.Bool
	run := runOp(t, app, time.Minute, nil, func(id string) {
		opID = id
		app.InstallPackages(id, packages, packages, paperPro)
		go func() {
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				if slices.ContainsFunc(modsProgress(id), func(p InstallProgress) bool { return p.Component == packages[1] }) {
					stopped.Store(true)
					app.CancelOperation(id)
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
		}()
	})
	if !stopped.Load() {
		t.Fatalf("the install never reached %s; progress: %+v", packages[1], modsProgress(opID))
	}
	if !run.result.Canceled || run.result.OK {
		t.Errorf("a stopped install finished as %+v, want canceled", run.result)
	}

	landed := modsFakeInstalled(tablet)
	time.Sleep(5 * time.Second)
	if later := modsFakeInstalled(tablet); !slices.Equal(later, landed) {
		t.Errorf("packages kept landing after the stop: %v then %v", landed, later)
	}
	if !slices.Contains(landed, packages[0]) || slices.Contains(landed, packages[len(packages)-1]) {
		t.Errorf("installed after the stop: %v, want %s and not %s", landed, packages[0], packages[len(packages)-1])
	}
	app.vellumClient.InvalidateInstalledCache()
	installed := modsInstalledVersions(t, app)
	for _, name := range packages {
		if _, ok := installed[name]; ok != slices.Contains(landed, name) {
			t.Errorf("the installed list says %s installed=%v, the tablet says %v", name, ok, landed)
		}
	}
}

func TestModsFailedInstallEndsWithTheReason(t *testing.T) {
	requireDocker(t)
	tablet := startFakeTablet(t, fakeTabletOptions{stepSeconds: 0})
	app := connectApp(t, tablet)
	modsProxyMode(t, app, false)
	tablet.failInstalls(true)
	packages := []string{"fail-one", "fail-two"}

	var opID string
	run := runOp(t, app, time.Minute, nil, func(id string) {
		opID = id
		app.InstallPackages(id, packages, packages, paperPro)
	})
	if run.result.OK || run.result.Canceled {
		t.Fatalf("a failing install finished as %+v, want failed", run.result)
	}
	if !strings.Contains(run.result.Error, "simulated failure") {
		t.Errorf("the result error %q does not carry vellum's reason", run.result.Error)
	}
	lines := strings.Split(strings.TrimSpace(run.output), "\n")
	if last := lines[len(lines)-1]; !strings.Contains(last, "simulated failure") {
		t.Errorf("the output ends with %q, want the reason:\n%s", last, run.output)
	}
	if !slices.ContainsFunc(modsProgress(opID), func(p InstallProgress) bool { return p.Status == "error" }) {
		t.Errorf("no progress row turned to error: %+v", modsProgress(opID))
	}
}

func TestModsProxyMode(t *testing.T) {
	requireDocker(t)
	tablet, app := vellumTablet(t)
	mods := modsCatalogPicks(app, modsIsSmallMod)
	if len(mods) < 2 {
		t.Fatalf("want two small mods, the catalog has %v", mods)
	}
	throughReManager := "Downloading packages through reManager"

	t.Run("on", func(t *testing.T) {
		modsProxyMode(t, app, true)
		tablet.run("cp /etc/hosts /etc/hosts.online && echo '127.0.0.1 packages.vellum.delivery' >> /etc/hosts")
		defer tablet.run("cat /etc/hosts.online > /etc/hosts")

		var opID string
		run := runOp(t, app, 2*time.Minute, nil, func(id string) {
			opID = id
			app.InstallPackages(id, []string{mods[0]}, []string{mods[0]}, paperPro)
		})
		if !run.result.OK {
			t.Fatalf("with the repository out of the tablet's reach, the proxied install of %s failed: %+v; log:\n%s", mods[0], run.result, modsProgressLog(modsProgress(opID)))
		}
		log := modsProgressLog(modsProgress(opID))
		if !strings.Contains(log, throughReManager) || !strings.Contains(log, "Transferring "+mods[0]) {
			t.Errorf("the proxied install never transferred %s from reManager:\n%s", mods[0], log)
		}
	})

	t.Run("off", func(t *testing.T) {
		modsProxyMode(t, app, false)
		if app.GetSettings().ProxyMode {
			t.Fatal("UpdateSettings did not turn proxy mode off")
		}
		install := func() (operationRun, string) {
			var opID string
			run := runOp(t, app, 2*time.Minute, nil, func(id string) {
				opID = id
				app.InstallPackages(id, []string{mods[1]}, []string{mods[1]}, paperPro)
			})
			return run, modsProgressLog(modsProgress(opID))
		}

		tablet.run("cp /etc/hosts /etc/hosts.online && echo '127.0.0.1 packages.vellum.delivery' >> /etc/hosts")
		offline, _ := install()
		tablet.run("cat /etc/hosts.online > /etc/hosts")
		if offline.result.OK {
			t.Errorf("with proxy mode off and the repository out of the tablet's reach, the install of %s still succeeded", mods[1])
		}

		run, log := install()
		if !run.result.OK {
			t.Fatalf("install of %s without proxy mode: %+v; log:\n%s", mods[1], run.result, log)
		}
		if strings.Contains(log, throughReManager) {
			t.Errorf("with proxy mode off the install still went through reManager:\n%s", log)
		}
		if solved := modsCommandsContaining(tablet.commands(), "vellum add --simulate '"+mods[1]+"'"); len(solved) != 0 {
			t.Errorf("with proxy mode off reManager still solved %s for a download: %q", mods[1], solved)
		}
		if _, ok := modsInstalledVersions(t, app)[mods[1]]; !ok {
			t.Errorf("%s not installed", mods[1])
		}
	})
}

func modsAnswerHashtable(startChoice string) promptAnswerer {
	return func(request DialogRequest) string {
		switch request.Title {
		case "Build hashtable":
			return "confirm"
		case "Hashtable rebuilt":
			return actionLabeled(request, startChoice)
		}
		return "cancel"
	}
}

func modsDeclineHashtable(DialogRequest) string { return "cancel" }

func modsPromptTitles(prompts []DialogRequest) []string {
	titles := make([]string, len(prompts))
	for i, prompt := range prompts {
		titles[i] = prompt.Title
	}
	return titles
}

func modsProgress(opID string) []InstallProgress {
	var progress []InstallProgress
	for _, event := range emitted.since(0) {
		if data, ok := event.Data.(OperationProgress); ok && data.ID == opID {
			progress = append(progress, data.Progress)
		}
	}
	return progress
}

// Install and removal report vellum's lines as progress messages rather than output, and this is the log the Mods plan shows from them.
func modsProgressLog(progress []InstallProgress) string {
	var log strings.Builder
	for _, p := range progress {
		if p.Message != "" {
			log.WriteString(p.Message + "\n")
		}
	}
	return log.String()
}

func modsInstalledVersions(t *testing.T, app *testApp) map[string]string {
	t.Helper()
	app.vellumClient.InvalidateInstalledCache()
	listed, err := app.GetInstalledPackagesWithVersions()
	if err != nil {
		t.Fatalf("GetInstalledPackagesWithVersions: %v", err)
	}
	versions := make(map[string]string, len(listed))
	for name, versionAndTime := range listed {
		version, _, _ := strings.Cut(versionAndTime, "|")
		versions[name] = version
	}
	return versions
}

func modsInstalledPackages(t *testing.T, tablet *fakeTablet) map[string]bool {
	t.Helper()
	installed := map[string]bool{}
	for _, name := range strings.Fields(modsVellum(t, tablet, "info -q")) {
		installed[name] = true
	}
	return installed
}

func modsFakeInstalled(tablet *fakeTablet) []string {
	var names []string
	for _, line := range strings.Split(tablet.run("cat /home/root/.vellum/state/fake-installed"), "\n") {
		if name, _, ok := strings.Cut(line, " "); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func modsVellum(t *testing.T, tablet *fakeTablet, args string) string {
	t.Helper()
	return tablet.run(vellum.VellumBin + " " + args + " 2>&1")
}

func modsCommandsContaining(commands, fragment string) []string {
	var matched []string
	for _, line := range strings.Split(commands, "\n") {
		if strings.Contains(line, fragment) {
			matched = append(matched, line)
		}
	}
	return matched
}

func modsBootstrapAgain(t *testing.T, app *testApp) {
	t.Helper()
	t.Setenv("VELLUM_BOOTSTRAP_DIR", cachedBootstrapFiles(t))
	if run := runOp(t, app, 5*time.Minute, nil, func(id string) { app.BootstrapVellum(id) }); !run.result.OK {
		t.Fatalf("bootstrap: %+v; output:\n%s", run.result, run.output)
	}
}

// The setting is stored in the shared sandbox, so it goes back to what it was for the tests that run after this one.
func modsProxyMode(t *testing.T, app *testApp, on bool) {
	t.Helper()
	save := func(proxyMode bool) {
		if err := app.UpdateSettings(SettingsPatch{ProxyMode: &proxyMode}); err != nil {
			t.Fatalf("UpdateSettings: %v", err)
		}
	}
	before := app.GetSettings().ProxyMode
	t.Cleanup(func() { save(before) })
	save(on)
}

func modsCatalogPicks(app *testApp, match func(PackageInfo) bool) []string {
	var names []string
	for _, pkg := range app.GetPackages(paperPro, currentOSVersion, "aarch64") {
		if pkg.Compatible && !pkg.Hidden && match(pkg) {
			names = append(names, pkg.Name)
		}
	}
	sort.Strings(names)
	return names
}

func modsCatalogPick(t *testing.T, app *testApp, match func(PackageInfo) bool) string {
	t.Helper()
	names := modsCatalogPicks(app, match)
	if len(names) == 0 {
		t.Fatal("no compatible package in the catalog fits this test")
	}
	return names[0]
}

// Command-line tools with no dependencies and no hooks, which install in a second and touch nothing outside Vellum's own tree.
var modsSmallModPreference = []string{"jq", "less", "vellum-hosts", "micro"}

func modsIsSmallMod(pkg PackageInfo) bool {
	return slices.Contains(modsSmallModPreference, pkg.Name) && len(pkg.Depends) == 0
}

func modsSmallMod(t *testing.T, app *testApp, except ...string) string {
	t.Helper()
	available := modsCatalogPicks(app, modsIsSmallMod)
	for _, name := range modsSmallModPreference {
		if slices.Contains(available, name) && !slices.Contains(except, name) {
			return name
		}
	}
	t.Fatalf("none of %v is in the catalog apart from %v", modsSmallModPreference, except)
	return ""
}

func modsNeedsOnlyTheHashtable(pkg PackageInfo) bool {
	return slices.Equal(pkg.Depends, []string{"qt-resource-rebuilder"})
}

func modsHashtableMod(t *testing.T, app *testApp) string {
	t.Helper()
	return modsCatalogPick(t, app, modsNeedsOnlyTheHashtable)
}

func modsLauncherApp(t *testing.T, app *testApp) string {
	t.Helper()
	return modsCatalogPick(t, app, func(pkg PackageInfo) bool { return slices.Equal(pkg.Depends, []string{"launcher"}) })
}

func modsUpgradableMod(t *testing.T, app *testApp) (name, olderVersion string) {
	t.Helper()
	for _, name := range modsCatalogPicks(app, func(pkg PackageInfo) bool { return len(pkg.Depends) == 0 && len(pkg.Provides) == 0 }) {
		var compatible []string
		for _, release := range app.GetPackageReleases(name) {
			if release.Compatible {
				compatible = append(compatible, release.Version)
			}
		}
		if len(compatible) >= 2 {
			return name, compatible[1]
		}
	}
	t.Fatal("no compatible package in the catalog has two installable releases")
	return "", ""
}

// Installing a version pins it in the world file, where apk would then refuse to upgrade it, so the pin is dropped to leave it installed as a plain add.
func modsInstallOlder(t *testing.T, tablet *fakeTablet, app *testApp, name, version string) {
	t.Helper()
	modsVellum(t, tablet, "add "+name+"="+version)
	tablet.run(fmt.Sprintf("sed -i 's/^%s=.*/%s/' %s/etc/apk/world", name, name, vellum.VellumRoot))
	if installed := modsInstalledVersions(t, app)[name]; installed != version {
		t.Fatalf("arranged %s at %s, the tablet has %q", name, version, installed)
	}
}
