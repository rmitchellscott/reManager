package vellum

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"reManager/internal/debug"
	"reManager/internal/httputil"
	versionpkg "reManager/internal/version"
)

const (
	PackagesMetadataURL  = "https://packages.vellum.delivery/packages-metadata.json"
	RemanagerMetadataURL = "https://packages.vellum.delivery/remanager-metadata.json"
	MetadataTimeout      = 10 * time.Second
)

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

type RemanagerMetadata struct {
	Packages map[string]RemanagerPackageInfo `json:"packages"`
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
	loaded    bool
	err       error
}

func NewMetadataStore() *MetadataStore {
	return &MetadataStore{}
}

func (m *MetadataStore) Load() error {
	if err := m.loadPackagesMetadata(); err != nil {
		m.err = fmt.Errorf("failed to fetch packages metadata: %w", err)
		return m.err
	}
	normalizePackagesMetadata(&m.packages)
	debug.Printf("[DEBUG] Loaded %d packages from metadata\n", len(m.packages.Packages))

	if err := m.loadRemanagerMetadata(); err != nil {
		m.err = fmt.Errorf("failed to fetch remanager metadata: %w", err)
		return m.err
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
		depName := stripDepVersion(dep)
		if depName == "/bin/sh" {
			continue
		}
		if !m.depsInstallable(depName, deviceType, firmware, arch, visited) {
			return false
		}
	}
	return true
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

func stripDepVersion(dep string) string {
	for _, op := range []string{">=", "<=", "=", ">", "<"} {
		if i := strings.Index(dep, op); i > 0 {
			return dep[:i]
		}
	}
	return dep
}

