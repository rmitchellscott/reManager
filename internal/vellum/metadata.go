package vellum

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	_ "embed"

	"reManager/internal/debug"
)

const (
	PackagesMetadataURL  = "https://packages.vellum.delivery/packages-metadata.json"
	RemanagerMetadataURL = "https://packages.vellum.delivery/remanager-metadata.json"
	MetadataTimeout      = 10 * time.Second
)

//go:embed fallback_packages.json
var fallbackPackagesJSON []byte

//go:embed fallback_remanager.json
var fallbackRemanagerJSON []byte

type PackageVersion struct {
	Pkgdesc        string   `json:"pkgdesc"`
	UpstreamAuthor string   `json:"upstream_author"`
	Categories     []string `json:"categories"`
	License        string   `json:"license"`
	URL            string   `json:"url"`
	OSMin          *string  `json:"os_min"`
	OSMax          *string  `json:"os_max"`
	Devices        []string `json:"devices"`
	Depends        []string `json:"depends"`
	Conflicts      []string `json:"conflicts"`
	Arch           []string `json:"arch"`
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
	Devices             []string
	Depends             []string
	Conflicts           []string
	Arch                []string
	MaintenanceCommands []MaintenanceCommand
	Hooks               *PackageHooks
}

type MetadataStore struct {
	mu        sync.RWMutex
	packages  PackagesMetadata
	remanager RemanagerMetadata
}

func NewMetadataStore() *MetadataStore {
	return &MetadataStore{}
}

func (m *MetadataStore) Load() error {
	if err := m.loadPackagesMetadata(); err != nil {
		debug.Printf("[DEBUG] HTTP fetch packages failed: %v, using fallback\n", err)
		if err := json.Unmarshal(fallbackPackagesJSON, &m.packages); err != nil {
			return fmt.Errorf("failed to parse fallback packages metadata: %w", err)
		}
	}
	debug.Printf("[DEBUG] Loaded %d packages from metadata\n", len(m.packages.Packages))

	if err := m.loadRemanagerMetadata(); err != nil {
		debug.Printf("[DEBUG] HTTP fetch remanager failed: %v, using fallback\n", err)
		if err := json.Unmarshal(fallbackRemanagerJSON, &m.remanager); err != nil {
			return fmt.Errorf("failed to parse fallback remanager metadata: %w", err)
		}
	}
	debug.Printf("[DEBUG] Loaded %d remanager package configs\n", len(m.remanager.Packages))

	return nil
}

func (m *MetadataStore) Refresh() error {
	client := &http.Client{Timeout: MetadataTimeout}

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

	m.mu.Lock()
	m.packages = newPackages
	if len(newRemanager.Packages) > 0 {
		m.remanager = newRemanager
	}
	m.mu.Unlock()

	debug.Printf("[DEBUG] Refreshed metadata: %d packages, %d remanager configs\n", len(m.packages.Packages), len(m.remanager.Packages))
	return nil
}

func (m *MetadataStore) loadPackagesMetadata() error {
	client := &http.Client{Timeout: MetadataTimeout}
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
	client := &http.Client{Timeout: MetadataTimeout}
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
			Devices:        latestInfo.Devices,
			Depends:        latestInfo.Depends,
			Conflicts:      latestInfo.Conflicts,
			Arch:           latestInfo.Arch,
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
		Devices:        latestInfo.Devices,
		Depends:        latestInfo.Depends,
		Conflicts:      latestInfo.Conflicts,
		Arch:           latestInfo.Arch,
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

		for version, info := range versions {
			if !isVersionCompatible(info, deviceType, firmware, arch) {
				continue
			}
			if bestVersion == "" || compareVersions(version, bestVersion) > 0 {
				bestVersion = version
				bestInfo = info
			}
		}

		if bestVersion == "" {
			continue
		}

		pkg := Package{
			Name:           name,
			Version:        bestVersion,
			Description:    bestInfo.Pkgdesc,
			UpstreamAuthor: bestInfo.UpstreamAuthor,
			Categories:     bestInfo.Categories,
			License:        bestInfo.License,
			URL:            bestInfo.URL,
			OSMin:          bestInfo.OSMin,
			OSMax:          bestInfo.OSMax,
			Devices:        bestInfo.Devices,
			Depends:        bestInfo.Depends,
			Conflicts:      bestInfo.Conflicts,
			Arch:           bestInfo.Arch,
		}

		if rmInfo, ok := m.remanager.Packages[name]; ok {
			pkg.MaintenanceCommands = rmInfo.MaintenanceCommands
			pkg.Hooks = rmInfo.Hooks
		}

		packages = append(packages, pkg)
	}

	return packages
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
		if info.OSMin != nil && *info.OSMin != "" {
			if compareVersions(firmware, *info.OSMin) < 0 {
				return false
			}
		}
		if info.OSMax != nil && *info.OSMax != "" {
			if compareVersions(firmware, *info.OSMax) > 0 {
				return false
			}
		}
	}

	return true
}

func compareVersions(a, b string) int {
	aParts := parseVersionParts(a)
	bParts := parseVersionParts(b)

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		var aNum, bNum int
		if i < len(aParts) {
			aNum = aParts[i]
		}
		if i < len(bParts) {
			bNum = bParts[i]
		}
		if aNum > bNum {
			return 1
		}
		if aNum < bNum {
			return -1
		}
	}
	return 0
}

func parseVersionParts(v string) []int {
	var parts []int
	var current string
	for _, c := range v {
		if c == '.' || c == '-' {
			if current != "" {
				parts = append(parts, parseNum(current))
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, parseNum(current))
	}
	return parts
}

func parseNum(s string) int {
	num := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		} else {
			break
		}
	}
	return num
}
