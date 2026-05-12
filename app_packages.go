package main

import (
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reManager/internal/component"
	"reManager/internal/debug"
	"reManager/internal/device"
	"reManager/internal/executor"
	"reManager/internal/httputil"
	"reManager/internal/installer"
	"reManager/internal/vellum"

	"reManager/internal/commands"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"
)

type PackageInfo struct {
	Name           string                `json:"name"`
	Version        string                `json:"version"`
	Description    string                `json:"description"`
	UpstreamAuthor string                `json:"upstreamAuthor"`
	Categories     []string              `json:"categories"`
	URL            string                `json:"url"`
	License        string                `json:"license"`
	Devices        []string              `json:"devices"`
	Depends        []string              `json:"depends"`
	Conflicts      []string              `json:"conflicts"`
	OSMin          *string               `json:"osMin"`
	OSMax          *string               `json:"osMax"`
	OSConstraints  []vellum.OSConstraint `json:"osConstraints"`
	Compatible         bool                  `json:"compatible"`
	IncompatibleReason string                `json:"incompatibleReason,omitempty"`
	Status             string                `json:"status"`
	DonateURL      *string               `json:"donateUrl"`
	ReadmeURL      *string               `json:"readmeUrl"`
}

type MaintenanceCommandInfo struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	Description      string `json:"description"`
	RequiresTerminal bool   `json:"requiresTerminal"`
	AllowStop        bool   `json:"allowStop"`
	Hook             string `json:"hook,omitempty"`
}

type SystemTaskInfo struct {
	ID                 string `json:"id"`
	Label              string `json:"label"`
	Description        string `json:"description"`
	RequiresTerminal   bool   `json:"requiresTerminal"`
	NeedsWriteableRoot bool   `json:"needsWriteableRoot"`
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

type InstallProgress struct {
	Component string `json:"component"`
	Index     int    `json:"index"`
	Total     int    `json:"total"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type InstallResult struct {
	Success  bool     `json:"success"`
	Errors   []string `json:"errors"`
	DNSError bool     `json:"dnsError"`
}

type InstallSimulationResult struct {
	Packages  []string `json:"packages"`
	Requested []string `json:"requested"`
}

type UninstallSimulationResult struct {
	Packages          []string            `json:"packages"`
	Blocked           map[string][]string `json:"blocked"`
	RecursivePackages []string            `json:"recursivePackages"`
	WorldDeps         []string            `json:"worldDeps"`
	AllAffected       []string            `json:"allAffected"`
}

var hiddenPackages = map[string]bool{
	"vellum":                 true,
	"vellum-bash-completion": true,
	"mount-utils":            true,
	"/bin/sh":                true,
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func (a *App) RefreshMetadata() error {
	return a.metadata.Refresh()
}

func (a *App) GetPackages(deviceType, firmware, arch string) []PackageInfo {
	debug.Printf("[DEBUG] GetPackages called, metadata=%p, deviceType=%s, firmware=%s, arch=%s\n", a.metadata, deviceType, firmware, arch)
	packages := a.metadata.GetAllPackagesForDevice(deviceType, firmware, arch)
	debug.Printf("[DEBUG] GetPackages got %d packages\n", len(packages))
	result := []PackageInfo{}

	for _, pkg := range packages {
		if hiddenPackages[pkg.Name] {
			continue
		}
		var visibleDepends []string
		for _, dep := range pkg.Depends {
			if !hiddenPackages[dep] {
				visibleDepends = append(visibleDepends, dep)
			}
		}
		result = append(result, PackageInfo{
			Name:           pkg.Name,
			Version:        pkg.Version,
			Description:    pkg.Description,
			UpstreamAuthor: pkg.UpstreamAuthor,
			Categories:     pkg.Categories,
			URL:            pkg.URL,
			License:        pkg.License,
			Devices:        pkg.Devices,
			Depends:        visibleDepends,
			Conflicts:      pkg.Conflicts,
			OSMin:          pkg.OSMin,
			OSMax:          pkg.OSMax,
			OSConstraints:  pkg.OSConstraints,
			Compatible:         pkg.Compatible,
			IncompatibleReason: pkg.IncompatibleReason,
			Status:             pkg.Status,
			DonateURL:      pkg.DonateURL,
			ReadmeURL:      pkg.ReadmeURL,
		})
	}

	debug.Printf("[DEBUG] GetPackages returning %d PackageInfo\n", len(result))
	return result
}

func (a *App) GetInstalledPackages() []string {
	if a.vellumClient == nil {
		return []string{}
	}

	packages, err := a.vellumClient.List()
	if err != nil {
		fmt.Printf("Error getting installed packages: %v\n", err)
		return []string{}
	}

	var result []string
	for _, pkg := range packages {
		if !hiddenPackages[pkg] {
			result = append(result, pkg)
		}
	}
	return result
}

func (a *App) GetInstalledPackagesWithOsCheck() InstalledPackagesResult {
	if a.vellumClient == nil {
		return InstalledPackagesResult{}
	}

	listResult, err := a.vellumClient.ListWithOsCheck()
	if err != nil {
		fmt.Printf("Error getting installed packages: %v\n", err)
		return InstalledPackagesResult{}
	}

	var packages []string
	for _, pkg := range listResult.Packages {
		if !hiddenPackages[pkg] {
			packages = append(packages, pkg)
		}
	}

	return InstalledPackagesResult{
		Packages:    packages,
		OsUpgraded:  listResult.OsUpgraded,
		PrevVersion: listResult.PrevVersion,
		NewVersion:  listResult.NewVersion,
	}
}

func (a *App) GetInstalledPackagesWithVersions() map[string]string {
	if a.vellumClient == nil {
		return map[string]string{}
	}

	versions, err := a.vellumClient.ListInstalledWithVersions()
	if err != nil {
		fmt.Printf("Error getting installed packages with versions: %v\n", err)
		return map[string]string{}
	}

	result := make(map[string]string)
	for pkg, version := range versions {
		if !hiddenPackages[pkg] {
			result[pkg] = version
		}
	}
	return result
}

func (a *App) GetPackageForOS(name, targetOS, deviceType, arch string) *PackageInfo {
	if a.metadata == nil {
		return nil
	}

	pkg := a.metadata.GetPackageForTargetOS(name, targetOS, deviceType, arch)
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
		OSMin:          pkg.OSMin,
		OSMax:          pkg.OSMax,
		OSConstraints:  pkg.OSConstraints,
		Status:         pkg.Status,
		DonateURL:      pkg.DonateURL,
		ReadmeURL:      pkg.ReadmeURL,
	}
}

func (a *App) GetReadmeContent(url string) (string, error) {
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

func (a *App) RunReenable() {
	if a.vellumClient == nil {
		return
	}

	if err := a.acquireWriteableRoot(a.connectedDeviceType); err != nil {
		runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Blocked: %v\n", err))
		return
	}
	defer a.releaseWriteableRoot(a.connectedDeviceType)

	runtime.EventsEmit(a.ctx, "command:output", "Running vellum reenable...\n")

	err := a.vellumClient.ReenableStreaming(func(line string) {
		runtime.EventsEmit(a.ctx, "command:output", line+"\n")
	})

	if err != nil {
		runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("\nError: %v\n", err))
	} else {
		runtime.EventsEmit(a.ctx, "command:output", "\nReenable completed successfully.\n")
	}
}

func (a *App) SimulatePackageUpgrade() (map[string]interface{}, error) {
	if a.vellumClient == nil {
		return nil, fmt.Errorf("not connected")
	}
	result, err := a.vellumClient.SimulateUpgrade()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"packages":    result.Packages,
		"hasUpgrades": result.HasUpgrades,
	}, nil
}

func (a *App) RunPackageUpgrade() {
	go func() {
		a.mu.Lock()
		vc := a.vellumClient
		sshClient := a.client
		arch := a.connectedDeviceArch
		a.mu.Unlock()
		if vc == nil {
			return
		}

		settings, _ := a.settingsStore.Load()
		proxyEnabled := settings == nil || settings.ProxyMode

		if proxyEnabled && sshClient != nil {
			proxy := vellum.NewProxy(vc, sshClient, string(arch))
			runtime.EventsEmit(a.ctx, "command:output", "Downloading upgrade packages via reManager...\n")
			err := proxy.ProxyUpgradeDownload(func(progress vellum.ProxyProgress) {
				runtime.EventsEmit(a.ctx, "command:output", progress.Message+"\n")
			})
			if err != nil {
				runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("\nProxy download failed: %v\n", err))
				runtime.EventsEmit(a.ctx, "package-upgrade:complete", map[string]interface{}{
					"success":  false,
					"dnsError": false,
				})
				return
			}
		}

		runtime.EventsEmit(a.ctx, "command:output", "Running vellum upgrade...\n")

		var dnsError bool
		err := vc.UpgradeStreaming(func(line string) {
			runtime.EventsEmit(a.ctx, "command:output", line+"\n")
			if strings.Contains(strings.ToLower(line), "dns:") {
				dnsError = true
			}
		})

		success := err == nil
		if success {
			runtime.EventsEmit(a.ctx, "command:output", "\nPackage upgrade completed.\n")
		} else {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("\nUpgrade error: %v\n", err))
		}
		runtime.EventsEmit(a.ctx, "package-upgrade:complete", map[string]interface{}{
			"success":  success,
			"dnsError": dnsError,
		})
	}()
}

func (a *App) GetOSVersionState() OSVersionStateResult {
	if a.vellumClient == nil {
		return OSVersionStateResult{}
	}

	state, err := a.vellumClient.GetOSVersionState()
	if err != nil {
		return OSVersionStateResult{}
	}

	return OSVersionStateResult{
		CurrentVersion: state.CurrentVersion,
		StoredVersion:  state.StoredVersion,
		Mismatch:       state.Mismatch,
	}
}

func (a *App) CheckOSCompatibility(targetOS string) CompatibilityResultJSON {
	if a.vellumClient == nil {
		return CompatibilityResultJSON{FetchFailed: true}
	}

	result, _ := a.vellumClient.CheckOSCompatibility(targetOS)
	if result == nil {
		return CompatibilityResultJSON{FetchFailed: true}
	}

	statusMap := make(map[string]string)
	for _, name := range append(append(result.Compatible, result.Incompatible...), result.NoConstraint...) {
		pkg := a.metadata.GetPackage(name)
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
	}
}

func (a *App) GetPackageCompatibilityStatus() PackageCompatibilityStatus {
	debug.Println("[DEBUG] GetPackageCompatibilityStatus: called")
	if a.vellumClient == nil {
		debug.Println("[DEBUG] GetPackageCompatibilityStatus: vellumClient is nil")
		return PackageCompatibilityStatus{FetchFailed: true}
	}

	osState, err := a.vellumClient.GetOSVersionState()
	if err != nil {
		debug.Printf("[DEBUG] GetPackageCompatibilityStatus: GetOSVersionState error: %v\n", err)
		return PackageCompatibilityStatus{FetchFailed: true}
	}
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: osState=%+v\n", osState)

	installed, err := a.vellumClient.List()
	if err != nil {
		debug.Printf("[DEBUG] GetPackageCompatibilityStatus: List error: %v\n", err)
		return PackageCompatibilityStatus{FetchFailed: true}
	}
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: installed=%v\n", installed)

	var filteredInstalled []string
	for _, pkg := range installed {
		if !hiddenPackages[pkg] {
			filteredInstalled = append(filteredInstalled, pkg)
		}
	}
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: filteredInstalled=%v\n", filteredInstalled)

	compat, err := a.vellumClient.CheckOSCompatibility(osState.CurrentVersion)
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
		pkg := a.metadata.GetPackage(name)
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

func (a *App) RunUpgrade() {
	go func() {
		a.mu.Lock()
		vc := a.vellumClient
		a.mu.Unlock()
		if vc == nil {
			return
		}

		osState, err := vc.GetOSVersionState()
		if err != nil {
			runtime.EventsEmit(a.ctx, "upgrade:error", "Failed to get OS version state")
			return
		}

		if osState.Mismatch {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Checking package compatibility with OS %s...\n", osState.CurrentVersion))

			compat, err := vc.CheckOSCompatibility(osState.CurrentVersion)
			if err != nil && compat.FetchFailed {
				runtime.EventsEmit(a.ctx, "upgrade:error", "Could not fetch package index to verify compatibility")
				return
			}

			if len(compat.Incompatible) > 0 {
				runtime.EventsEmit(a.ctx, "upgrade:blocked", CompatibilityResultJSON{
					Compatible:   compat.Compatible,
					Incompatible: compat.Incompatible,
					NoConstraint: compat.NoConstraint,
					FetchFailed:  compat.FetchFailed,
				})
				return
			}

			runtime.EventsEmit(a.ctx, "command:output", "All packages compatible. Proceeding with upgrade...\n\n")
		}

		settings, _ := a.settingsStore.Load()
		proxyEnabled := settings == nil || settings.ProxyMode

		a.mu.Lock()
		sshClient := a.client
		arch := a.connectedDeviceArch
		a.mu.Unlock()

		if proxyEnabled && sshClient != nil {
			proxy := vellum.NewProxy(vc, sshClient, string(arch))
			runtime.EventsEmit(a.ctx, "command:output", "Downloading upgrade packages via reManager...\n")
			proxyErr := proxy.ProxyUpgradeDownload(func(progress vellum.ProxyProgress) {
				runtime.EventsEmit(a.ctx, "command:output", progress.Message+"\n")
			})
			if proxyErr != nil {
				runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("\nProxy download failed: %v\n", proxyErr))
				runtime.EventsEmit(a.ctx, "upgrade:complete", map[string]interface{}{
					"success":  false,
					"dnsError": false,
				})
				return
			}
		}

		runtime.EventsEmit(a.ctx, "command:output", "Running vellum upgrade...\n")

		var dnsError bool
		err = vc.UpgradeStreaming(func(line string) {
			runtime.EventsEmit(a.ctx, "command:output", line+"\n")
			if strings.Contains(strings.ToLower(line), "dns:") {
				dnsError = true
			}
		})

		if err != nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("\nUpgrade error: %v\n", err))
			runtime.EventsEmit(a.ctx, "upgrade:complete", map[string]interface{}{
				"success":  false,
				"dnsError": dnsError,
			})
			return
		}

		runtime.EventsEmit(a.ctx, "command:output", "\nUpgrade completed successfully.\n")
		runtime.EventsEmit(a.ctx, "upgrade:complete", map[string]interface{}{
			"success":  true,
			"dnsError": dnsError,
		})
	}()
}

func (a *App) GetMaintenanceCommands(pkgName string) []MaintenanceCommandInfo {
	commands := a.metadata.GetMaintenanceCommands(pkgName)
	if commands == nil {
		return nil
	}

	result := make([]MaintenanceCommandInfo, len(commands))
	for i, cmd := range commands {
		result[i] = MaintenanceCommandInfo{
			ID:               cmd.ID,
			Label:            cmd.Label,
			Description:      cmd.Description,
			RequiresTerminal: cmd.RequiresTerminal,
			AllowStop:        cmd.AllowStop,
			Hook:             cmd.Hook,
		}
	}

	return result
}

func (a *App) GetAllMaintenanceCommands() map[string][]MaintenanceCommandInfo {
	result := make(map[string][]MaintenanceCommandInfo)
	for _, pkg := range a.metadata.GetAllPackages() {
		cmds := a.GetMaintenanceCommands(pkg.Name)
		if len(cmds) > 0 {
			result[pkg.Name] = cmds
		}
	}
	return result
}

func (a *App) GetSystemTasksInfo() []SystemTaskInfo {
	result := make([]SystemTaskInfo, len(device.SystemTasks))
	for i, task := range device.SystemTasks {
		result[i] = SystemTaskInfo{
			ID:                 task.ID,
			Label:              task.Label,
			Description:        task.Description,
			RequiresTerminal:   task.RequiresTerminal,
			NeedsWriteableRoot: task.NeedsWriteableRoot,
		}
	}
	return result
}

func (a *App) SimulateInstall(packageNames []string, deviceType string) (*InstallSimulationResult, error) {
	if a.vellumClient == nil {
		return &InstallSimulationResult{Packages: packageNames, Requested: packageNames}, nil
	}

	allPackages, err := a.vellumClient.SimulateAdd(packageNames...)
	if err != nil {
		debug.Printf("[DEBUG] SimulateAdd failed: %v, using packageNames only\n", err)
		return &InstallSimulationResult{Packages: packageNames, Requested: packageNames}, nil
	}

	if len(allPackages) == 0 {
		return &InstallSimulationResult{Packages: []string{}, Requested: packageNames}, nil
	}

	return &InstallSimulationResult{Packages: allPackages, Requested: packageNames}, nil
}

func (a *App) SimulateUninstall(packageNames []string) (*UninstallSimulationResult, error) {
	if a.vellumClient == nil {
		return &UninstallSimulationResult{Packages: packageNames}, nil
	}

	simResult, err := a.vellumClient.SimulateDel(packageNames...)
	if err != nil {
		debug.Printf("[DEBUG] SimulateDel failed: %v, trying recursive\n", err)
		recursiveList, rErr := a.vellumClient.SimulateDelRecursive(packageNames...)
		if rErr != nil {
			debug.Printf("[DEBUG] SimulateDelRecursive also failed: %v\n", rErr)
			return &UninstallSimulationResult{Packages: packageNames}, nil
		}
		return &UninstallSimulationResult{
			Packages:          packageNames,
			RecursivePackages: recursiveList,
		}, nil
	}

	packages := simResult.Packages
	if packages == nil {
		packages = []string{}
	}

	result := &UninstallSimulationResult{
		Packages: packages,
		Blocked:  simResult.Blocked,
	}

	if len(simResult.Blocked) > 0 {
		recursiveList, err := a.vellumClient.SimulateDelRecursive(packageNames...)
		if err != nil {
			debug.Printf("[DEBUG] SimulateDelRecursive failed: %v\n", err)
		} else if len(recursiveList) > 0 {
			result.RecursivePackages = recursiveList
		}

		if len(result.RecursivePackages) == 0 {
			seen := make(map[string]bool)
			for _, name := range packageNames {
				seen[name] = true
			}
			for _, dependents := range simResult.Blocked {
				for _, dep := range dependents {
					seen[dep] = true
				}
			}
			all := make([]string, 0, len(seen))
			for name := range seen {
				all = append(all, name)
			}
			result.RecursivePackages = all
		}
	}

	if a.vellumClient != nil && (len(result.Packages) == 0 || len(result.Blocked) > 0) {
		worldToRemove, allAffected, err := a.resolveWorldDeps(packageNames)
		if err != nil {
			debug.Printf("[DEBUG] resolveWorldDeps failed: %v\n", err)
		} else if len(worldToRemove) > 0 {
			result.WorldDeps = worldToRemove
			result.AllAffected = allAffected
		}
	}

	return result, nil
}

func (a *App) resolveWorldDeps(targets []string) (worldToRemove []string, allAffected []string, err error) {
	worldPkgs, err := a.vellumClient.GetWorldPackages()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read world file: %w", err)
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

		rdeps, err := a.vellumClient.GetReverseDeps(pkg)
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
		return nil, nil, nil
	}

	simResult, err := a.vellumClient.SimulateDel(worldToRemove...)
	if err != nil {
		debug.Printf("[DEBUG] SimulateDel for world deps failed: %v\n", err)
		return worldToRemove, allAffected, nil
	}

	for _, pkg := range simResult.Packages {
		if !visited[pkg] {
			visited[pkg] = true
			allAffected = append(allAffected, pkg)
		}
	}

	return worldToRemove, allAffected, nil
}

func (a *App) InstallPackages(packageNames []string, deviceType string) {
	if a.logger != nil {
		a.logger.LogInstall("install", strings.Join(packageNames, ", "), "started")
	}
	go func() {
		a.mu.Lock()
		vc := a.vellumClient
		a.installCancelCh = make(chan struct{})
		cancelCh := a.installCancelCh
		a.mu.Unlock()
		if vc == nil {
			runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
				Success: false,
				Errors:  []string{"Not connected"},
			})
			return
		}

		defer func() {
			a.mu.Lock()
			if a.installCancelCh == cancelCh {
				a.installCancelCh = nil
			}
			a.mu.Unlock()
		}()

		isCancelled := func() bool {
			select {
			case <-cancelCh:
				return true
			default:
				return false
			}
		}

		a.dialogResponse = make(chan string, 1)
		defer func() {
			close(a.dialogResponse)
			a.dialogResponse = nil
		}()

		dev := rmdevice.Type(deviceType)
		arch := a.connectedDeviceArch

		ctx := component.CommandContext{
			Arch:   arch,
			Device: dev,
		}

		settings, _ := a.settingsStore.Load()
		proxyEnabled := settings == nil || settings.ProxyMode

		a.mu.Lock()
		sshClient := a.client
		a.mu.Unlock()

		var allPackages []string
		if sshClient != nil && proxyEnabled {
			proxy := vellum.NewProxy(vc, sshClient, string(arch))
			runtime.EventsEmit(a.ctx, "install:progress", InstallProgress{
				Status:  "downloading",
				Message: "Downloading packages via reManager...",
			})

			var err error
			allPackages, err = proxy.ProxyDownloadWithProgress(packageNames, func(progress vellum.ProxyProgress) {
				runtime.EventsEmit(a.ctx, "install:progress", InstallProgress{
					Component: progress.Package,
					Index:     progress.Current - 1,
					Total:     progress.Total,
					Status:    progress.Phase,
					Message:   progress.Message,
				})
			})
			if err != nil {
				runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
					Success: false,
					Errors:  []string{fmt.Sprintf("Proxy download failed: %v", err)},
				})
				return
			}
		} else {
			allPackages = packageNames
		}

		if isCancelled() {
			runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
				Success: false,
				Errors:  []string{"Installation cancelled"},
			})
			return
		}

		exec := &wailsExecutor{app: a}
		inst := installer.NewInstaller(vc, a.metadata, exec)

		result := inst.Install(
			packageNames,
			allPackages,
			ctx,
			func(progress executor.ProgressInfo) {
				if a.logger != nil {
					a.logger.LogInstall("install", progress.CurrentComponent, progress.Message)
				}
				runtime.EventsEmit(a.ctx, "install:progress", InstallProgress{
					Component: progress.CurrentComponent,
					Index:     progress.CurrentIndex,
					Total:     progress.TotalComponents,
					Status:    string(progress.Status),
					Message:   progress.Message,
				})
			},
			func(hookResult *component.HookExecutionResult) error {
				if hookResult.DialogConfig != nil {
					runtime.EventsEmit(a.ctx, "hook:dialog", dialogRequestFromConfig(hookResult.DialogConfig))

					response := <-a.dialogResponse
					if response == "" || response == "cancel" {
						return fmt.Errorf("user cancelled")
					}

					runtime.EventsEmit(a.ctx, "hook:started", map[string]string{
						"title": hookResult.DialogConfig.Title,
					})

					if hookResult.Command != nil {
						runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", hookResult.Command.Script))
						if err := exec.Execute([]component.CommandResult{*hookResult.Command}); err != nil {
							return err
						}
					}

					if hookResult.DialogConfig.PostCommandDialog != nil {
						runtime.EventsEmit(a.ctx, "hook:dialog", dialogRequestFromConfig(hookResult.DialogConfig.PostCommandDialog))

						postResponse := <-a.dialogResponse
						for _, action := range hookResult.DialogConfig.PostCommandDialog.Actions {
							if action.Id == postResponse && action.Type == "run_command" {
								runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", action.Value))
								if err := exec.Execute([]component.CommandResult{{Script: action.Value}}); err != nil {
									return err
								}
								break
							}
						}
					}
				}
				return nil
			},
		)

		runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
			Success:  result.Success,
			Errors:   result.Errors,
			DNSError: result.DNSError,
		})
	}()
}

func (a *App) UninstallPackages(packageNames []string, deviceType string) {
	if a.logger != nil {
		a.logger.LogInstall("uninstall", strings.Join(packageNames, ", "), "started")
	}
	go func() {
		a.mu.Lock()
		vc := a.vellumClient
		a.installCancelCh = make(chan struct{})
		cancelCh := a.installCancelCh
		a.mu.Unlock()
		if vc == nil {
			runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
				Success: false,
				Errors:  []string{"Not connected"},
			})
			return
		}

		defer func() {
			a.mu.Lock()
			if a.installCancelCh == cancelCh {
				a.installCancelCh = nil
			}
			a.mu.Unlock()
		}()

		isCancelled := func() bool {
			select {
			case <-cancelCh:
				return true
			default:
				return false
			}
		}

		a.dialogResponse = make(chan string, 1)
		defer func() {
			close(a.dialogResponse)
			a.dialogResponse = nil
		}()

		dev := rmdevice.Type(deviceType)
		arch := a.connectedDeviceArch

		ctx := component.CommandContext{
			Arch:   arch,
			Device: dev,
		}

		var allPackages []string
		useRecursive := false
		useBatch := false

		simResult, err := vc.SimulateDel(packageNames...)
		if err != nil {
			debug.Printf("[DEBUG] SimulateDel failed: %v, trying recursive\n", err)
			useRecursive = true
			allPackages, err = vc.SimulateDelRecursive(packageNames...)
			if err != nil {
				debug.Printf("[DEBUG] SimulateDelRecursive also failed: %v\n", err)
				allPackages = packageNames
			}
		} else if len(simResult.Blocked) > 0 || len(simResult.Packages) == 0 {
			worldToRemove, allAffected, wErr := a.resolveWorldDeps(packageNames)
			if wErr != nil || len(worldToRemove) == 0 {
				debug.Printf("[DEBUG] resolveWorldDeps failed or empty: %v\n", wErr)
				useRecursive = true
				allPackages, err = vc.SimulateDelRecursive(packageNames...)
				if err != nil {
					debug.Printf("[DEBUG] SimulateDelRecursive failed: %v\n", err)
					allPackages = packageNames
				}
			} else {
				packageNames = worldToRemove
				allPackages = allAffected
				useBatch = true
			}
		} else {
			allPackages = simResult.Packages
			if len(allPackages) == 0 {
				allPackages = packageNames
			}
		}

		if isCancelled() {
			runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
				Success: false,
				Errors:  []string{"Uninstall cancelled"},
			})
			return
		}

		exec := &wailsExecutor{app: a}
		inst := installer.NewInstaller(vc, a.metadata, exec)

		result := inst.Uninstall(
			packageNames,
			allPackages,
			useRecursive,
			useBatch,
			ctx,
			func(progress executor.ProgressInfo) {
				runtime.EventsEmit(a.ctx, "install:progress", InstallProgress{
					Component: progress.CurrentComponent,
					Index:     progress.CurrentIndex,
					Total:     progress.TotalComponents,
					Status:    string(progress.Status),
					Message:   progress.Message,
				})
			},
			func(hookResult *component.HookExecutionResult) error {
				if hookResult.DialogConfig != nil {
					runtime.EventsEmit(a.ctx, "hook:dialog", dialogRequestFromConfig(hookResult.DialogConfig))

					response := <-a.dialogResponse
					if response == "" || response == "cancel" {
						return fmt.Errorf("user cancelled")
					}

					if hookResult.Command != nil {
						runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", hookResult.Command.Script))
						if err := exec.Execute([]component.CommandResult{*hookResult.Command}); err != nil {
							return err
						}
					}

					if hookResult.DialogConfig.PostCommandDialog != nil {
						runtime.EventsEmit(a.ctx, "hook:dialog", dialogRequestFromConfig(hookResult.DialogConfig.PostCommandDialog))

						postResponse := <-a.dialogResponse
						for _, action := range hookResult.DialogConfig.PostCommandDialog.Actions {
							if action.Id == postResponse && action.Type == "run_command" {
								runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", action.Value))
								if err := exec.Execute([]component.CommandResult{{Script: action.Value}}); err != nil {
									return err
								}
								break
							}
						}
					}
				}
				return nil
			},
		)

		runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
			Success:  result.Success,
			Errors:   result.Errors,
			DNSError: result.DNSError,
		})
	}()
}

func (a *App) RunMaintenanceCommand(pkgName, commandID, deviceType string) {
	go func() {
		commands := a.metadata.GetMaintenanceCommands(pkgName)
		if commands == nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("No maintenance commands for package: %s\n", pkgName))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		var cmd *vellum.MaintenanceCommand
		for i := range commands {
			if commands[i].ID == commandID {
				cmd = &commands[i]
				break
			}
		}
		if cmd == nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Command not found: %s\n", commandID))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		if cmd.Hook != "" {
			hookFunc := vellum.GetHook(cmd.Hook)
			if hookFunc != nil {
				dev := rmdevice.Type(deviceType)
				ctx := component.CommandContext{
					Arch:   a.connectedDeviceArch,
					Device: dev,
				}

				hookResult, err := hookFunc(ctx)
				if err != nil {
					runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Hook error: %v\n", err))
					runtime.EventsEmit(a.ctx, "command:done", false)
					return
				}

				if hookResult != nil && hookResult.DialogConfig != nil {
					a.dialogResponse = make(chan string, 1)
					defer func() {
						close(a.dialogResponse)
						a.dialogResponse = nil
					}()

					runtime.EventsEmit(a.ctx, "hook:dialog", dialogRequestFromConfig(hookResult.DialogConfig))

					response := <-a.dialogResponse
					if response == "" || response == "cancel" {
						runtime.EventsEmit(a.ctx, "command:done", false)
						return
					}

					runtime.EventsEmit(a.ctx, "hook:started", map[string]string{
						"title": hookResult.DialogConfig.Title,
					})

					exec := &wailsExecutor{app: a}

					if hookResult.Command != nil {
						runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", hookResult.Command.Script))
						if err := exec.Execute([]component.CommandResult{*hookResult.Command}); err != nil {
							runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
							runtime.EventsEmit(a.ctx, "command:done", false)
							return
						}
					}

					if hookResult.DialogConfig.PostCommandDialog != nil {
						runtime.EventsEmit(a.ctx, "hook:dialog", dialogRequestFromConfig(hookResult.DialogConfig.PostCommandDialog))

						postResponse := <-a.dialogResponse
						for _, action := range hookResult.DialogConfig.PostCommandDialog.Actions {
							if action.Id == postResponse && action.Type == "run_command" {
								runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", action.Value))
								if err := exec.Execute([]component.CommandResult{{Script: action.Value}}); err != nil {
									runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
								}
								break
							}
						}
					}

					runtime.EventsEmit(a.ctx, "command:done", true)
					return
				}
			}
		}

		runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", cmd.Command))
		a.RunCommandWithOutput(cmd.Command, cmd.RequiresTerminal)
	}()
}

func (a *App) RunSystemTask(taskID, deviceType string) {
	go func() {
		task := device.GetSystemTask(taskID)
		if task == nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Task not found: %s\n", taskID))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		dev := rmdevice.Type(deviceType)
		ctx := component.CommandContext{
			Arch:   a.connectedDeviceArch,
			Device: dev,
		}

		cmdResults := task.Command(ctx)

		if task.NeedsWriteableRoot {
			cmdResults = commands.WrapWithWriteableRoot(cmdResults, dev)
			if err := a.acquireWriteableRoot(dev); err != nil {
				runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Blocked: %v\n", err))
				runtime.EventsEmit(a.ctx, "command:done", false)
				return
			}
			defer a.releaseWriteableRoot(dev)
		}

		var operationErr error
		if a.logger != nil {
			a.operationLog = a.logger.StartCommandLog(a.connectedDeviceID, taskID)
			defer func() {
				a.operationLog.WriteExitCode(operationErr)
				a.operationLog.Close()
				a.operationLog = nil
			}()
		}

		for _, c := range cmdResults {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", c.Script))

			done := make(chan bool, 1)
			unsub := runtime.EventsOn(a.ctx, "command:done", func(optionalData ...interface{}) {
				if len(optionalData) > 0 {
					if success, ok := optionalData[0].(bool); ok {
						done <- success
						return
					}
				}
				done <- false
			})

			a.RunCommandWithOutput(c.Script, c.RequiresPTY)
			success := <-done
			unsub()

			if !success {
				operationErr = fmt.Errorf("command failed: %s", c.Script)
				runtime.EventsEmit(a.ctx, "command:output", "Command failed, stopping execution\n")
				return
			}
		}
		runtime.EventsEmit(a.ctx, "systemtask:complete", true)
	}()
}
