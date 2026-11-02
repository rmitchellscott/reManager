package vellum

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"reManager/internal/debug"
	"reManager/internal/httputil"
	versionpkg "reManager/internal/version"
)

const (
	PackagesMetadataURL  = "https://packages.vellum.delivery/packages-metadata.json"
	RemanagerMetadataURL = "https://remanager.io/remanager-metadata.json"
	PackagesStatsURL     = "https://packages.vellum.delivery/stats/stats-overview.json"
	MetadataTimeout      = 10 * time.Second
)

//go:embed fallback_remanager.json
var fallbackRemanagerJSON []byte

type OSConstraint struct {
	Version  string `json:"version"`
	Operator string `json:"operator"`
}

type PackageVersion struct {
	Pkgdesc        string         `json:"pkgdesc"`
	UpstreamAuthor string         `json:"upstream_author"`
	Categories     []string       `json:"categories"`
	License        string         `json:"license"`
	URL            string         `json:"url"`
	OSMin          *string        `json:"os_min"`
	OSMax          *string        `json:"os_max"`
	OSConstraints  []OSConstraint `json:"os_constraints"`
	Devices        []string       `json:"devices"`
	Depends        []string       `json:"depends"`
	Conflicts      []string       `json:"conflicts"`
	Provides       []string       `json:"provides"`
	Arch           []string       `json:"arch"`
	Status         string         `json:"status"`
	DonateURL      *string        `json:"donateurl"`
	ReadmeURL      *string        `json:"readmeurl"`
	ChangelogURL   *string        `json:"changelogurl"`
	Released       string         `json:"released"`
	FirstReleased  string         `json:"first_released"`
}

type PackagesMetadata struct {
	Generated string                               `json:"generated"`
	Packages  map[string]map[string]PackageVersion `json:"packages"`
}

// ShowOutput marks a command run for its output, not a log of it running; ExpectedSeconds is a progress estimate, not a deadline.
type MaintenanceCommand struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	ProgressLabel    string `json:"progressLabel,omitempty"`
	Description      string `json:"description,omitempty"`
	Command          string `json:"command"`
	RequiresTerminal bool   `json:"requiresTerminal,omitempty"`
	ShowOutput       bool   `json:"showOutput,omitempty"`
	ExpectedSeconds  int    `json:"expectedSeconds,omitempty"`
	AllowStop        bool   `json:"allowStop,omitempty"`
	Hook             string `json:"hook,omitempty"`
}

type PackageHooks struct {
	PostInstall   string `json:"postInstall,omitempty"`
	PreUninstall  string `json:"preUninstall,omitempty"`
	PostUninstall string `json:"postUninstall,omitempty"`
}

type RemanagerPackageInfo struct {
	Hooks *PackageHooks `json:"hooks,omitempty"`
}

type VirtualConfig struct {
	Default string `json:"default,omitempty"`
}

type RemanagerMetadata struct {
	Packages map[string]RemanagerPackageInfo `json:"packages"`
	Virtuals map[string]VirtualConfig        `json:"virtuals,omitempty"`
}

type PackageStats struct {
	Name           string         `json:"name"`
	Total          int            `json:"total"`
	TotalDownloads map[string]int `json:"total_downloads"`
	IsBootstrap    bool           `json:"is_bootstrap"`
	IsFramework    bool           `json:"is_framework"`
}

type PackagesStatsData struct {
	Packages []PackageStats `json:"packages"`
}

type Package struct {
	Name               string
	Version            string
	Description        string
	UpstreamAuthor     string
	Categories         []string
	License            string
	URL                string
	OSMin              *string
	OSMax              *string
	OSConstraints      []OSConstraint
	Devices            []string
	Depends            []string
	Conflicts          []string
	Provides           []string
	Arch               []string
	Status             string
	DonateURL          *string
	ReadmeURL          *string
	ChangelogURL       *string
	Released           string
	FirstReleased      string
	Downloads30d       int
	DownloadsAll       int
	IsFramework        bool
	Compatible         bool
	IncompatibleReason string
}

var deviceAliases = map[string]string{
	"rmppm": "rmppmove",
}

func normalizeDevices(devices []string) []string {
	seen := make(map[string]bool, len(devices))
	out := make([]string, 0, len(devices))
	for _, d := range devices {
		if alias, ok := deviceAliases[d]; ok {
			d = alias
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func normalizePackagesMetadata(pm *PackagesMetadata) {
	for _, versions := range pm.Packages {
		for ver, info := range versions {
			info.Devices = normalizeDevices(info.Devices)
			versions[ver] = info
		}
	}
}

type MetadataStore struct {
	mu        sync.RWMutex
	packages  PackagesMetadata
	remanager RemanagerMetadata
	providers map[string][]string
	stats     map[string]PackageStats
	commands  map[string][]MaintenanceCommand
	loaded    bool
	err       error
}

func buildProviderIndex(pm *PackagesMetadata) map[string][]string {
	index := make(map[string][]string)
	for name, versions := range pm.Packages {
		seen := make(map[string]bool)
		for _, info := range versions {
			for _, provided := range info.Provides {
				provided = stripDepVersion(provided)
				if provided == "" || seen[provided] {
					continue
				}
				if _, isReal := pm.Packages[provided]; isReal {
					continue
				}
				seen[provided] = true
				index[provided] = append(index[provided], name)
			}
		}
	}
	for _, names := range index {
		sort.Strings(names)
	}
	return index
}

func NewMetadataStore() *MetadataStore {
	m := &MetadataStore{commands: maintenanceCommands}
	if err := json.Unmarshal(fallbackRemanagerJSON, &m.remanager); err != nil {
		debug.Printf("[DEBUG] Failed to parse embedded fallback remanager metadata: %v\n", err)
	}
	return m
}

func (m *MetadataStore) Load() error {
	if err := m.fetchAndSwap(); err != nil {
		m.mu.Lock()
		m.err = err
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *MetadataStore) Ready() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loaded
}

func (m *MetadataStore) Err() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.err
}

// A failed refresh keeps the catalog already loaded.
func (m *MetadataStore) Refresh() error {
	return m.fetchAndSwap()
}

// Everything is fetched into locals and swapped in under the lock, so a reader never sees a half-loaded catalog and a malformed reManager file leaves the embedded fallback in place.
func (m *MetadataStore) fetchAndSwap() error {
	packages, err := fetchPackagesMetadata()
	if err != nil {
		return fmt.Errorf("failed to fetch packages metadata: %w", err)
	}
	normalizePackagesMetadata(&packages)
	providers := buildProviderIndex(&packages)

	remanager, remanagerErr := fetchRemanagerMetadata()
	if remanagerErr != nil {
		debug.Printf("[DEBUG] Remanager metadata fetch failed (non-fatal): %v\n", remanagerErr)
	}

	stats, statsErr := m.fetchStats()
	if statsErr != nil {
		debug.Printf("[DEBUG] Stats fetch failed (non-fatal): %v\n", statsErr)
	}

	m.mu.Lock()
	m.packages = packages
	m.providers = providers
	if remanagerErr == nil && len(remanager.Packages) > 0 {
		m.remanager = remanager
	}
	if stats != nil {
		m.stats = stats
	}
	m.loaded = true
	m.err = nil
	remanagerCount := len(m.remanager.Packages)
	m.mu.Unlock()

	debug.Printf("[DEBUG] Loaded metadata: %d packages, %d remanager configs\n", len(packages.Packages), remanagerCount)
	return nil
}

func fetchPackagesMetadata() (PackagesMetadata, error) {
	var packages PackagesMetadata
	body, err := fetchMetadataFile(PackagesMetadataURL)
	if err != nil {
		return packages, err
	}
	err = json.Unmarshal(body, &packages)
	return packages, err
}

func fetchRemanagerMetadata() (RemanagerMetadata, error) {
	var remanager RemanagerMetadata
	body, err := fetchMetadataFile(RemanagerMetadataURL)
	if err != nil {
		return remanager, err
	}
	err = json.Unmarshal(body, &remanager)
	return remanager, err
}

func fetchMetadataFile(url string) ([]byte, error) {
	client := httputil.NewClient(MetadataTimeout)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (m *MetadataStore) fetchStats() (map[string]PackageStats, error) {
	client := httputil.NewClient(MetadataTimeout)
	resp, err := client.Get(PackagesStatsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stats HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data PackagesStatsData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	result := make(map[string]PackageStats, len(data.Packages))
	for _, p := range data.Packages {
		result[p.Name] = p
	}
	return result, nil
}

func (m *MetadataStore) getStats(name string) (dl30d int, dlAll int, isFramework bool) {
	if s, ok := m.stats[name]; ok {
		return s.TotalDownloads["30d"], s.TotalDownloads["all"], s.IsFramework
	}
	return 0, 0, false
}

func (m *MetadataStore) GetAllPackages() []Package {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var packages []Package

	for name, versions := range m.packages.Packages {
		var latestVersion string
		var latestInfo PackageVersion
		for version, info := range versions {
			if latestVersion == "" || versionpkg.Compare(version, latestVersion) > 0 {
				latestVersion = version
				latestInfo = info
			}
		}

		pkg := Package{
			Name:           name,
			Version:        latestVersion,
			Description:    latestInfo.Pkgdesc,
			UpstreamAuthor: latestInfo.UpstreamAuthor,
			Categories:     latestInfo.Categories,
			License:        latestInfo.License,
			URL:            latestInfo.URL,
			OSMin:          latestInfo.OSMin,
			OSMax:          latestInfo.OSMax,
			OSConstraints:  latestInfo.OSConstraints,
			Devices:        latestInfo.Devices,
			Depends:        latestInfo.Depends,
			Conflicts:      latestInfo.Conflicts,
			Provides:       latestInfo.Provides,
			Arch:           latestInfo.Arch,
			Status:         latestInfo.Status,
			DonateURL:      latestInfo.DonateURL,
			ReadmeURL:      latestInfo.ReadmeURL,
			ChangelogURL:   latestInfo.ChangelogURL,
			Released:       latestInfo.Released,
			FirstReleased:  latestInfo.FirstReleased,
		}

		dl30, dlAll, framework := m.getStats(name)
		pkg.Downloads30d = dl30
		pkg.DownloadsAll = dlAll
		pkg.IsFramework = framework

		packages = append(packages, pkg)
	}

	return packages
}

func (m *MetadataStore) GetPackage(name string) *Package {
	m.mu.RLock()
	defer m.mu.RUnlock()

	versions, ok := m.packages.Packages[name]
	if !ok {
		return nil
	}

	var latestVersion string
	var latestInfo PackageVersion
	for version, info := range versions {
		if latestVersion == "" || versionpkg.Compare(version, latestVersion) > 0 {
			latestVersion = version
			latestInfo = info
		}
	}

	pkg := &Package{
		Name:           name,
		Version:        latestVersion,
		Description:    latestInfo.Pkgdesc,
		UpstreamAuthor: latestInfo.UpstreamAuthor,
		Categories:     latestInfo.Categories,
		License:        latestInfo.License,
		URL:            latestInfo.URL,
		OSMin:          latestInfo.OSMin,
		OSMax:          latestInfo.OSMax,
		OSConstraints:  latestInfo.OSConstraints,
		Devices:        latestInfo.Devices,
		Depends:        latestInfo.Depends,
		Conflicts:      latestInfo.Conflicts,
		Provides:       latestInfo.Provides,
		Arch:           latestInfo.Arch,
		Status:         latestInfo.Status,
		DonateURL:      latestInfo.DonateURL,
		ReadmeURL:      latestInfo.ReadmeURL,
		ChangelogURL:   latestInfo.ChangelogURL,
		Released:       latestInfo.Released,
		FirstReleased:  latestInfo.FirstReleased,
	}

	dl30, dlAll, framework := m.getStats(name)
	pkg.Downloads30d = dl30
	pkg.DownloadsAll = dlAll
	pkg.IsFramework = framework

	return pkg
}

// The index stores at most one pkgrel per pkgver, so a rel bump replaces a row rather than adding one.
type PackageRelease struct {
	Version       string         `json:"version"`
	Released      string         `json:"released"`
	OSMin         *string        `json:"osMin"`
	OSMax         *string        `json:"osMax"`
	OSConstraints []OSConstraint `json:"osConstraints"`
	Depends       []string       `json:"depends"`
	Arch          []string       `json:"arch"`
	Devices       []string       `json:"devices"`
	ChangelogURL  *string        `json:"changelogUrl"`
	// False when the attached reMarkable cannot install this version; with no device attached every version is compatible.
	Compatible bool `json:"compatible"`
}

// Ordered by version, not release date: a maintenance release on an older branch can ship after a newer one. Not filtered by compatibility; each release carries its own verdict.
func (m *MetadataStore) GetPackageReleases(name, deviceType, firmware, arch string) []PackageRelease {
	m.mu.RLock()
	defer m.mu.RUnlock()

	versions, ok := m.packages.Packages[name]
	if !ok {
		return nil
	}

	releases := make([]PackageRelease, 0, len(versions))
	for ver, info := range versions {
		releases = append(releases, PackageRelease{
			Version:       ver,
			Released:      info.Released,
			OSMin:         info.OSMin,
			OSMax:         info.OSMax,
			OSConstraints: info.OSConstraints,
			Depends:       info.Depends,
			Arch:          info.Arch,
			Devices:       info.Devices,
			ChangelogURL:  info.ChangelogURL,
			Compatible:    isVersionCompatible(info, deviceType, firmware, arch),
		})
	}

	sort.Slice(releases, func(i, j int) bool {
		if cmp := versionpkg.Compare(releases[i].Version, releases[j].Version); cmp != 0 {
			return cmp > 0
		}
		return releases[i].Version > releases[j].Version
	})
	return releases
}

// Catalog diff for a disconnected device, not vellum's solver: the solver can block an upgrade this count offers.
func (m *MetadataStore) UpgradesFor(installed map[string]string, deviceType, firmware, arch string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for name, version := range installed {
		best, _ := m.bestVersion(name, deviceType, firmware, arch)
		if best != "" && best != version {
			count++
		}
	}
	return count
}

// Installed packages with no version compatible with targetOS. A package absent from the index blocks nothing: its constraints are unknown, and treating unknown as a blocker would hide a usable update.
func (m *MetadataStore) PackagesBlockingOS(installed map[string]string, deviceType, targetOS, arch string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var blocking []string
	for name := range installed {
		versions, ok := m.packages.Packages[name]
		if !ok {
			continue
		}
		supported := false
		for _, info := range versions {
			if isVersionCompatible(info, deviceType, targetOS, arch) {
				supported = true
				break
			}
		}
		if !supported {
			blocking = append(blocking, name)
		}
	}
	sort.Strings(blocking)
	return blocking
}

func (m *MetadataStore) GetMaintenanceCommands(name string) []MaintenanceCommand {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.commands[name]
}

func (m *MetadataStore) AllMaintenanceCommands() map[string][]MaintenanceCommand {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]MaintenanceCommand, len(m.commands))
	for name, commands := range m.commands {
		if len(commands) > 0 {
			result[name] = commands
		}
	}
	return result
}

func (m *MetadataStore) GetHooks(name string) *PackageHooks {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if rmInfo, ok := m.remanager.Packages[name]; ok {
		return rmInfo.Hooks
	}
	return nil
}

func (m *MetadataStore) GetAllPackagesForDevice(deviceType, firmware, arch string) []Package {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var packages []Package

	for name, versions := range m.packages.Packages {
		var bestVersion string
		var bestInfo PackageVersion
		compatible := true

		for version, info := range versions {
			if !isVersionCompatible(info, deviceType, firmware, arch) {
				continue
			}
			if bestVersion == "" || versionpkg.Compare(version, bestVersion) > 0 {
				bestVersion = version
				bestInfo = info
			}
		}

		var incompatibleReason string

		if bestVersion == "" {
			compatible = false
			for version, info := range versions {
				if !isVersionCompatible(info, deviceType, "", arch) {
					continue
				}
				if bestVersion == "" || versionpkg.Compare(version, bestVersion) > 0 {
					bestVersion = version
					bestInfo = info
				}
			}
			if bestVersion != "" {
				incompatibleReason = "os"
			}
		}

		if bestVersion == "" {
			compatible = false
			incompatibleReason = "device"
			for version, info := range versions {
				if bestVersion == "" || versionpkg.Compare(version, bestVersion) > 0 {
					bestVersion = version
					bestInfo = info
				}
			}
		}

		if bestVersion == "" {
			continue
		}

		if compatible && firmware != "" {
			visited := map[string]bool{}
			if !m.depsInstallable(name, deviceType, firmware, arch, visited) {
				compatible = false
				incompatibleReason = "os"
			}
		}

		pkg := Package{
			Name:               name,
			Version:            bestVersion,
			Description:        bestInfo.Pkgdesc,
			UpstreamAuthor:     bestInfo.UpstreamAuthor,
			Categories:         bestInfo.Categories,
			License:            bestInfo.License,
			URL:                bestInfo.URL,
			OSMin:              bestInfo.OSMin,
			OSMax:              bestInfo.OSMax,
			OSConstraints:      bestInfo.OSConstraints,
			Devices:            bestInfo.Devices,
			Depends:            bestInfo.Depends,
			Conflicts:          bestInfo.Conflicts,
			Provides:           bestInfo.Provides,
			Arch:               bestInfo.Arch,
			Status:             bestInfo.Status,
			DonateURL:          bestInfo.DonateURL,
			ReadmeURL:          bestInfo.ReadmeURL,
			ChangelogURL:       bestInfo.ChangelogURL,
			Released:           bestInfo.Released,
			FirstReleased:      bestInfo.FirstReleased,
			Compatible:         compatible,
			IncompatibleReason: incompatibleReason,
		}

		dl30, dlAll, framework := m.getStats(name)
		pkg.Downloads30d = dl30
		pkg.DownloadsAll = dlAll
		pkg.IsFramework = framework

		packages = append(packages, pkg)
	}

	return packages
}

func (m *MetadataStore) GetPackageForTargetOS(name, targetOS, deviceType, arch string) *Package {
	m.mu.RLock()
	defer m.mu.RUnlock()

	versions, ok := m.packages.Packages[name]
	if !ok {
		return nil
	}

	var bestVersion string
	var bestInfo PackageVersion

	for version, info := range versions {
		if !isVersionCompatible(info, deviceType, targetOS, arch) {
			continue
		}
		if bestVersion == "" || versionpkg.Compare(version, bestVersion) > 0 {
			bestVersion = version
			bestInfo = info
		}
	}

	if bestVersion == "" {
		return nil
	}

	pkg := &Package{
		Name:           name,
		Version:        bestVersion,
		Description:    bestInfo.Pkgdesc,
		UpstreamAuthor: bestInfo.UpstreamAuthor,
		Categories:     bestInfo.Categories,
		License:        bestInfo.License,
		URL:            bestInfo.URL,
		OSMin:          bestInfo.OSMin,
		OSMax:          bestInfo.OSMax,
		OSConstraints:  bestInfo.OSConstraints,
		Devices:        bestInfo.Devices,
		Depends:        bestInfo.Depends,
		Conflicts:      bestInfo.Conflicts,
		Provides:       bestInfo.Provides,
		Arch:           bestInfo.Arch,
		Status:         bestInfo.Status,
		DonateURL:      bestInfo.DonateURL,
		ReadmeURL:      bestInfo.ReadmeURL,
		ChangelogURL:   bestInfo.ChangelogURL,
		Released:       bestInfo.Released,
		FirstReleased:  bestInfo.FirstReleased,
	}

	dl30, dlAll, framework := m.getStats(name)
	pkg.Downloads30d = dl30
	pkg.DownloadsAll = dlAll
	pkg.IsFramework = framework

	return pkg
}

// depsInstallable checks whether all transitive dependencies of a package
// have at least one compatible version available. Must be called under m.mu.RLock.
func (m *MetadataStore) depsInstallable(name, deviceType, firmware, arch string, visited map[string]bool) bool {
	if visited[name] {
		return true
	}
	visited[name] = true

	versions, ok := m.packages.Packages[name]
	if !ok {
		return false
	}

	var bestInfo *PackageVersion
	var bestVersion string
	for version, info := range versions {
		if !isVersionCompatible(info, deviceType, firmware, arch) {
			continue
		}
		if bestVersion == "" || versionpkg.Compare(version, bestVersion) > 0 {
			bestVersion = version
			v := info
			bestInfo = &v
		}
	}
	if bestInfo == nil {
		return false
	}

	for _, dep := range bestInfo.Depends {
		if strings.HasPrefix(dep, "!") {
			continue
		}
		depName := stripDepVersion(dep)
		if depName == "/bin/sh" {
			continue
		}

		if _, isReal := m.packages.Packages[depName]; !isReal {
			providers := m.providers[depName]
			if len(providers) == 0 {
				continue
			}
			satisfiable := false
			for _, provider := range providers {
				if m.depsInstallable(provider, deviceType, firmware, arch, visited) {
					satisfiable = true
					break
				}
			}
			if !satisfiable {
				return false
			}
			continue
		}

		if !m.depsInstallable(depName, deviceType, firmware, arch, visited) {
			return false
		}
	}
	return true
}

type ProviderOption struct {
	Name               string         `json:"name"`
	Version            string         `json:"version"`
	Description        string         `json:"description"`
	Compatible         bool           `json:"compatible"`
	IncompatibleReason string         `json:"incompatibleReason,omitempty"`
	OSMin              *string        `json:"osMin"`
	OSMax              *string        `json:"osMax"`
	OSConstraints      []OSConstraint `json:"osConstraints"`
}

type VirtualChoice struct {
	Virtual    string           `json:"virtual"`
	RequiredBy []string         `json:"requiredBy"`
	Providers  []ProviderOption `json:"providers"`
	Default    string           `json:"default,omitempty"`
}

func (c VirtualChoice) PreferredProvider() string {
	for _, provider := range c.Providers {
		if provider.Compatible && provider.Name == c.Default {
			return provider.Name
		}
	}
	for _, provider := range c.Providers {
		if provider.Compatible {
			return provider.Name
		}
	}
	return ""
}

func anyCompatible(providers []ProviderOption) bool {
	for _, provider := range providers {
		if provider.Compatible {
			return true
		}
	}
	return false
}

func (m *MetadataStore) defaultProvider(virtual string, providers []ProviderOption) string {
	name := m.remanager.Virtuals[virtual].Default
	for _, provider := range providers {
		if provider.Name == name {
			return name
		}
	}
	return ""
}

func (m *MetadataStore) providersOf(name, deviceType, firmware, arch string) []ProviderOption {
	var options []ProviderOption
	for _, provider := range m.providers[name] {
		version, info := m.bestVersion(provider, deviceType, firmware, arch)
		option := ProviderOption{Name: provider, Compatible: true}

		if info == nil {
			option.Compatible = false
			option.IncompatibleReason = "os"
			version, info = m.bestVersion(provider, deviceType, "", arch)
			if info == nil {
				option.IncompatibleReason = "device"
				version, info = m.bestVersion(provider, "", "", "")
			}
			if info == nil {
				continue
			}
		}

		option.Version = version
		option.Description = info.Pkgdesc
		option.OSMin = info.OSMin
		option.OSMax = info.OSMax
		option.OSConstraints = info.OSConstraints
		options = append(options, option)
	}
	return options
}

func (m *MetadataStore) bestVersion(name, deviceType, firmware, arch string) (string, *PackageVersion) {
	versions, ok := m.packages.Packages[name]
	if !ok {
		return "", nil
	}

	var bestVersion string
	var bestInfo *PackageVersion
	for version, info := range versions {
		if !isVersionCompatible(info, deviceType, firmware, arch) {
			continue
		}
		if bestVersion == "" || versionpkg.Compare(version, bestVersion) > 0 {
			bestVersion = version
			v := info
			bestInfo = &v
		}
	}
	return bestVersion, bestInfo
}

func (m *MetadataStore) ResolveVirtualChoices(requested, installed []string, deviceType, firmware, arch string) []VirtualChoice {
	m.mu.RLock()
	defer m.mu.RUnlock()

	present := make(map[string]bool, len(installed)+len(requested))
	for _, name := range installed {
		present[name] = true
	}
	for _, name := range requested {
		present[name] = true
	}

	dependents := make(map[string]map[string]bool)
	var order []string

	visited := make(map[string]bool)
	queue := append([]string{}, requested...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if visited[name] {
			continue
		}
		visited[name] = true

		_, info := m.bestVersion(name, deviceType, firmware, arch)
		if info == nil {
			continue
		}
		present[name] = true

		for _, dep := range info.Depends {
			if strings.HasPrefix(dep, "!") {
				continue
			}
			depName := stripDepVersion(dep)
			if _, isReal := m.packages.Packages[depName]; isReal {
				queue = append(queue, depName)
				continue
			}
			if len(m.providers[depName]) == 0 {
				continue
			}
			if _, seen := dependents[depName]; !seen {
				order = append(order, depName)
				dependents[depName] = make(map[string]bool)
			}
			dependents[depName][name] = true
		}
	}

	// Decided after the walk, so a provider that arrives as a plain dependency of something else in the tree covers the virtual too.
	satisfied := func(virtual string) bool {
		for _, provider := range m.providers[virtual] {
			if present[provider] {
				return true
			}
		}
		return false
	}

	sort.Strings(order)
	var choices []VirtualChoice
	for _, virtual := range order {
		if satisfied(virtual) {
			continue
		}
		providers := m.providersOf(virtual, deviceType, firmware, arch)
		if !anyCompatible(providers) {
			continue
		}
		choices = append(choices, VirtualChoice{
			Virtual:    virtual,
			RequiredBy: m.requestedNeeding(dependents[virtual], requested, deviceType, firmware, arch),
			Providers:  providers,
			Default:    m.defaultProvider(virtual, providers),
		})
	}
	return choices
}

// Names the requested packages rather than the dependency that declares the virtual, so the plan knows which of its entries wait on the answer.
func (m *MetadataStore) requestedNeeding(dependents map[string]bool, requested []string, deviceType, firmware, arch string) []string {
	var roots []string
	for _, root := range requested {
		for _, pkg := range m.dependencyTree(root, deviceType, firmware, arch) {
			if dependents[pkg.name] {
				roots = append(roots, root)
				break
			}
		}
	}
	return roots
}

func isVersionCompatible(info PackageVersion, deviceType, firmware, arch string) bool {
	if deviceType != "" && len(info.Devices) > 0 {
		found := false
		for _, d := range info.Devices {
			if d == deviceType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if arch != "" && len(info.Arch) > 0 {
		found := false
		for _, a := range info.Arch {
			if a == arch {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if firmware != "" {
		if len(info.OSConstraints) > 0 {
			for _, c := range info.OSConstraints {
				cmp := versionpkg.Compare(firmware, c.Version)
				switch c.Operator {
				case ">=":
					if cmp < 0 {
						return false
					}
				case ">":
					if cmp <= 0 {
						return false
					}
				case "<=":
					if cmp > 0 {
						return false
					}
				case "<":
					if cmp >= 0 {
						return false
					}
				case "=":
					if cmp != 0 {
						return false
					}
				}
			}
		} else {
			// Legacy fallback: os_max is exclusive
			if info.OSMin != nil && *info.OSMin != "" {
				if versionpkg.Compare(firmware, *info.OSMin) < 0 {
					return false
				}
			}
			if info.OSMax != nil && *info.OSMax != "" {
				if versionpkg.Compare(firmware, *info.OSMax) >= 0 {
					return false
				}
			}
		}
	}

	return true
}
