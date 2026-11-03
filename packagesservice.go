package main

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"slices"
	"strings"
	"time"

	"reManager/internal/component"
	"reManager/internal/debug"
	"reManager/internal/device"
	apperrors "reManager/internal/errors"
	"reManager/internal/executor"
	"reManager/internal/httputil"
	"reManager/internal/installer"
	"reManager/internal/vellum"

	"reManager/internal/commands"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"
)

type PackagesService struct {
	*core
	os *OSService
}

type PackageInfo struct {
	Name               string                `json:"name"`
	Version            string                `json:"version"`
	Description        string                `json:"description"`
	UpstreamAuthor     string                `json:"upstreamAuthor"`
	Categories         []string              `json:"categories"`
	URL                string                `json:"url"`
	License            string                `json:"license"`
	Devices            []string              `json:"devices"`
	Depends            []string              `json:"depends"`
	Conflicts          []string              `json:"conflicts"`
	Provides           []string              `json:"provides"`
	OSMin              *string               `json:"osMin"`
	OSMax              *string               `json:"osMax"`
	OSConstraints      []vellum.OSConstraint `json:"osConstraints"`
	Compatible         bool                  `json:"compatible"`
	IncompatibleReason string                `json:"incompatibleReason,omitempty"`
	Status             string                `json:"status"`
	DonateURL          *string               `json:"donateUrl"`
	ReadmeURL          *string               `json:"readmeUrl"`
	ChangelogURL       *string               `json:"changelogUrl"`
	Released           string                `json:"released"`
	FirstReleased      string                `json:"firstReleased"`
	Downloads30d       int                   `json:"downloads30d"`
	DownloadsAll       int                   `json:"downloadsAll"`
	IsFramework        bool                  `json:"isFramework"`
	// Cataloged and upgradable, but never a thing to browse for.
	Hidden bool `json:"hidden"`
}

type UpgradeCandidate struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type MaintenanceCommandInfo struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	ProgressLabel    string `json:"progressLabel,omitempty"`
	Description      string `json:"description"`
	RequiresTerminal bool   `json:"requiresTerminal"`
	ShowOutput       bool   `json:"showOutput,omitempty"`
	ExpectedSeconds  int    `json:"expectedSeconds,omitempty"`
	AllowStop        bool   `json:"allowStop"`
	Hook             string `json:"hook,omitempty"`
}

type SystemTaskInfo struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	Description       string `json:"description"`
	RequiresTerminal  bool   `json:"requiresTerminal"`
	NeedsWritableRoot bool   `json:"needsWritableRoot"`
}

type InstalledPackagesResult struct {
	Packages    []string `json:"packages"`
	OsUpgraded  bool     `json:"osUpgraded"`
	PrevVersion string   `json:"prevVersion"`
	NewVersion  string   `json:"newVersion"`
}

type OSVersionStateResult struct {
	CurrentVersion string `json:"currentVersion"`
	StoredVersion  string `json:"storedVersion"`
	Mismatch       bool   `json:"mismatch"`
}

type CompatibilityResultJSON struct {
	Compatible   []string          `json:"compatible"`
	Incompatible []string          `json:"incompatible"`
	NoConstraint []string          `json:"noConstraint"`
	FetchFailed  bool              `json:"fetchFailed"`
	StatusMap    map[string]string `json:"statusMap"`
}

type PackageCompatibilityStatus struct {
	InstalledPackages    []string          `json:"installedPackages"`
	CompatiblePackages   []string          `json:"compatiblePackages"`
	IncompatiblePackages []string          `json:"incompatiblePackages"`
	CurrentOsVersion     string            `json:"currentOsVersion"`
	StoredOsVersion      string            `json:"storedOsVersion"`
	FetchFailed          bool              `json:"fetchFailed"`
	StatusMap            map[string]string `json:"statusMap"`
}

type InstallStatus string

const (
	InstallInstalling  InstallStatus = "installing"
	InstallCompleted   InstallStatus = "completed"
	InstallError       InstallStatus = "error"
	InstallImporting   InstallStatus = "importing"
	InstallDownloading InstallStatus = "downloading"
	InstallGenerating  InstallStatus = "generating"
	InstallCollecting  InstallStatus = "collecting"
	InstallUploading   InstallStatus = "uploading"
	// The phases vellum.ProxyProgress reports, passed through as they are.
	InstallIndex        InstallStatus = "index"
	InstallResolving    InstallStatus = "resolving"
	InstallTransferring InstallStatus = "transferring"
	InstallComplete     InstallStatus = "complete"
)

type InstallProgress struct {
	Component string        `json:"component"`
	Index     int           `json:"index"`
	Total     int           `json:"total"`
	Status    InstallStatus `json:"status"`
	Message   string        `json:"message"`
}

type InstallResult struct {
	Success  bool     `json:"success"`
	Errors   []string `json:"errors"`
	DNSError bool     `json:"dnsError"`
}

type InstallSimulationResult struct {
	Packages           []string                    `json:"packages"`
	Requested          []string                    `json:"requested"`
	Error              string                      `json:"error,omitempty"`
	Conflicts          []vellum.QueueConflictEntry `json:"conflicts,omitempty"`
	UnresolvedVirtuals []string                    `json:"unresolvedVirtuals,omitempty"`
	Upgrades           map[string]UpgradeCandidate `json:"upgrades,omitempty"`
}

type UninstallSimulationResult struct {
	Packages []string `json:"packages"`
	// Set when an install in the same plan needs a package the removal would purge; installing first leaves apk a reason to keep it.
	InstallFirst bool `json:"installFirst"`
}

// Marked rather than dropped: catalog browse never offers the framework, but an upgrade the solver decided on still has to be staged.
var hiddenPackages = map[string]bool{
	"vellum":                 true,
	"vellum-bash-completion": true,
	"mount-utils":            true,
	"/bin/sh":                true,
}

func isHiddenPackage(name string) bool {
	return hiddenPackages[name] || vellum.IsVirtualPackage(name)
}

// The startup load reports its failure by event, which can fire before the frontend is listening, so the frontend also asks once it is.
func (s *PackagesService) CatalogLoadFailed() bool {
	return s.metadata != nil && !s.metadata.Ready() && s.metadata.Err() != nil
}

func (s *PackagesService) RefreshMetadata() error {
	return s.metadata.Refresh()
}

func (s *PackagesService) GetPackages(deviceType, firmware, arch string) []PackageInfo {
	debug.Printf("[DEBUG] GetPackages called, metadata=%p, deviceType=%s, firmware=%s, arch=%s\n", s.metadata, deviceType, firmware, arch)
	if s.metadata == nil || !s.metadata.Ready() {
		return []PackageInfo{}
	}
	packages := s.metadata.GetAllPackagesForDevice(deviceType, firmware, arch)
	debug.Printf("[DEBUG] GetPackages got %d packages\n", len(packages))
	result := []PackageInfo{}

	for _, pkg := range packages {
		var visibleDepends []string
		for _, dep := range pkg.Depends {
			if !hiddenPackages[vellum.ParseConstraint(dep).Name] {
				visibleDepends = append(visibleDepends, dep)
			}
		}
		result = append(result, PackageInfo{
			Name:               pkg.Name,
			Version:            pkg.Version,
			Description:        pkg.Description,
			UpstreamAuthor:     pkg.UpstreamAuthor,
			Categories:         pkg.Categories,
			URL:                pkg.URL,
			License:            pkg.License,
			Devices:            pkg.Devices,
			Depends:            visibleDepends,
			Conflicts:          pkg.Conflicts,
			Provides:           pkg.Provides,
			OSMin:              pkg.OSMin,
			OSMax:              pkg.OSMax,
			OSConstraints:      pkg.OSConstraints,
			Compatible:         pkg.Compatible,
			IncompatibleReason: pkg.IncompatibleReason,
			Status:             pkg.Status,
			DonateURL:          pkg.DonateURL,
			ReadmeURL:          pkg.ReadmeURL,
			ChangelogURL:       pkg.ChangelogURL,
			Released:           pkg.Released,
			FirstReleased:      pkg.FirstReleased,
			Downloads30d:       pkg.Downloads30d,
			DownloadsAll:       pkg.DownloadsAll,
			IsFramework:        pkg.IsFramework,
			Hidden:             hiddenPackages[pkg.Name],
		})
	}

	debug.Printf("[DEBUG] GetPackages returning %d PackageInfo\n", len(result))
	return result
}

func (s *PackagesService) GetInstalledPackagesWithOsCheck() (InstalledPackagesResult, error) {
	if s.inDemoMode() {
		return InstalledPackagesResult{}, nil
	}
	vc := s.getVellumClient()
	if vc == nil {
		return InstalledPackagesResult{}, errNotConnected
	}

	// An error rather than an empty list, which would read as a tablet with no mods installed; the frontend keeps the list it had.
	listResult, err := vc.ListWithOsCheck()
	if err != nil {
		debug.Printf("[DEBUG] Error getting installed packages: %v\n", err)
		return InstalledPackagesResult{}, err
	}

	var packages []string
	for _, pkg := range listResult.Packages {
		if !isHiddenPackage(pkg) {
			packages = append(packages, pkg)
		}
	}

	return InstalledPackagesResult{
		Packages:    packages,
		OsUpgraded:  listResult.OsUpgraded,
		PrevVersion: listResult.PrevVersion,
		NewVersion:  listResult.NewVersion,
	}, nil
}

func (s *PackagesService) GetInstalledPackagesWithVersions() (map[string]string, error) {
	if s.inDemoMode() {
		return s.demoInstalled(), nil
	}
	vc := s.getVellumClient()
	if vc == nil {
		return map[string]string{}, errNotConnected
	}

	s.mu.Lock()
	deviceID := s.connectedDeviceID
	s.mu.Unlock()

	versions, err := vc.ListInstalledWithVersions()
	if err != nil {
		debug.Printf("[DEBUG] Error getting installed packages with versions: %v\n", err)
		return map[string]string{}, err
	}
	s.recordCachedInstalled(deviceID, versions)

	times, err := vc.ListInstalledWithTimes()
	if isTransportError(err) {
		return map[string]string{}, err
	}

	result := make(map[string]string)
	for pkg, version := range versions {
		if !isHiddenPackage(pkg) {
			if ts, ok := times[pkg]; ok {
				result[pkg] = fmt.Sprintf("%s|%d", version, ts)
			} else {
				result[pkg] = version
			}
		}
	}
	return result, nil
}

// Written on every successful read, not once at disconnect: a read there queues behind whatever vellum command holds the lock, and fails on a dropped link.
func (s *PackagesService) recordCachedInstalled(deviceID string, versions map[string]string) {
	if deviceID == "" || s.deviceInfoCache == nil || versions == nil {
		return
	}

	cached, err := s.deviceInfoCache.Get(deviceID)
	if err != nil || cached == nil {
		return
	}
	cached.InstalledPackages = versions
	cached.CachedAt = time.Now().Unix()
	if err := s.deviceInfoCache.Set(deviceID, *cached); err != nil {
		debug.Printf("[DEBUG] recordCachedInstalled: %v\n", err)
	}
}

func (s *PackagesService) GetPackageForOS(name, targetOS, deviceType, arch string) *PackageInfo {
	if s.metadata == nil {
		return nil
	}

	pkg := s.metadata.GetPackageForTargetOS(name, targetOS, deviceType, arch)
	if pkg == nil {
		return nil
	}

	return &PackageInfo{
		Name:           pkg.Name,
		Version:        pkg.Version,
		Description:    pkg.Description,
		UpstreamAuthor: pkg.UpstreamAuthor,
		Categories:     pkg.Categories,
		URL:            pkg.URL,
		License:        pkg.License,
		Devices:        pkg.Devices,
		Depends:        pkg.Depends,
		Conflicts:      pkg.Conflicts,
		Provides:       pkg.Provides,
		OSMin:          pkg.OSMin,
		OSMax:          pkg.OSMax,
		OSConstraints:  pkg.OSConstraints,
		Compatible:     true,
		Status:         pkg.Status,
		DonateURL:      pkg.DonateURL,
		ReadmeURL:      pkg.ReadmeURL,
		ChangelogURL:   pkg.ChangelogURL,
		Released:       pkg.Released,
		FirstReleased:  pkg.FirstReleased,
		Downloads30d:   pkg.Downloads30d,
		DownloadsAll:   pkg.DownloadsAll,
		IsFramework:    pkg.IsFramework,
		Hidden:         hiddenPackages[pkg.Name],
	}
}

// Favorites are host-side, so one starred set covers every reMarkable connected from this machine and survives a vellum uninstall, device wipe or OS upgrade.
func (s *PackagesService) GetFavorites() []string {
	if s.favoritesStore == nil {
		return nil
	}
	favorites, err := s.favoritesStore.Load()
	if err != nil {
		debug.Printf("[DEBUG] GetFavorites: failed to load: %v\n", err)
		return nil
	}
	names := make([]string, 0, len(favorites.Packages))
	for _, fav := range favorites.Packages {
		names = append(names, fav.Name)
	}
	return names
}

func (s *PackagesService) ToggleFavorite(name string) (bool, error) {
	if s.favoritesStore == nil {
		return false, fmt.Errorf("favorites store unavailable")
	}
	return s.favoritesStore.Toggle(name)
}

// Host-side like favorites, carrying the timestamp the feed baselines against: a developer's back catalog is never new, only what they publish after you follow them.
type FollowInfo struct {
	Name       string   `json:"name"`
	FollowedAt string   `json:"followedAt"`
	Seen       []string `json:"seen"`
}

func (s *PackagesService) GetFollows() []FollowInfo {
	if s.followsStore == nil {
		return nil
	}
	s.pruneFollowSeen()
	follows, err := s.followsStore.Load()
	if err != nil {
		debug.Printf("[DEBUG] GetFollows: failed to load: %v\n", err)
		return nil
	}
	result := make([]FollowInfo, 0, len(follows.Developers))
	for _, follow := range follows.Developers {
		seen := follow.Seen
		if seen == nil {
			seen = []string{}
		}
		result = append(result, FollowInfo{
			Name:       follow.Name,
			FollowedAt: follow.FollowedAt,
			Seen:       seen,
		})
	}
	return result
}

func (s *PackagesService) ToggleFollow(name string) (bool, error) {
	if s.followsStore == nil {
		return false, fmt.Errorf("follows store unavailable")
	}
	return s.followsStore.Toggle(name)
}

func (s *PackagesService) SetPackageSeen(developer, pkg string, seen bool) error {
	if s.followsStore == nil {
		return fmt.Errorf("follows store unavailable")
	}
	return s.followsStore.SetSeen(developer, pkg, seen)
}

// Runs off GetAllPackages, not the device-filtered list, so a package merely incompatible with the connected reMarkable is not mistaken for one withdrawn.
func (s *PackagesService) pruneFollowSeen() {
	if s.metadata == nil || !s.metadata.Ready() {
		return
	}
	packages := s.metadata.GetAllPackages()
	indexed := make(map[string]bool, len(packages))
	for _, pkg := range packages {
		indexed[pkg.Name] = true
	}
	if err := s.followsStore.PruneSeen(indexed); err != nil {
		debug.Printf("[DEBUG] pruneFollowSeen: %v\n", err)
	}
}

func (s *PackagesService) GetPackageReleases(name string) []vellum.PackageRelease {
	if s.metadata == nil {
		return nil
	}
	s.mu.Lock()
	deviceType, arch, firmware := s.connectedDeviceType, s.connectedDeviceArch, s.connectedFirmware
	s.mu.Unlock()
	return s.metadata.GetPackageReleases(name, string(deviceType), firmware, commands.GetDownloadArch(arch))
}

func (s *PackagesService) GetReadmeContent(url string) (string, error) {
	parsed, err := neturl.Parse(url)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme: %s", parsed.Scheme)
	}

	client := httputil.NewClient(10 * time.Second)
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// Finishes with the status vellum reports afterward, not an assumed "ok", so a run that did not take shows as still needed.
func (s *PackagesService) RunReenable(opID string) {
	s.runOperation(opID, "Re-enabling system partition mods", func(op *Operation) (any, error) {
		if s.inDemoMode() {
			if err := s.demoRunOperationCommand(op); err != nil {
				return nil, err
			}
			return "ok", nil
		}
		vc := s.getVellumClient()
		if vc == nil {
			return nil, errNotConnected
		}

		deviceType := s.getConnectedDeviceType()
		if err := s.claimWritableRoot(deviceType); err != nil {
			return nil, err
		}
		defer s.releaseWritableRootClaim(deviceType)

		op.Output("Running vellum reenable…\n")
		if err := vc.ReenableStreaming(func(line string) {
			op.Output(line + "\n")
		}); err != nil {
			return nil, err
		}
		op.Output("\nRe-enable complete.\n")

		status, err := vc.ReenableStatus()
		if err != nil || status == "" {
			return vellum.ReenableOK, nil
		}
		return status, nil
	})
}

// vellum's solver answer, not a version compare, so it reflects dependencies and blocking constraints; in proxy mode the device has no network, so its APKINDEX is uploaded first or the solve runs stale.
func (s *PackagesService) GetUpgradeCandidates() (map[string]UpgradeCandidate, error) {
	if s.inDemoMode() {
		return s.demoUpgradeCandidates(), nil
	}
	s.mu.Lock()
	vc := s.vellumClient
	sshClient := s.client
	arch := s.connectedDeviceArch
	s.mu.Unlock()

	if vc == nil {
		return nil, fmt.Errorf("not connected")
	}

	settings, _ := s.settingsStore.Load()
	proxyEnabled := settings == nil || settings.ProxyMode
	// A running operation may be inside apk reading the index this would replace, so the solve would run against that operation's staged index.
	if proxyEnabled && sshClient != nil && !s.operationRunning() {
		proxy := vellum.NewProxy(vc, sshClient, string(arch))
		if err := proxy.UploadAPKINDEX(s.ctx, func(msg string) {
			debug.Printf("[DEBUG] GetUpgradeCandidates APKINDEX: %s\n", msg)
		}); err != nil {
			return nil, fmt.Errorf("failed to refresh package index: %w", err)
		}
	}

	candidates, err := vc.SimulateUpgradeWithVersions()
	if err != nil {
		return nil, err
	}

	// Cosmetic, so a failure here costs a version line rather than the solve.
	installed, err := vc.ListInstalledWithVersions()
	if err != nil {
		debug.Printf("[DEBUG] GetUpgradeCandidates installed versions: %v\n", err)
	}

	result := make(map[string]UpgradeCandidate, len(candidates))
	for _, c := range candidates {
		if vellum.IsVirtualPackage(c.Name) {
			continue
		}
		result[c.Name] = UpgradeCandidate{From: installed[c.Name], To: c.Version}
	}
	return result, nil
}

// An empty package list upgrades everything (the Vellum tools action); a non-empty list is the queued subset from the Updates page.
type UpgradeResult struct {
	DNSError bool                     `json:"dnsError"`
	Blocked  *CompatibilityResultJSON `json:"blocked,omitempty"`
}

func (s *PackagesService) RunPackageUpgrade(opID string, packages []string) {
	s.runOperation(opID, "Upgrading mods", func(op *Operation) (any, error) {
		if err := checkPackageNames(packages); err != nil {
			return nil, err
		}
		if s.inDemoMode() {
			return UpgradeResult{}, s.demoUpgrade(op, packages)
		}
		s.mu.Lock()
		vc := s.vellumClient
		sshClient := s.client
		arch := s.connectedDeviceArch
		s.mu.Unlock()
		if vc == nil {
			return nil, errNotConnected
		}

		settings, _ := s.settingsStore.Load()
		proxyEnabled := settings == nil || settings.ProxyMode

		if proxyEnabled && sshClient != nil {
			proxy := vellum.NewProxy(vc, sshClient, string(arch))
			op.Output("Downloading upgrades through reManager…\n")
			err := proxy.ProxyUpgradeDownload(op.ctx, func(progress vellum.ProxyProgress) {
				op.Output(progress.Message + "\n")
			}, packages...)
			if err != nil {
				return UpgradeResult{}, fmt.Errorf("could not download through reManager: %w", err)
			}
		}

		op.Output("Running vellum upgrade…\n")

		var result UpgradeResult
		progress := upgradeProgressReporter{op: op}
		before, beforeErr := vc.ListInstalledWithVersions()
		err := vc.UpgradeStreaming(func(line string) {
			op.Output(line + "\n")
			if strings.Contains(strings.ToLower(line), "dns:") {
				result.DNSError = true
			}
			progress.line(line)
		}, packages...)
		if op.Canceled() {
			reportPackageChangesAfterStop(op, vc, before, beforeErr)
		}
		if err != nil {
			return result, fmt.Errorf("upgrade failed: %w", err)
		}

		progress.finish()
		op.Output("\nUpgrade complete.\n")
		return result, nil
	})
}

// apk announces each package as it starts, so the previous one is done when the next announcement arrives.
type upgradeProgressReporter struct {
	op        *Operation
	lastPkg   string
	lastTotal int
}

func (r *upgradeProgressReporter) line(line string) {
	pkg, index, total, ok := vellum.ParseUpgradeProgress(line)
	if !ok {
		return
	}
	if r.lastPkg != "" {
		r.op.Progress(InstallProgress{Component: r.lastPkg, Index: index - 1, Total: total, Status: InstallCompleted})
	}
	r.lastPkg, r.lastTotal = pkg, total
	r.op.Progress(InstallProgress{Component: pkg, Index: index, Total: total, Status: InstallInstalling, Message: line})
}

// The last package has no successor to imply its completion.
func (r *upgradeProgressReporter) finish() {
	if r.lastPkg != "" {
		r.op.Progress(InstallProgress{Component: r.lastPkg, Index: r.lastTotal - 1, Total: r.lastTotal, Status: InstallCompleted})
	}
}

func (s *PackagesService) getOSVersionState() OSVersionStateResult {
	vc := s.getVellumClient()
	if vc == nil {
		return OSVersionStateResult{}
	}

	state, err := vc.GetOSVersionState()
	if err != nil {
		return OSVersionStateResult{}
	}

	return OSVersionStateResult{
		CurrentVersion: state.CurrentVersion,
		StoredVersion:  state.StoredVersion,
		Mismatch:       state.Mismatch,
	}
}

func (s *PackagesService) CheckOSCompatibility(targetOS string) (CompatibilityResultJSON, error) {
	if !vellum.IsValidOSVersion(targetOS) {
		return CompatibilityResultJSON{}, apperrors.New(apperrors.ErrInvalidInput,
			fmt.Sprintf("Could not check %q. Enter an OS version such as 3.20.0.92.", targetOS), nil, false)
	}
	if s.inDemoMode() {
		return s.demoCheckOSCompatibility(targetOS), nil
	}
	vc := s.getVellumClient()
	if vc == nil {
		return CompatibilityResultJSON{FetchFailed: true}, nil
	}

	result, _ := vc.CheckOSCompatibility(targetOS)
	if result == nil {
		return CompatibilityResultJSON{FetchFailed: true}, nil
	}

	statusMap := make(map[string]string)
	for _, name := range append(append(result.Compatible, result.Incompatible...), result.NoConstraint...) {
		pkg := s.metadata.GetPackage(name)
		if pkg != nil && pkg.Status != "" && pkg.Status != "maintained" {
			statusMap[name] = pkg.Status
		}
	}

	return CompatibilityResultJSON{
		Compatible:   result.Compatible,
		Incompatible: result.Incompatible,
		NoConstraint: result.NoConstraint,
		FetchFailed:  result.FetchFailed,
		StatusMap:    statusMap,
	}, nil
}

// Skipped while an operation runs, because apk may be reading the index.
func (s *PackagesService) refreshDevicePackageIndex(vc *vellum.Client, arch string) {
	if vc == nil || s.operationRunning() {
		return
	}

	settings, _ := s.settingsStore.Load()
	proxyEnabled := settings == nil || settings.ProxyMode

	s.mu.Lock()
	sshClient := s.client
	s.mu.Unlock()

	if !proxyEnabled || sshClient == nil {
		if err := vc.UpdateIndex(); err != nil {
			debug.Printf("[DEBUG] Package index refresh failed: %v\n", err)
		}
		return
	}

	proxy := vellum.NewProxy(vc, sshClient, arch)
	if err := proxy.UploadAPKINDEX(s.ctx, func(msg string) {
		debug.Printf("[DEBUG] Package index refresh: %s\n", msg)
	}); err != nil {
		debug.Printf("[DEBUG] Package index refresh failed: %v\n", err)
	}
}

func (s *PackagesService) GetPackageCompatibilityStatus() PackageCompatibilityStatus {
	debug.Println("[DEBUG] GetPackageCompatibilityStatus: called")
	vc := s.getVellumClient()
	if vc == nil {
		debug.Println("[DEBUG] GetPackageCompatibilityStatus: vellumClient is nil")
		return PackageCompatibilityStatus{FetchFailed: true}
	}

	osState, err := vc.GetOSVersionState()
	if err != nil {
		debug.Printf("[DEBUG] GetPackageCompatibilityStatus: GetOSVersionState error: %v\n", err)
		return PackageCompatibilityStatus{FetchFailed: true}
	}
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: osState=%+v\n", osState)

	installed, err := vc.List()
	if err != nil {
		debug.Printf("[DEBUG] GetPackageCompatibilityStatus: List error: %v\n", err)
		return PackageCompatibilityStatus{FetchFailed: true}
	}
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: installed=%v\n", installed)

	var filteredInstalled []string
	for _, pkg := range installed {
		if !isHiddenPackage(pkg) {
			filteredInstalled = append(filteredInstalled, pkg)
		}
	}
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: filteredInstalled=%v\n", filteredInstalled)

	_, arch := s.getConnectedDevice()
	s.refreshDevicePackageIndex(vc, string(arch))

	compat, err := vc.CheckOSCompatibility(osState.CurrentVersion)
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: compat=%+v, err=%v\n", compat, err)

	if err != nil && (compat == nil || compat.FetchFailed) {
		debug.Println("[DEBUG] GetPackageCompatibilityStatus: returning with FetchFailed due to error")
		return PackageCompatibilityStatus{
			InstalledPackages: filteredInstalled,
			CurrentOsVersion:  osState.CurrentVersion,
			StoredOsVersion:   osState.StoredVersion,
			FetchFailed:       true,
		}
	}

	allEmpty := len(compat.Compatible) == 0 && len(compat.Incompatible) == 0 && len(compat.NoConstraint) == 0
	if compat.FetchFailed || allEmpty {
		debug.Printf("[DEBUG] GetPackageCompatibilityStatus: fallback (FetchFailed=%v, allEmpty=%v)\n", compat.FetchFailed, allEmpty)
		return PackageCompatibilityStatus{
			InstalledPackages:    filteredInstalled,
			CompatiblePackages:   filteredInstalled,
			IncompatiblePackages: []string{},
			CurrentOsVersion:     osState.CurrentVersion,
			StoredOsVersion:      osState.StoredVersion,
			FetchFailed:          true,
		}
	}

	statusMap := make(map[string]string)
	for _, name := range append(append(compat.Compatible, compat.Incompatible...), compat.NoConstraint...) {
		pkg := s.metadata.GetPackage(name)
		if pkg != nil && pkg.Status != "" && pkg.Status != "maintained" {
			statusMap[name] = pkg.Status
		}
	}

	result := PackageCompatibilityStatus{
		InstalledPackages:    filteredInstalled,
		CompatiblePackages:   append(compat.Compatible, compat.NoConstraint...),
		IncompatiblePackages: compat.Incompatible,
		CurrentOsVersion:     osState.CurrentVersion,
		StoredOsVersion:      osState.StoredVersion,
		FetchFailed:          false,
		StatusMap:            statusMap,
	}
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: returning result=%+v\n", result)
	return result
}

func (s *PackagesService) RunUpgrade(opID string) {
	s.runOperation(opID, "Upgrading mods", func(op *Operation) (any, error) {
		if s.inDemoMode() {
			return UpgradeResult{}, s.demoUpgrade(op, nil)
		}
		s.mu.Lock()
		vc := s.vellumClient
		s.mu.Unlock()
		if vc == nil {
			return nil, errNotConnected
		}

		osState, err := vc.GetOSVersionState()
		if err != nil {
			return nil, errors.New("could not read the OS version")
		}

		if osState.Mismatch {
			op.Output(fmt.Sprintf("Checking mod compatibility with OS %s…\n", osState.CurrentVersion))

			// A fetch that failed without an error leaves Incompatible empty, which reads as everything compatible.
			compat, err := vc.CheckOSCompatibility(osState.CurrentVersion)
			if err != nil || compat.FetchFailed {
				return nil, errors.New("could not fetch the package index to check compatibility")
			}

			if len(compat.Incompatible) > 0 {
				return UpgradeResult{Blocked: &CompatibilityResultJSON{
					Compatible:   compat.Compatible,
					Incompatible: compat.Incompatible,
					NoConstraint: compat.NoConstraint,
					FetchFailed:  compat.FetchFailed,
				}}, fmt.Errorf("these mods do not support OS %s yet: %s", osState.CurrentVersion, strings.Join(compat.Incompatible, ", "))
			}

			op.Output("All mods are compatible. Upgrading…\n\n")
		}

		settings, _ := s.settingsStore.Load()
		proxyEnabled := settings == nil || settings.ProxyMode

		s.mu.Lock()
		sshClient := s.client
		arch := s.connectedDeviceArch
		s.mu.Unlock()

		if proxyEnabled && sshClient != nil {
			proxy := vellum.NewProxy(vc, sshClient, string(arch))
			op.Output("Downloading upgrades through reManager…\n")
			if err := proxy.ProxyUpgradeDownload(op.ctx, func(progress vellum.ProxyProgress) {
				op.Output(progress.Message + "\n")
			}); err != nil {
				return UpgradeResult{}, fmt.Errorf("could not download through reManager: %w", err)
			}
		}

		op.Output("Running vellum upgrade…\n")

		var result UpgradeResult
		progress := upgradeProgressReporter{op: op}
		before, beforeErr := vc.ListInstalledWithVersions()
		err = vc.UpgradeStreaming(func(line string) {
			op.Output(line + "\n")
			if strings.Contains(strings.ToLower(line), "dns:") {
				result.DNSError = true
			}
			progress.line(line)
		})
		if op.Canceled() {
			reportPackageChangesAfterStop(op, vc, before, beforeErr)
		}
		if err != nil {
			return result, fmt.Errorf("upgrade failed: %w", err)
		}

		progress.finish()
		op.Output("\nUpgrade complete.\n")
		return result, nil
	})
}

func (s *PackagesService) getMaintenanceCommands(pkgName string) []MaintenanceCommandInfo {
	commands := s.metadata.GetMaintenanceCommands(pkgName)
	if commands == nil {
		return nil
	}

	result := make([]MaintenanceCommandInfo, len(commands))
	for i, cmd := range commands {
		result[i] = MaintenanceCommandInfo{
			ID:               cmd.ID,
			Label:            cmd.Label,
			ProgressLabel:    cmd.ProgressLabel,
			Description:      cmd.Description,
			RequiresTerminal: cmd.RequiresTerminal,
			ShowOutput:       cmd.ShowOutput,
			ExpectedSeconds:  cmd.ExpectedSeconds,
			AllowStop:        cmd.AllowStop,
			Hook:             cmd.Hook,
		}
	}

	return result
}

func (s *PackagesService) GetAllMaintenanceCommands() map[string][]MaintenanceCommandInfo {
	result := make(map[string][]MaintenanceCommandInfo)
	for name := range s.metadata.AllMaintenanceCommands() {
		cmds := s.getMaintenanceCommands(name)
		if len(cmds) > 0 {
			result[name] = cmds
		}
	}
	return result
}

func (s *PackagesService) GetSystemTasksInfo() []SystemTaskInfo {
	result := make([]SystemTaskInfo, len(device.SystemTasks))
	for i, task := range device.SystemTasks {
		result[i] = SystemTaskInfo{
			ID:                task.ID,
			Label:             task.Label,
			Description:       task.Description,
			RequiresTerminal:  task.RequiresTerminal,
			NeedsWritableRoot: s.os.taskNeedsWritableRoot(task),
		}
	}
	return result
}

func checkPackageNames(lists ...[]string) error {
	for _, names := range lists {
		for _, name := range names {
			if !vellum.IsValidPackageName(name) {
				return apperrors.New(apperrors.ErrInvalidInput,
					fmt.Sprintf("Could not use %q. It is not a valid package name.", name), nil, false)
			}
		}
	}
	return nil
}

// removals are packages the same plan deletes first, solved as forbidden not present, so a conflict with one does not block the install that follows.
func (s *PackagesService) SimulateInstall(packageNames []string, removals []string, deviceType string) (*InstallSimulationResult, error) {
	if err := checkPackageNames(packageNames, removals); err != nil {
		return nil, err
	}
	if s.inDemoMode() {
		return s.demoSimulateInstall(packageNames), nil
	}
	vc := s.getVellumClient()
	if vc == nil {
		return &InstallSimulationResult{Packages: packageNames, Requested: packageNames}, nil
	}

	toSimulate := s.withPreferredProviders(packageNames, deviceType)

	start := time.Now()
	simulation, err := vc.SimulateAddWithout(toSimulate, removals)
	debug.Printf("[DEBUG] SimulateAdd: %d package(s) in %v\n", len(toSimulate), time.Since(start))
	if err != nil {
		debug.Printf("[DEBUG] SimulateAdd failed: %v, using packageNames only\n", err)
		failure := vellum.ResolutionFailure(err)
		result := &InstallSimulationResult{
			Packages:           packageNames,
			Requested:          packageNames,
			Error:              failure,
			UnresolvedVirtuals: vellum.ParseUnselectedVirtuals(failure),
		}
		if failure != "" && len(result.UnresolvedVirtuals) == 0 && s.metadata != nil && s.metadata.Ready() {
			firmware := ""
			if osState, osErr := vc.GetOSVersionState(); osErr == nil {
				firmware = osState.CurrentVersion
			}
			_, deviceArch := s.getConnectedDevice()
			arch := commands.GetDownloadArch(deviceArch)
			result.Conflicts = s.metadata.AttributeConflictsToQueue(
				vellum.ParseResolutionConflicts(failure), packageNames, deviceType, firmware, arch)
		}
		return result, nil
	}

	if len(simulation.Packages) == 0 {
		return &InstallSimulationResult{Packages: []string{}, Requested: packageNames}, nil
	}

	result := &InstallSimulationResult{Packages: simulation.Packages, Requested: packageNames}
	if len(simulation.VersionChanges) > 0 {
		result.Upgrades = make(map[string]UpgradeCandidate, len(simulation.VersionChanges))
		for _, change := range simulation.VersionChanges {
			result.Upgrades[change.Name] = UpgradeCandidate{From: change.From, To: change.To}
		}
	}
	return result, nil
}

func (s *PackagesService) withPreferredProviders(packageNames []string, deviceType string) []string {
	withProviders := packageNames
	for _, choice := range s.GetInstallChoices(packageNames, deviceType) {
		if provider := choice.PreferredProvider(); provider != "" {
			withProviders = append([]string{provider}, withProviders...)
		}
	}
	return withProviders
}

func (s *PackagesService) GetInstallChoices(packageNames []string, deviceType string) []vellum.VirtualChoice {
	if s.inDemoMode() {
		return s.demoInstallChoices(packageNames)
	}
	vc := s.getVellumClient()
	if s.metadata == nil || !s.metadata.Ready() || vc == nil {
		return []vellum.VirtualChoice{}
	}

	firmware := ""
	if osState, err := vc.GetOSVersionState(); err == nil {
		firmware = osState.CurrentVersion
	} else {
		debug.Printf("[DEBUG] GetInstallChoices: GetOSVersionState error: %v\n", err)
	}

	installed, err := vc.List()
	if err != nil {
		debug.Printf("[DEBUG] GetInstallChoices: List error: %v\n", err)
		return []vellum.VirtualChoice{}
	}

	_, deviceArch := s.getConnectedDevice()
	arch := commands.GetDownloadArch(deviceArch)
	choices := s.metadata.ResolveVirtualChoices(packageNames, installed, deviceType, firmware, arch)
	debug.Printf("[DEBUG] GetInstallChoices: %d choice(s) for %v\n", len(choices), packageNames)
	if choices == nil {
		return []vellum.VirtualChoice{}
	}
	return choices
}

func (s *PackagesService) GetPackageConflicts(installed map[string]string, queue []string, deviceType, firmware string) map[string]vellum.PackageConflict {
	if s.inDemoMode() {
		return map[string]vellum.PackageConflict{}
	}
	if s.metadata == nil || !s.metadata.Ready() {
		return map[string]vellum.PackageConflict{}
	}

	_, deviceArch := s.getConnectedDevice()
	arch := commands.GetDownloadArch(deviceArch)
	start := time.Now()
	conflicts := s.metadata.FindConflicts(installed, queue, deviceType, firmware, arch)
	debug.Printf("[DEBUG] GetPackageConflicts: %d conflict(s) for queue %v in %v\n", len(conflicts), queue, time.Since(start))
	return conflicts
}

// installs are packages the same plan adds; when one needs a package the removal would purge, the result asks to install first so the shared package is not purged and reinstalled.
func (s *PackagesService) SimulateUninstall(packageNames []string, installs []string, deviceType string) (*UninstallSimulationResult, error) {
	if err := checkPackageNames(packageNames, installs); err != nil {
		return nil, err
	}
	if s.inDemoMode() {
		return &UninstallSimulationResult{Packages: packageNames}, nil
	}
	s.mu.Lock()
	vc := s.vellumClient
	s.mu.Unlock()
	if vc == nil {
		return &UninstallSimulationResult{Packages: packageNames}, nil
	}
	removalAlone := &UninstallSimulationResult{Packages: s.resolveUninstall(vc, packageNames).allPackages}
	if len(installs) == 0 {
		return removalAlone, nil
	}

	installsWithProviders := s.withPreferredProviders(installs, deviceType)
	if _, err := vc.SimulateAdd(installsWithProviders...); err != nil {
		debug.Printf("[DEBUG] SimulateUninstall: install needs the removal first: %v\n", err)
		return removalAlone, nil
	}
	purged, err := vc.SimulateAddPurges(installsWithProviders, packageNames)
	if err != nil {
		debug.Printf("[DEBUG] SimulateUninstall: combined solve failed: %v\n", err)
		return removalAlone, nil
	}
	purgedSet := make(map[string]bool, len(purged))
	for _, name := range purged {
		purgedSet[name] = true
	}
	for _, name := range removalAlone.Packages {
		if !purgedSet[name] {
			return &UninstallSimulationResult{Packages: purged, InstallFirst: true}, nil
		}
	}
	return removalAlone, nil
}

type uninstallResolution struct {
	targets     []string
	allPackages []string
	recursive   bool
	batch       bool
	// Requested packages the removal would leave installed, because nothing in the world file pulls them in.
	stranded []string
}

func (s *PackagesService) resolveUninstall(vc *vellum.Client, packageNames []string) uninstallResolution {
	resolution := uninstallResolution{targets: packageNames}

	simResult, err := vc.SimulateDel(packageNames...)
	if err != nil {
		debug.Printf("[DEBUG] SimulateDel failed: %v, trying recursive\n", err)
		resolution.recursive = true
		resolution.allPackages, err = vc.SimulateDelRecursive(packageNames...)
		if err != nil {
			debug.Printf("[DEBUG] SimulateDelRecursive also failed: %v\n", err)
			resolution.allPackages = packageNames
		}
	} else if len(simResult.Blocked) > 0 || len(simResult.Packages) == 0 {
		worldToRemove, allAffected, stranded, wErr := s.resolveWorldDeps(vc, packageNames)
		if wErr != nil || len(worldToRemove) == 0 {
			debug.Printf("[DEBUG] resolveWorldDeps failed or empty: %v\n", wErr)
			resolution.recursive = true
			resolution.allPackages, err = vc.SimulateDelRecursive(packageNames...)
			if err != nil {
				debug.Printf("[DEBUG] SimulateDelRecursive failed: %v\n", err)
				resolution.allPackages = packageNames
			}
		} else {
			resolution.targets = worldToRemove
			resolution.allPackages = allAffected
			resolution.batch = true
			resolution.stranded = stranded
		}
	} else {
		resolution.allPackages = simResult.Packages
		if len(resolution.allPackages) == 0 {
			resolution.allPackages = packageNames
		}
	}
	return resolution
}

func (s *PackagesService) resolveWorldDeps(vc *vellum.Client, targets []string) (worldToRemove []string, allAffected []string, stranded []string, err error) {
	worldPkgs, err := vc.GetWorldPackages()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read world file: %w", err)
	}

	worldSet := make(map[string]bool, len(worldPkgs))
	for _, pkg := range worldPkgs {
		worldSet[pkg] = true
	}

	visited := make(map[string]bool)
	queue := make([]string, len(targets))
	copy(queue, targets)
	for _, t := range targets {
		visited[t] = true
	}

	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]

		rdeps, err := vc.GetReverseDeps(pkg)
		if err != nil {
			debug.Printf("[DEBUG] GetReverseDeps(%s) failed: %v\n", pkg, err)
			continue
		}

		for _, rdep := range rdeps {
			if !visited[rdep] {
				visited[rdep] = true
				queue = append(queue, rdep)
			}
		}
	}

	for pkg := range visited {
		allAffected = append(allAffected, pkg)
		if worldSet[pkg] {
			worldToRemove = append(worldToRemove, pkg)
		}
	}

	if len(worldToRemove) == 0 {
		return nil, nil, nil, nil
	}

	simResult, err := vc.SimulateDel(worldToRemove...)
	if err != nil {
		debug.Printf("[DEBUG] SimulateDel for world deps failed: %v\n", err)
		return worldToRemove, allAffected, nil, nil
	}

	removed := make(map[string]bool, len(simResult.Packages))
	for _, pkg := range simResult.Packages {
		removed[pkg] = true
		if !visited[pkg] {
			visited[pkg] = true
			allAffected = append(allAffected, pkg)
		}
	}
	for _, target := range targets {
		if !removed[target] {
			stranded = append(stranded, target)
		}
	}

	return worldToRemove, allAffected, stranded, nil
}

func (s *PackagesService) InstallPackages(opID string, requested []string, resolved []string, deviceType string) {
	if len(resolved) == 0 {
		resolved = requested
	}
	packageNames := requested
	if s.logger != nil && !s.inDemoMode() {
		s.logger.LogInstall("install", strings.Join(packageNames, ", "), "started")
	}
	s.runOperation(opID, "Installing mods", func(op *Operation) (any, error) {
		if err := checkPackageNames(requested, resolved); err != nil {
			return nil, err
		}
		if s.inDemoMode() {
			return installOutcome(InstallResult{Success: true}, s.demoInstall(op, resolved))
		}
		s.mu.Lock()
		vc := s.vellumClient
		arch := s.connectedDeviceArch
		s.mu.Unlock()
		if vc == nil {
			return nil, errNotConnected
		}

		dev := rmdevice.Type(deviceType)
		ctx := component.CommandContext{
			Arch:   arch,
			Device: dev,
		}

		settings, _ := s.settingsStore.Load()
		proxyEnabled := settings == nil || settings.ProxyMode

		s.mu.Lock()
		sshClient := s.client
		s.mu.Unlock()

		var allPackages []string
		if sshClient != nil && proxyEnabled {
			proxy := vellum.NewProxy(vc, sshClient, string(arch))
			op.Progress(InstallProgress{Status: InstallDownloading, Message: "Downloading packages through reManager…"})

			var err error
			allPackages, err = proxy.ProxyDownloadWithProgress(op.ctx, packageNames, func(progress vellum.ProxyProgress) {
				op.Progress(InstallProgress{
					Component: progress.Package,
					Index:     progress.Current - 1,
					Total:     progress.Total,
					Status:    InstallStatus(progress.Phase),
					Message:   progress.Message,
				})
			})
			if op.Canceled() {
				return nil, errOperationCanceled
			}
			if err != nil {
				return nil, fmt.Errorf("could not download through reManager: %w", err)
			}
		} else {
			allPackages = resolved
		}

		if op.Canceled() {
			return nil, errOperationCanceled
		}

		exec := &wailsExecutor{core: s.core, op: op}
		inst := installer.NewInstaller(vc, s.metadata, exec)

		before, beforeErr := vc.ListInstalledWithVersions()
		result := inst.Install(
			packageNames,
			allPackages,
			ctx,
			func(progress executor.ProgressInfo) {
				if s.logger != nil {
					s.logger.LogInstall("install", progress.CurrentComponent, progress.Message)
				}
				op.Progress(InstallProgress{
					Component: progress.CurrentComponent,
					Index:     progress.CurrentIndex,
					Total:     progress.TotalComponents,
					Status:    InstallStatus(progress.Status),
					Message:   progress.Message,
				})
			},
			func(hookResult *component.HookExecutionResult) error {
				return s.runHook(op, hookResult, exec)
			},
		)
		if op.Canceled() {
			reportPackageChangesAfterStop(op, vc, before, beforeErr)
		}

		return installOutcome(InstallResult{
			Success:  result.Success,
			Errors:   result.Errors,
			DNSError: result.DNSError,
		}, nil)
	})
}

func installOutcome(result InstallResult, err error) (any, error) {
	if err != nil {
		result.Success = false
		return result, err
	}
	if !result.Success {
		if len(result.Errors) == 0 {
			return result, errors.New("the operation did not complete")
		}
		return result, errors.New(strings.Join(result.Errors, "\n"))
	}
	return result, nil
}

// A stop can land after apk has committed, so a stopped run says what apk changed rather than letting the stop suggest nothing did.
func reportPackageChangesAfterStop(op *Operation, vc *vellum.Client, before map[string]string, beforeErr error) {
	after, err := vc.ListInstalledWithVersions()
	if beforeErr != nil || err != nil {
		op.Output("\nCould not check which packages changed before the stop.\n")
		return
	}
	op.Output(describePackageChanges(before, after))
}

func describePackageChanges(before, after map[string]string) string {
	var installed, upgraded, removed []string
	for name, version := range after {
		previous, had := before[name]
		switch {
		case !had:
			installed = append(installed, name+" "+version)
		case previous != version:
			upgraded = append(upgraded, fmt.Sprintf("%s %s to %s", name, previous, version))
		}
	}
	for name := range before {
		if _, kept := after[name]; !kept {
			removed = append(removed, name)
		}
	}
	if len(installed)+len(upgraded)+len(removed) == 0 {
		return "\nStopped before any package changed.\n"
	}
	var report strings.Builder
	report.WriteString("\nStopped, but apk had already made changes.\n")
	for _, change := range []struct {
		label    string
		packages []string
	}{{"Installed", installed}, {"Upgraded", upgraded}, {"Removed", removed}} {
		if len(change.packages) > 0 {
			slices.Sort(change.packages)
			fmt.Fprintf(&report, "%s: %s.\n", change.label, strings.Join(change.packages, ", "))
		}
	}
	return report.String()
}

func (s *PackagesService) UninstallPackages(opID string, packageNames []string, deviceType string) {
	if s.logger != nil && !s.inDemoMode() {
		s.logger.LogInstall("uninstall", strings.Join(packageNames, ", "), "started")
	}
	s.runOperation(opID, "Removing mods", func(op *Operation) (any, error) {
		if err := checkPackageNames(packageNames); err != nil {
			return nil, err
		}
		if s.inDemoMode() {
			return installOutcome(InstallResult{Success: true}, s.demoUninstall(op, packageNames))
		}
		s.mu.Lock()
		vc := s.vellumClient
		arch := s.connectedDeviceArch
		s.mu.Unlock()
		if vc == nil {
			return nil, errNotConnected
		}

		dev := rmdevice.Type(deviceType)
		ctx := component.CommandContext{
			Arch:   arch,
			Device: dev,
		}

		resolution := s.resolveUninstall(vc, packageNames)
		if len(resolution.stranded) > 0 {
			return nil, fmt.Errorf("Vellum would leave %s installed, so nothing was removed", strings.Join(resolution.stranded, ", "))
		}

		if op.Canceled() {
			return nil, errOperationCanceled
		}

		exec := &wailsExecutor{core: s.core, op: op}
		inst := installer.NewInstaller(vc, s.metadata, exec)

		before, beforeErr := vc.ListInstalledWithVersions()
		result := inst.Uninstall(
			resolution.targets,
			resolution.allPackages,
			resolution.recursive,
			resolution.batch,
			ctx,
			func(progress executor.ProgressInfo) {
				op.Progress(InstallProgress{
					Component: progress.CurrentComponent,
					Index:     progress.CurrentIndex,
					Total:     progress.TotalComponents,
					Status:    InstallStatus(progress.Status),
					Message:   progress.Message,
				})
			},
			func(hookResult *component.HookExecutionResult) error {
				return s.runHook(op, hookResult, exec)
			},
		)
		if op.Canceled() {
			reportPackageChangesAfterStop(op, vc, before, beforeErr)
		}

		return installOutcome(InstallResult{
			Success:  result.Success,
			Errors:   result.Errors,
			DNSError: result.DNSError,
		}, nil)
	})
}

func (s *PackagesService) RunMaintenanceCommand(opID, pkgName, commandID, deviceType string) {
	var cmd *vellum.MaintenanceCommand
	for _, candidate := range s.metadata.GetMaintenanceCommands(pkgName) {
		if candidate.ID == commandID {
			cmd = &candidate
			break
		}
	}
	title := pkgName
	if cmd != nil {
		title = cmp.Or(cmd.ProgressLabel, cmd.Label)
	}
	s.runOperation(opID, title, func(op *Operation) (any, error) {
		if cmd == nil {
			return nil, fmt.Errorf("command not found: %s %s", pkgName, commandID)
		}

		if cmd.Hook != "" {
			hookFunc := vellum.GetHook(cmd.Hook)
			if hookFunc == nil {
				return nil, fmt.Errorf("unknown hook: %s", cmd.Hook)
			}
			exec := &wailsExecutor{core: s.core, op: op}
			_, arch := s.getConnectedDevice()
			hookResult, err := hookFunc(component.CommandContext{
				Arch:   arch,
				Device: rmdevice.Type(deviceType),
				Exec:   exec,
			})
			if err != nil {
				return nil, fmt.Errorf("package script failed: %w", err)
			}
			if hookResult == nil {
				return nil, nil
			}
			return nil, s.runHook(op, hookResult, exec)
		}

		return nil, s.runOperationCommand(op, cmd.Command, cmd.RequiresTerminal)
	})
}

func (s *PackagesService) RunSystemTask(opID, taskID, deviceType string) {
	task := device.GetSystemTask(taskID)
	title := taskID
	if task != nil {
		title = task.Label
	}
	s.runOperation(opID, title, func(op *Operation) (any, error) {
		if task == nil {
			return nil, fmt.Errorf("task not found: %s", taskID)
		}

		dev := rmdevice.Type(deviceType)
		_, arch := s.getConnectedDevice()
		cmdResults := task.Command(component.CommandContext{
			Arch:   arch,
			Device: dev,
		})

		needsWritableRoot := s.os.taskNeedsWritableRoot(*task)
		if needsWritableRoot {
			if err := s.claimWritableRoot(dev); err != nil {
				return nil, err
			}
			defer s.releaseWritableRootClaim(dev)
		}

		var operationErr error
		if s.logger != nil {
			s.operationLog = s.logger.StartCommandLog(s.connectedDeviceID, taskID)
			defer func() {
				s.operationLog.WriteExitCode(operationErr)
				s.operationLog.Close()
				s.operationLog = nil
			}()
		}

		if needsWritableRoot {
			operationErr = s.runOperationCommandsWithWritableRoot(op, dev, cmdResults)
		} else {
			for _, c := range cmdResults {
				if operationErr = s.runOperationCommand(op, c.Script, c.RequiresPTY); operationErr != nil {
					break
				}
			}
		}
		if operationErr != nil {
			return nil, operationErr
		}
		// Recorded after the run, not before, so a demo failure leaves the switch where it was, as a real one would.
		if s.inDemoMode() {
			s.recordDemoSystemTask(taskID)
		}
		return nil, nil
	})
}

func (s *PackagesService) BootstrapVellum(opID string) {
	s.runOperation(opID, "Installing Vellum", func(op *Operation) (any, error) {
		if s.inDemoMode() {
			return nil, s.demoBootstrapVellum(op)
		}
		s.mu.Lock()
		sshClient := s.client
		if sshClient != nil && s.vellumClient == nil {
			s.vellumClient = vellum.NewClient(&wailsExecutor{core: s.core})
		}
		vc := s.vellumClient
		s.mu.Unlock()

		if sshClient == nil {
			return nil, errNotConnected
		}

		_, arch, _, err := s.detectDevice()
		if err != nil {
			return nil, fmt.Errorf("could not detect the reMarkable model: %w", err)
		}

		if err := vc.BootstrapOfflineWithPackages(op.ctx, sshClient, arch, op.Output); err != nil {
			return nil, err
		}
		s.recordVellumState(s.connGen.Load(), true, nil)
		return nil, nil
	})
}

func (s *PackagesService) UninstallVellum(opID string, removeAllPackages bool) {
	s.runOperation(opID, "Uninstalling Vellum", func(op *Operation) (any, error) {
		if s.inDemoMode() {
			return nil, s.demoUninstallVellum(op)
		}
		vc := s.getVellumClient()
		if vc == nil {
			return nil, errNotConnected
		}
		if err := vc.UninstallVellum(removeAllPackages, op.Output); err != nil {
			return nil, err
		}
		s.clearVellumClient(vc)
		s.recordVellumState(s.connGen.Load(), false, nil)
		return nil, nil
	})
}

func (s *PackagesService) CleanupBrokenVellum(opID string) {
	s.runOperation(opID, "Removing the broken Vellum install", func(op *Operation) (any, error) {
		vc := s.getVellumClient()
		if err := s.runOperationCommand(op, "rm -rf "+vellum.VellumRoot, false); err != nil {
			return nil, err
		}
		s.clearVellumClient(vc)
		s.recordVellumState(s.connGen.Load(), false, nil)
		return nil, nil
	})
}
