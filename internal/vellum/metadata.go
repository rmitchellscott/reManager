package vellum

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	_ "embed"

	"reManager/internal/debug"
	"reManager/internal/httputil"
	versionpkg "reManager/internal/version"
)

const (
	PackagesMetadataURL  = "https://packages.vellum.delivery/packages-metadata.json"
	RemanagerMetadataURL = "https://remanager.io/remanager-metadata.json"
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
}

type PackagesMetadata struct {
	Generated string                                `json:"generated"`
	Packages  map[string]map[string]PackageVersion `json:"packages"`
}

type MaintenanceCommand struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	Description      string `json:"description,omitempty"`
	Command          string `json:"command"`
	RequiresTerminal bool   `json:"requiresTerminal,omitempty"`
	AllowStop        bool   `json:"allowStop,omitempty"`
	Hook             string `json:"hook,omitempty"`
}

type PackageHooks struct {
	PostInstall   string `json:"postInstall,omitempty"`
	PreUninstall  string `json:"preUninstall,omitempty"`
	PostUninstall string `json:"postUninstall,omitempty"`
}

type RemanagerPackageInfo struct {
	MaintenanceCommands []MaintenanceCommand `json:"maintenanceCommands,omitempty"`
	Hooks               *PackageHooks        `json:"hooks,omitempty"`
}

type VirtualConfig struct {
	Default string `json:"default,omitempty"`
}

type RemanagerMetadata struct {
	Packages map[string]RemanagerPackageInfo `json:"packages"`
	Virtuals map[string]VirtualConfig        `json:"virtuals,omitempty"`
}

type Package struct {
	Name                string
	Version             string
	Description         string
	UpstreamAuthor      string
	Categories          []string
	License             string
	URL                 string
	OSMin               *string
	OSMax               *string
	OSConstraints       []OSConstraint
	Devices             []string
	Depends             []string
	Conflicts           []string
	Provides            []string
	Arch                []string
	Status              string
	DonateURL           *string
	ReadmeURL           *string
	Compatible          bool
	IncompatibleReason  string
	MaintenanceCommands []MaintenanceCommand
	Hooks               *PackageHooks
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
	m := &MetadataStore{}
	if err := json.Unmarshal(fallbackRemanagerJSON, &m.remanager); err != nil {
		debug.Printf("[DEBUG] Failed to parse embedded fallback remanager metadata: %v\n", err)
	}
	return m
}

func (m *MetadataStore) Load() error {
	if err := m.loadPackagesMetadata(); err != nil {
		m.err = fmt.Errorf("failed to fetch packages metadata: %w", err)
		return m.err
	}
	normalizePackagesMetadata(&m.packages)
	m.providers = buildProviderIndex(&m.packages)
	debug.Printf("[DEBUG] Loaded %d packages from metadata\n", len(m.packages.Packages))

	if err := m.loadRemanagerMetadata(); err != nil {
		debug.Printf("[DEBUG] HTTP fetch remanager failed: %v, using fallback\n", err)
		if err := json.Unmarshal(fallbackRemanagerJSON, &m.remanager); err != nil {
			return fmt.Errorf("failed to parse fallback remanager metadata: %w", err)
		}
	}
	debug.Printf("[DEBUG] Loaded %d remanager package configs\n", len(m.remanager.Packages))

	m.loaded = true
	return nil
}

func (m *MetadataStore) Ready() bool {
	return m.loaded
}

func (m *MetadataStore) Err() error {
	return m.err
}

func (m *MetadataStore) Refresh() error {
	client := httputil.NewClient(MetadataTimeout)

	var newPackages PackagesMetadata
	var newRemanager RemanagerMetadata

	resp, err := client.Get(PackagesMetadataURL)
	if err != nil {
		return fmt.Errorf("failed to fetch packages metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("packages metadata HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read packages metadata: %w", err)
	}

	if err := json.Unmarshal(body, &newPackages); err != nil {
		return fmt.Errorf("failed to parse packages metadata: %w", err)
	}

	resp2, err := client.Get(RemanagerMetadataURL)
	if err == nil {
		defer resp2.Body.Close()
		if resp2.StatusCode == http.StatusOK {
			body2, err := io.ReadAll(resp2.Body)
			if err == nil {
				json.Unmarshal(body2, &newRemanager)
			}
		}
	}

	normalizePackagesMetadata(&newPackages)

	m.mu.Lock()
	m.packages = newPackages
	m.providers = buildProviderIndex(&m.packages)
	if len(newRemanager.Packages) > 0 {
		m.remanager = newRemanager
	}
	m.loaded = true
	m.err = nil
	m.mu.Unlock()

	debug.Printf("[DEBUG] Refreshed metadata: %d packages, %d remanager configs\n", len(m.packages.Packages), len(m.remanager.Packages))
	return nil
}

func (m *MetadataStore) loadPackagesMetadata() error {
	client := httputil.NewClient(MetadataTimeout)
	resp, err := client.Get(PackagesMetadataURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(body, &m.packages)
}

func (m *MetadataStore) loadRemanagerMetadata() error {
	client := httputil.NewClient(MetadataTimeout)
	resp, err := client.Get(RemanagerMetadataURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(body, &m.remanager)
}

func (m *MetadataStore) GetAllPackages() []Package {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var packages []Package

	for name, versions := range m.packages.Packages {
		var latestVersion string
		var latestInfo PackageVersion
		for version, info := range versions {
			if latestVersion == "" || version > latestVersion {
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
		}

		if rmInfo, ok := m.remanager.Packages[name]; ok {
			pkg.MaintenanceCommands = rmInfo.MaintenanceCommands
			pkg.Hooks = rmInfo.Hooks
		}

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
		if latestVersion == "" || version > latestVersion {
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
	}

	if rmInfo, ok := m.remanager.Packages[name]; ok {
		pkg.MaintenanceCommands = rmInfo.MaintenanceCommands
		pkg.Hooks = rmInfo.Hooks
	}

	return pkg
}

func (m *MetadataStore) GetMaintenanceCommands(name string) []MaintenanceCommand {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if rmInfo, ok := m.remanager.Packages[name]; ok {
		return rmInfo.MaintenanceCommands
	}
	return nil
}

func (m *MetadataStore) AllMaintenanceCommands() map[string][]MaintenanceCommand {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]MaintenanceCommand, len(m.remanager.Packages))
	for name, info := range m.remanager.Packages {
		if len(info.MaintenanceCommands) > 0 {
			result[name] = info.MaintenanceCommands
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
			Compatible:         compatible,
			IncompatibleReason: incompatibleReason,
		}

		if rmInfo, ok := m.remanager.Packages[name]; ok {
			pkg.MaintenanceCommands = rmInfo.MaintenanceCommands
			pkg.Hooks = rmInfo.Hooks
		}

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
	}

	if rmInfo, ok := m.remanager.Packages[name]; ok {
		pkg.MaintenanceCommands = rmInfo.MaintenanceCommands
		pkg.Hooks = rmInfo.Hooks
	}

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

	satisfied := make(map[string]bool)
	for virtual, providers := range m.providers {
		for _, provider := range providers {
			if present[provider] {
				satisfied[virtual] = true
				break
			}
		}
	}

	requiredBy := make(map[string][]string)
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

		for _, dep := range info.Depends {
			if strings.HasPrefix(dep, "!") {
				continue
			}
			depName := stripDepVersion(dep)
			if _, isReal := m.packages.Packages[depName]; isReal {
				queue = append(queue, depName)
				continue
			}
			if satisfied[depName] || len(m.providers[depName]) == 0 {
				continue
			}
			if _, seen := requiredBy[depName]; !seen {
				order = append(order, depName)
			}
			requiredBy[depName] = append(requiredBy[depName], name)
		}
	}

	sort.Strings(order)
	var choices []VirtualChoice
	for _, virtual := range order {
		providers := m.providersOf(virtual, deviceType, firmware, arch)
		if !anyCompatible(providers) {
			continue
		}
		choices = append(choices, VirtualChoice{
			Virtual:    virtual,
			RequiredBy: requiredBy[virtual],
			Providers:  providers,
			Default:    m.defaultProvider(virtual, providers),
		})
	}
	return choices
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


