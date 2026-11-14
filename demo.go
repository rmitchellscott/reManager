package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"
	"github.com/rmitchellscott/remarkable-go/partition"
	"reManager/internal/backup"
	"reManager/internal/vellum"
)

// An App Store reviewer has no reMarkable; reMarkable generates a random device password, so this demo pair cannot collide with a real credential.
const (
	demoHost   = "demo"
	demoSecret = "demo"
	// Store screenshots need the terminal and file browser to look like a real tablet's, hiding from App Review why nothing runs, so the lookalike sits behind its own password.
	demoScreenshotSecret = "screenshots"
	// The screenshot demo as it connects after an OS update, for the steps that put the mods back.
	demoOSUpdateSecret = "os-update"
)

const (
	demoDeviceType = rmdevice.RMPPure
	demoArch       = string(rmdevice.Aarch64)
	// 3.20 predates usable vellum index data, so a package's OS constraints reject it and the catalog reads as incompatible.
	demoFirmware         = "3.26.0.68"
	demoFallbackFirmware = "3.25.0.142"
	demoMachineID        = "demo-device"
	demoDefaultTimezone  = "UTC"
	demoSeedCount        = 6
	demoOutdatedCount    = 3
	demoPopularShelfSize = 8

	// The connect dialog stays up while the catalog loads, because the seeded packages come from it and an empty seed leaves Installed empty.
	demoCatalogWait = 20 * time.Second
)

// The plan shot in scripts/screenshots/shots.mjs stages the first two and shows the other three added as their dependencies, so the seed leaves all five uninstalled.
var demoStorePlanPackages = map[string]bool{
	"quicksettings-screenshot": true,
	"rmstream":                 true,
	"appload":                  true,
	"framebuffer-spy":          true,
	"rm-shot":                  true,
}

var errDemoUnavailable = errors.New("This needs a real reMarkable, so it isn't available in demo mode.")

// Seeded from the real catalog, with a few pinned one real release behind so Updates and the upgrade path have something to apply.
func (s *PackagesService) seedDemoPackages() map[string]string {
	out := map[string]string{}
	if s.metadata == nil {
		return out
	}
	// The catalog arrives in map order, so without a sort every connect seeds a different tablet.
	candidates := s.metadata.GetAllPackagesForDevice(string(demoDeviceType), demoFirmware, demoArch)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Downloads30d != candidates[j].Downloads30d {
			return candidates[i].Downloads30d > candidates[j].Downloads30d
		}
		return candidates[i].Name < candidates[j].Name
	})

	// Discover's Popular this month shelf lists the most downloaded packages that are not installed, so seeding one (or anything that pulls one in) would empty it.
	popularShelf := map[string]bool{}
	var userFacing []vellum.Package
	for _, pkg := range candidates {
		if pkg.IsFramework || isHiddenPackage(pkg.Name) || pkg.Version == "" || !pkg.Compatible || pkg.Status == "deprecated" {
			continue
		}
		userFacing = append(userFacing, pkg)
		if len(popularShelf) < demoPopularShelfSize {
			popularShelf[pkg.Name] = true
		}
	}

	outdated := 0
	for _, pkg := range userFacing {
		if len(out) >= demoSeedCount {
			break
		}
		if popularShelf[pkg.Name] || s.pullsInAny(pkg.Name, popularShelf) ||
			demoStorePlanPackages[pkg.Name] || s.pullsInAny(pkg.Name, demoStorePlanPackages) {
			continue
		}
		out[pkg.Name] = pkg.Version
		if outdated < demoOutdatedCount {
			if previous := s.previousCompatibleRelease(pkg.Name, pkg.Version); previous != "" {
				out[pkg.Name] = previous
				outdated++
			}
		}
	}
	// A tablet with appload and no xovi does not exist, and a plan would then offer to install what the seeded packages already need.
	var seeded []string
	for name := range out {
		seeded = append(seeded, name)
	}
	for _, name := range s.resolveAgainst(map[string]string{}, seeded) {
		if _, present := out[name]; !present {
			out[name] = s.demoVersionOf(name)
		}
	}
	return out
}

func (s *PackagesService) pullsInAny(name string, names map[string]bool) bool {
	for _, dependency := range s.resolveAgainst(map[string]string{}, []string{name}) {
		if dependency != name && names[dependency] {
			return true
		}
	}
	return false
}

func (s *PackagesService) previousCompatibleRelease(name, current string) string {
	for _, release := range s.metadata.GetPackageReleases(name, string(demoDeviceType), demoFirmware, demoArch) {
		if release.Compatible && release.Version != current {
			return release.Version
		}
	}
	return ""
}

func (s *PackagesService) waitForDemoCatalog() {
	if s.metadata == nil {
		return
	}
	deadline := time.Now().Add(demoCatalogWait)
	for !s.metadata.Ready() && s.metadata.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
}

// Answers from the catalog filtered for this device, so a recorded version matches what the app compares against; the unfiltered newest version can differ and read as an upgrade.
func (s *PackagesService) demoCatalogEntry(name string) *vellum.Package {
	if s.metadata == nil {
		return nil
	}
	return s.metadata.GetPackageForTargetOS(name, demoFirmware, string(demoDeviceType), demoArch)
}

func (c *core) demoInstalled() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.demoPackages))
	for k, v := range c.demoPackages {
		out[k] = v
	}
	return out
}

func demoDependencyName(dep string) string {
	if end := strings.IndexAny(dep, "<>=~"); end >= 0 {
		return dep[:end]
	}
	return dep
}

// Walks the catalog's depends like a real apk resolve; a virtual dependency such as launcher has no entry of its own and arrives among the requested names instead.
func (s *PackagesService) demoResolve(requested []string) []string {
	return s.resolveAgainst(s.demoInstalled(), requested)
}

// Takes the installed set rather than reading it, because seeding runs while the lock that guards the installed set is held.
func (s *PackagesService) resolveAgainst(installed map[string]string, requested []string) []string {
	seen := map[string]bool{}
	var order []string

	var walk func(name string, isDependency bool)
	walk = func(name string, isDependency bool) {
		if seen[name] || installed[name] != "" {
			return
		}
		seen[name] = true
		pkg := s.demoCatalogEntry(name)
		if pkg == nil && isDependency {
			return
		}
		if pkg != nil {
			for _, dep := range pkg.Depends {
				if strings.HasPrefix(dep, "!") {
					continue
				}
				walk(demoDependencyName(dep), true)
			}
		}
		order = append(order, name)
	}
	for _, name := range requested {
		walk(name, false)
	}
	return order
}

func demoDeviceInfo() map[string]string {
	return map[string]string{
		"model":        "reMarkable " + demoDeviceType.DisplayName(),
		"machine":      demoDeviceType.DisplayName(),
		"firmware":     demoFirmware,
		"serial":       "RM-DEMO-0000",
		"free":         "5.2G",
		"total":        "8.0G",
		"batteryLevel": "84",
	}
}

func isDemoCredential(host, authType, secret string) bool {
	if authType != "password" || host != demoHost {
		return false
	}
	return secret == demoSecret || secret == demoScreenshotSecret || secret == demoOSUpdateSecret
}

func (c *core) inDemoMode() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.demo
}

// Stands in for establishConnection, setting the same state and events; a saved demo device keeps its own ID so device-store lookups still find it.
func (s *ConnectionService) startDemo(deviceID string, screenshots, osUpdate bool) ConnectionResult {
	if deviceID == "" {
		deviceID = demoMachineID
	}
	s.packages.waitForDemoCatalog()

	s.mu.Lock()
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	s.vellumClient = nil
	s.demo = true
	s.demoScreenshots = screenshots
	s.demoSession++
	s.demoPackages = s.packages.seedDemoPackages()
	s.demoAutoUpdates = true
	s.demoTimezone = demoDefaultTimezone
	s.connectedDeviceID = deviceID
	s.connectedDeviceType = demoDeviceType
	s.connectedDeviceArch = rmdevice.Aarch64
	s.connectedFirmware = demoFirmware
	s.connectedMachineID = demoMachineID
	previousMachineID := s.lastMachineID
	s.lastMachineID = demoMachineID
	session := s.demoSession
	s.mu.Unlock()

	// The frontend subscribes to the connect checks only once Connect has returned, so an emit made before the return can land with nothing listening.
	go func() {
		time.Sleep(time.Second)
		if !s.demoSessionCurrent(session) {
			return
		}
		gen := s.connGen.Load()
		s.emitVellumState(gen, true, nil)
		hashtab, _ := s.device.CheckHashtabVersion()
		updateService, _ := s.device.GetUpdateServiceStatus()
		xochitl, _ := s.device.GetXochitlStatus()
		timezoneStatus, _ := s.device.GetTimezoneStatus()
		warnings := ConnectWarnings{
			Hashtab:        &hashtab,
			UpdateService:  &updateService,
			Xochitl:        &xochitl,
			TimezoneStatus: &timezoneStatus,
		}
		if osUpdate {
			warnings.OSMismatch, warnings.CompatibilityStatus = s.demoOSUpdate()
		}
		s.emitConnectWarnings(gen, warnings)
		s.emitGuideStatus(gen, s.files.checkUserGuide())
	}()

	return ConnectionResult{
		Success:     true,
		Message:     "Connected to the demo reMarkable",
		Device:      string(demoDeviceType),
		FreshDevice: previousMachineID != demoMachineID,
	}
}

// The stored OS is the one the installed mods were last resolved for, so naming the standby partition's version reads as an update since the last connect.
func (s *ConnectionService) demoOSUpdate() (*OSVersionChange, *PackageCompatibilityStatus) {
	var installed []string
	for name := range s.demoInstalled() {
		if !hiddenPackages[name] {
			installed = append(installed, name)
		}
	}
	sort.Strings(installed)
	compat := s.packages.demoCheckOSCompatibility(demoFirmware)
	return &OSVersionChange{PrevVersion: demoFallbackFirmware, NewVersion: demoFirmware},
		&PackageCompatibilityStatus{
			InstalledPackages:    installed,
			CompatiblePackages:   append(compat.Compatible, compat.NoConstraint...),
			IncompatiblePackages: compat.Incompatible,
			CurrentOsVersion:     demoFirmware,
			StoredOsVersion:      demoFallbackFirmware,
		}
}

func (s *ConnectionService) stopDemo() {
	s.mu.Lock()
	wasDemo := s.demo
	s.demo = false
	s.demoScreenshots = false
	s.demoSession++
	s.demoPackages = nil
	if wasDemo {
		s.connectedDeviceID = ""
		s.connectedMachineID = ""
	}
	s.mu.Unlock()
	s.shell.stopDemoShell()
}

func (c *core) demoSessionCurrent(session uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.demo && c.demoSession == session
}

func (c *core) currentDemoSession() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.demoSession
}

// Runs on a timer rather than over SSH so progress animates; a failure set from the demo terminal stops it halfway, the shape a real failure has.
func (s *PackagesService) demoApply(op *Operation, names []string, verb string, step time.Duration, record func(installed map[string]string)) error {
	session := s.currentDemoSession()
	fail := s.takeDemoFailure()
	total := len(names)
	for i, name := range names {
		if !s.demoSessionCurrent(session) {
			return errDemoUnavailable
		}
		if op.Canceled() {
			return errOperationCanceled
		}
		op.Progress(InstallProgress{Component: name, Index: i, Total: total, Status: InstallInstalling, Message: verb + " " + name})
		time.Sleep(step)
		if fail && i >= total/2 {
			op.Progress(InstallProgress{Component: name, Index: i, Total: total, Status: InstallError, Message: demoFailureMessage})
			return errors.New(demoFailureMessage)
		}
		op.Progress(InstallProgress{Component: name, Index: i, Total: total, Status: InstallCompleted})
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.demo || s.demoSession != session {
		return errDemoUnavailable
	}
	record(s.demoPackages)
	return nil
}

func (s *PackagesService) demoVersionOf(name string) string {
	if pkg := s.demoCatalogEntry(name); pkg != nil && pkg.Version != "" {
		return pkg.Version
	}
	return "1.0.0"
}

func (s *PackagesService) demoInstall(op *Operation, requested []string) error {
	resolved := s.demoResolve(requested)
	versions := make(map[string]string, len(resolved))
	for _, name := range resolved {
		versions[name] = s.demoVersionOf(name)
	}
	return s.demoApply(op, resolved, "Installing", 900*time.Millisecond, func(installed map[string]string) {
		for name, version := range versions {
			installed[name] = version
		}
	})
}

func (s *PackagesService) demoUninstall(op *Operation, packageNames []string) error {
	return s.demoApply(op, packageNames, "Removing", 700*time.Millisecond, func(installed map[string]string) {
		for _, name := range packageNames {
			delete(installed, name)
		}
	})
}

func (s *PackagesService) demoUpgrade(op *Operation, packageNames []string) error {
	candidates := s.demoUpgradeCandidates()
	var names []string
	if len(packageNames) == 0 {
		for name := range candidates {
			names = append(names, name)
		}
	} else {
		for _, name := range packageNames {
			if _, ok := candidates[name]; ok {
				names = append(names, name)
			}
		}
	}
	return s.demoApply(op, names, "Upgrading", 900*time.Millisecond, func(installed map[string]string) {
		for _, name := range names {
			installed[name] = candidates[name].To
		}
	})
}

func (s *PackagesService) demoUpgradeCandidates() map[string]UpgradeCandidate {
	candidates := map[string]UpgradeCandidate{}
	for name, version := range s.demoInstalled() {
		pkg := s.demoCatalogEntry(name)
		if pkg == nil || pkg.Version == "" || pkg.Version == version {
			continue
		}
		candidates[name] = UpgradeCandidate{From: version, To: pkg.Version}
	}
	return candidates
}

func (s *PackagesService) demoSimulateInstall(packageNames []string) *InstallSimulationResult {
	return &InstallSimulationResult{
		Packages:  s.demoResolve(packageNames),
		Requested: packageNames,
	}
}

func (s *PackagesService) demoInstallChoices(packageNames []string) []vellum.VirtualChoice {
	if s.metadata == nil {
		return []vellum.VirtualChoice{}
	}
	var installed []string
	for name := range s.demoInstalled() {
		installed = append(installed, name)
	}
	choices := s.metadata.ResolveVirtualChoices(packageNames, installed, string(demoDeviceType), demoFirmware, demoArch)
	if choices == nil {
		return []vellum.VirtualChoice{}
	}
	return choices
}

func (s *PackagesService) demoCheckOSCompatibility(targetOS string) CompatibilityResultJSON {
	installed := s.demoInstalled()
	blocking := map[string]bool{}
	if s.metadata != nil {
		for _, name := range s.metadata.PackagesBlockingOS(installed, string(demoDeviceType), targetOS, demoArch) {
			blocking[name] = true
		}
	}
	result := CompatibilityResultJSON{Compatible: []string{}, Incompatible: []string{}, NoConstraint: []string{}}
	for name := range installed {
		if blocking[name] {
			result.Incompatible = append(result.Incompatible, name)
		} else {
			result.Compatible = append(result.Compatible, name)
		}
	}
	return result
}

func (s *DeviceService) demoXochitlStatus() XochitlStatus {
	_, xoviInstalled := s.demoInstalled()["xovi"]
	return XochitlStatus{Running: true, XoviActive: xoviInstalled}
}

func demoPartitionInfo() *partition.SystemInfo {
	return &partition.SystemInfo{
		Active:     partition.Info{Number: 2, Version: demoFirmware, Label: "a", IsActive: true, IsNextBoot: true},
		Fallback:   partition.Info{Number: 3, Version: demoFallbackFirmware, Label: "b"},
		DeviceType: demoDeviceType,
	}
}

// A command inside a hook with an estimate pauses that long, so its progress bar paces against it.
func (c *core) demoRunOperationCommand(op *Operation) error {
	op.mu.Lock()
	duration := time.Duration(op.hookSeconds) * time.Second
	op.mu.Unlock()
	if duration == 0 {
		duration = 600 * time.Millisecond
	}
	select {
	case <-time.After(duration):
	case <-op.ctx.Done():
		return errOperationCanceled
	}
	if c.takeDemoFailure() {
		return errors.New(demoFailureMessage)
	}
	return nil
}

const demoFailureMessage = "Demo failure: this run was set to fail from the demo terminal."

// The demo terminal arms a failure ("fail next"/"fail always"/"fail off"), since none can occur on the fabricated device.
func (c *core) takeDemoFailure() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.demoFailure {
	case "next":
		c.demoFailure = ""
		return true
	case "always":
		return true
	}
	return false
}

func (s *ShellService) demoShellCommand(line string) string {
	switch strings.TrimSpace(line) {
	case "":
		return ""
	case "fail next", "fail always", "fail off":
		mode := strings.TrimPrefix(strings.TrimSpace(line), "fail ")
		s.mu.Lock()
		if mode == "off" {
			s.demoFailure = ""
		} else {
			s.demoFailure = mode
		}
		s.mu.Unlock()
		return "Demo failures: " + mode + "\r\n"
	}
	return errDemoUnavailable.Error() + "\r\n"
}

func (s *PackagesService) recordDemoSystemTask(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch taskID {
	case "enable-updates":
		s.demoAutoUpdates = true
	case "disable-updates":
		s.demoAutoUpdates = false
	}
}

// The fabricated tablet's hashtable is always current, so a reviewer is never sent through a rebuild that only waits out its estimate.
func demoHashtabStatus() HashtabVersionStatus {
	return HashtabVersionStatus{Installed: true, HashtabVersion: demoFirmware, FirmwareVersion: demoFirmware}
}

const demoShellPrompt = "demo:~# "

const (
	screenshotShellPrompt = "root@imx93-tatsu:~# "
	// The stock /etc/motd of a Paper Pure, as its shell prints it at login.
	screenshotShellBanner = "\r\n" +
		"\x1b[0;1;38;5;196mｒ\x1b[0;1;38;5;202mｅ\x1b[0;1;38;5;208mＭ\x1b[0;1;38;5;214mａ\x1b[0;1;38;5;220mｒ\x1b[0;1;38;5;226mｋ\x1b[0;1;38;5;220mａ\x1b[0;1;38;5;214mｂ\x1b[0;1;38;5;208mｌ\x1b[0;1;38;5;202mｅ\x1b[0m\r\n" +
		"\x1b[0;1;38;5;226m╺\x1b[0;1;38;5;220m┳\x1b[0;1;38;5;214m╸\x1b[0;1;38;5;208m┏\x1b[0;1;38;5;202m━\x1b[0;1;38;5;196m┓\x1b[0;1;38;5;202m╺\x1b[0;1;38;5;208m┳\x1b[0;1;38;5;214m╸\x1b[0;1;38;5;220m┏\x1b[0;1;38;5;226m━\x1b[0;1;38;5;220m┓\x1b[0;1;38;5;214m╻\x1b[0;1;38;5;208m \x1b[0;1;38;5;202m╻\x1b[0m\r\n" +
		"\x1b[0;1;38;5;226m \x1b[0;1;38;5;220m┃\x1b[0;1;38;5;214m \x1b[0;1;38;5;208m┣\x1b[0;1;38;5;202m━\x1b[0;1;38;5;196m┫\x1b[0;1;38;5;202m \x1b[0;1;38;5;208m┃\x1b[0;1;38;5;214m \x1b[0;1;38;5;220m┗\x1b[0;1;38;5;226m━\x1b[0;1;38;5;220m┓\x1b[0;1;38;5;214m┃\x1b[0;1;38;5;208m \x1b[0;1;38;5;202m┃\x1b[0m\r\n" +
		"\x1b[0;1;38;5;226m \x1b[0;1;38;5;220m╹\x1b[0;1;38;5;214m \x1b[0;1;38;5;208m╹\x1b[0;1;38;5;202m \x1b[0;1;38;5;196m╹\x1b[0;1;38;5;202m \x1b[0;1;38;5;208m╹\x1b[0;1;38;5;214m \x1b[0;1;38;5;220m┗\x1b[0;1;38;5;226m━\x1b[0;1;38;5;220m┛\x1b[0;1;38;5;214m┗\x1b[0;1;38;5;208m━\x1b[0;1;38;5;202m┛\x1b[0m\r\n"
)

func (s *ShellService) demoPrompt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.demoScreenshots {
		return screenshotShellPrompt
	}
	return demoShellPrompt
}

func (s *ShellService) greetDemoShell() {
	if prompt := s.demoPrompt(); prompt == screenshotShellPrompt {
		s.wails.Event.Emit(eventShellOutput, screenshotShellBanner+prompt)
		return
	}
	s.wails.Event.Emit(eventShellOutput, "Demo mode: no reMarkable is connected, so commands typed here do not run.\r\n\r\n"+demoShellPrompt)
}

func (s *ShellService) writeToDemoShell(data string) {
	var echo strings.Builder
	for _, r := range data {
		switch r {
		case '\r', '\n':
			s.shellMu.Lock()
			line := s.demoShellInput
			s.demoShellInput = ""
			s.shellMu.Unlock()
			echo.WriteString("\r\n" + s.demoShellCommand(line) + s.demoPrompt())
		case 0x7f, '\b':
			s.shellMu.Lock()
			if n := len(s.demoShellInput); n > 0 {
				s.demoShellInput = s.demoShellInput[:n-1]
			}
			s.shellMu.Unlock()
			echo.WriteString("\b \b")
		default:
			s.shellMu.Lock()
			s.demoShellInput += string(r)
			s.shellMu.Unlock()
			echo.WriteRune(r)
		}
	}
	s.wails.Event.Emit(eventShellOutput, echo.String())
}

func (s *ShellService) demoShellRunning() bool {
	s.shellMu.Lock()
	defer s.shellMu.Unlock()
	return s.demoShellActive
}

func (s *ShellService) stopDemoShell() {
	s.shellMu.Lock()
	wasActive := s.demoShellActive
	s.demoShellActive = false
	if wasActive {
		s.shellActive = false
	}
	s.shellMu.Unlock()
	if wasActive {
		s.wails.Event.Emit(eventShellStopped)
	}
}

// /home/root of a modded Paper Pure, pruned to what the demo seed would leave there; every other folder reads as empty.
var screenshotHomeListing = []FileInfo{
	{Name: ".cache", Path: "/home/root/.cache", Size: 4096, IsDir: true, ModTime: 1786555364, Mode: "drwxr-xr-x"},
	{Name: ".config", Path: "/home/root/.config", Size: 4096, IsDir: true, ModTime: 1789341548, Mode: "drwxr-xr-x"},
	{Name: "dictionary", Path: "/home/root/dictionary", Size: 4096, IsDir: true, ModTime: 1789148893, Mode: "drwxr-xr-x"},
	{Name: ".dropbear", Path: "/home/root/.dropbear", Size: 4096, IsDir: true, ModTime: 1780001573, Mode: "drwxr-xr-x"},
	{Name: "field-notes.pdf", Path: "/home/root/field-notes.pdf", Size: 1248317, ModTime: 1790112406, Mode: "-rw-r--r--"},
	{Name: "Frankenstein.epub", Path: "/home/root/Frankenstein.epub", Size: 471892, ModTime: 1789904215, Mode: "-rw-r--r--"},
	{Name: ".local", Path: "/home/root/.local", Size: 4096, IsDir: true, ModTime: 1780001496, Mode: "drwxr-xr-x"},
	{Name: ".memfault", Path: "/home/root/.memfault", Size: 4096, IsDir: true, ModTime: 1790631833, Mode: "drwxr-xr-x"},
	{Name: ".ssh", Path: "/home/root/.ssh", Size: 4096, IsDir: true, ModTime: 1780008620, Mode: "drwx------"},
	{Name: "suspended.png", Path: "/home/root/suspended.png", Size: 183644, ModTime: 1790374958, Mode: "-rw-r--r--"},
	{Name: ".vellum", Path: "/home/root/.vellum", Size: 4096, IsDir: true, ModTime: 1788388701, Mode: "drwxr-xr-x"},
	{Name: "xovi", Path: "/home/root/xovi", Size: 4096, IsDir: true, ModTime: 1788527393, Mode: "drwxr-xr-x"},
	{Name: ".bashrc", Path: "/home/root/.bashrc", Size: 185, ModTime: 1788388762, Mode: "-rw-r--r--"},
}

func (c *core) inScreenshotDemo() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.demo && c.demoScreenshots
}

func (s *FilesService) screenshotListing(dirPath string) ([]FileInfo, bool) {
	if !s.inScreenshotDemo() {
		return nil, false
	}
	if dirPath == "/home/root" {
		return screenshotHomeListing, true
	}
	return []FileInfo{}, true
}

func (s *FilesService) rejectDemoTransfer() bool {
	if !s.inDemoMode() {
		return false
	}
	s.wails.Event.Emit(eventFileBrowserError, FileBrowserError{Message: errDemoUnavailable.Error()})
	return true
}

const (
	screenshotBackupFiles = 1284
	screenshotBackupBytes = 431_276_032
	screenshotBackupSteps = 60
)

// Runs long enough for a capture to land mid-download, and writes no archive.
func (s *FilesService) screenshotBackup(op *Operation, destPath string) (any, error) {
	op.BackupProgress(backup.Progress{Phase: backup.PhaseScanning, Message: "Scanning files on your reMarkable…"})
	for step := 1; step <= screenshotBackupSteps; step++ {
		select {
		case <-op.ctx.Done():
			return nil, op.ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
		filesDone := screenshotBackupFiles * step / screenshotBackupSteps
		op.BackupProgress(backup.Progress{
			Phase:      backup.PhaseDownloading,
			FilesDone:  filesDone,
			FilesTotal: screenshotBackupFiles,
			BytesDone:  screenshotBackupBytes * int64(step) / screenshotBackupSteps,
			BytesTotal: screenshotBackupBytes,
			Message:    fmt.Sprintf("Downloaded %d/%d files", filesDone, screenshotBackupFiles),
			Percentage: float64(step) * 100 / screenshotBackupSteps,
		})
	}
	return destPath, nil
}

// Picked from ~/Documents, as the file picker would return them, though no such files exist.
var screenshotImportFiles = []ImportFileInfo{
	{Path: "Pride and Prejudice.epub", Size: 812_544, FileType: "epub"},
	{Path: "Hiking Trail Map.pdf", Size: 2_389_101, PageCount: 6, FileType: "pdf"},
	{Path: "Meeting Notes.rmdoc", Size: 418_302, PageCount: 4, FileType: "rmdoc", VisibleName: "Meeting Notes"},
}

func screenshotImportPath(name string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Documents", name)
}

func (s *FilesService) screenshotImportPaths() ([]string, bool) {
	if !s.inScreenshotDemo() {
		return nil, false
	}
	paths := make([]string, len(screenshotImportFiles))
	for i, file := range screenshotImportFiles {
		paths[i] = screenshotImportPath(file.Path)
	}
	return paths, true
}

func (s *FilesService) screenshotImportFileInfo(localPath string) (ImportFileInfo, bool) {
	if !s.inScreenshotDemo() {
		return ImportFileInfo{}, false
	}
	for _, file := range screenshotImportFiles {
		if localPath == screenshotImportPath(file.Path) {
			file.Path = localPath
			return file, true
		}
	}
	return ImportFileInfo{}, false
}

func (s *FilesService) demoImport() error {
	time.Sleep(800 * time.Millisecond)
	if s.takeDemoFailure() {
		return errors.New(demoFailureMessage)
	}
	return nil
}

func (s *FilesService) demoGuideStatus() UserGuideStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return UserGuideStatus{Installed: s.demoHasGuide}
}

func (s *FilesService) demoInstallGuide() error {
	time.Sleep(800 * time.Millisecond)
	if s.takeDemoFailure() {
		return errors.New(demoFailureMessage)
	}
	s.mu.Lock()
	s.demoHasGuide = true
	s.mu.Unlock()
	return nil
}

func (s *PackagesService) demoBootstrapVellum(op *Operation) error {
	if err := s.demoOutputLines(op, "Installing Vellum…\n", "Vellum installed.\n"); err != nil {
		return err
	}
	if s.takeDemoFailure() {
		return errors.New(demoFailureMessage)
	}
	return nil
}

func (s *PackagesService) demoUninstallVellum(op *Operation) error {
	session := s.currentDemoSession()
	if err := s.demoOutputLines(op, "Removing installed mods…\n", "Removing Vellum…\n", "Vellum removed.\n"); err != nil {
		return err
	}
	if s.takeDemoFailure() {
		return errors.New(demoFailureMessage)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.demo || s.demoSession != session {
		return errNotConnected
	}
	s.demoPackages = map[string]string{}
	return nil
}

func (s *PackagesService) demoOutputLines(op *Operation, lines ...string) error {
	for _, line := range lines {
		select {
		case <-time.After(700 * time.Millisecond):
		case <-op.ctx.Done():
			return errOperationCanceled
		}
		op.Output(line)
	}
	return nil
}
