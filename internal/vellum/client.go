package vellum

import (
	"fmt"
	"regexp"
	"strings"

	"reManager/internal/debug"
	"reManager/internal/executor"
)

const (
	VellumRoot = "/home/root/.vellum"
	VellumBin  = "/home/root/.vellum/bin/vellum"
)

var CorePackages = []string{"vellum", "remarkable-os"}

type Client struct {
	executor executor.CommandExecutor
}

func NewClient(exec executor.CommandExecutor) *Client {
	return &Client{executor: exec}
}

func (c *Client) IsInstalled() (bool, error) {
	cmd := fmt.Sprintf("test -x %s && echo yes || echo no", VellumBin)
	debug.Printf("[DEBUG] IsInstalled running: %s\n", cmd)
	output, err := c.executor.ExecuteWithOutput(cmd)
	debug.Printf("[DEBUG] IsInstalled output: %q, err: %v\n", output, err)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) == "yes", nil
}

func (c *Client) ValidateInstall() (valid bool, missing []string, err error) {
	installed, err := c.List()
	if err != nil {
		return false, nil, err
	}

	installedSet := make(map[string]bool)
	for _, pkg := range installed {
		installedSet[pkg] = true
	}

	for _, core := range CorePackages {
		if !installedSet[core] {
			missing = append(missing, core)
		}
	}

	return len(missing) == 0, missing, nil
}

func (c *Client) Add(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	cmd := fmt.Sprintf("%s add %s", VellumBin, strings.Join(packages, " "))
	_, err := c.executor.ExecuteWithOutput(cmd)
	return err
}

func (c *Client) AddStreaming(onOutput func(line string), packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	cmd := fmt.Sprintf("%s add %s", VellumBin, strings.Join(packages, " "))
	debug.Printf("[DEBUG] AddStreaming running: %s\n", cmd)
	err := c.executor.ExecuteStreaming(cmd, onOutput)
	debug.Printf("[DEBUG] AddStreaming result: err=%v\n", err)
	return err
}

func (c *Client) Del(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	cmd := fmt.Sprintf("%s del %s", VellumBin, strings.Join(packages, " "))
	_, err := c.executor.ExecuteWithOutput(cmd)
	return err
}

func (c *Client) DelStreaming(onOutput func(line string), packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	cmd := fmt.Sprintf("%s del %s", VellumBin, strings.Join(packages, " "))
	return c.executor.ExecuteStreaming(cmd, onOutput)
}

func (c *Client) Update() error {
	cmd := fmt.Sprintf("%s update -y", VellumBin)
	_, err := c.executor.ExecuteWithOutput(cmd)
	return err
}

func (c *Client) List() ([]string, error) {
	result, err := c.ListWithOsCheck()
	if err != nil {
		return nil, err
	}
	return result.Packages, nil
}

type ListResult struct {
	Packages    []string
	OsUpgraded  bool
	PrevVersion string
	NewVersion  string
}

type OSVersionState struct {
	CurrentVersion string
	StoredVersion  string
	Mismatch       bool
}

type CompatibilityResult struct {
	Compatible   []string
	Incompatible []string
	NoConstraint []string
	FetchFailed  bool
}

var osUpgradeRegex = regexp.MustCompile(`OS updated \(([^\s]+) → ([^\s]+)\)`)

func (c *Client) ListWithOsCheck() (*ListResult, error) {
	cmd := fmt.Sprintf("%s info -q", VellumBin)
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		return nil, err
	}

	result := &ListResult{}

	for _, line := range strings.Split(output, "\n") {
		if matches := osUpgradeRegex.FindStringSubmatch(line); len(matches) >= 3 {
			result.OsUpgraded = true
			result.PrevVersion = matches[1]
			result.NewVersion = matches[2]
			continue
		}

		pkg := strings.TrimSpace(line)
		if pkg != "" && !strings.HasPrefix(pkg, "Run 'vellum") {
			result.Packages = append(result.Packages, pkg)
		}
	}
	return result, nil
}

func (c *Client) ReenableStreaming(onOutput func(line string)) error {
	cmd := fmt.Sprintf("%s reenable", VellumBin)
	return c.executor.ExecuteStreaming(cmd, onOutput)
}

func (c *Client) GetOSVersionState() (*OSVersionState, error) {
	var currentVersion string

	output, err := c.executor.ExecuteWithOutput("grep '^RELEASE_VERSION=' /usr/share/remarkable/update.conf 2>/dev/null")
	if err == nil {
		if parts := strings.SplitN(output, "=", 2); len(parts) == 2 {
			currentVersion = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		}
	}

	if currentVersion == "" {
		output, err = c.executor.ExecuteWithOutput("grep '^IMG_VERSION=' /etc/os-release 2>/dev/null")
		if err == nil {
			if parts := strings.SplitN(output, "=", 2); len(parts) == 2 {
				currentVersion = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
			}
		}
	}

	storedOutput, _ := c.executor.ExecuteWithOutput(fmt.Sprintf("cat %s/state/osver 2>/dev/null", VellumRoot))
	storedVersion := strings.TrimSpace(storedOutput)

	return &OSVersionState{
		CurrentVersion: currentVersion,
		StoredVersion:  storedVersion,
		Mismatch:       currentVersion != "" && storedVersion != "" && currentVersion != storedVersion,
	}, nil
}

func (c *Client) CheckOSCompatibility(targetOS string) (*CompatibilityResult, error) {
	cmd := fmt.Sprintf("%s check-os %s", VellumBin, targetOS)
	debug.Printf("[DEBUG] CheckOSCompatibility: running '%s'\n", cmd)
	output, err := c.executor.ExecuteWithOutput(cmd)
	debug.Printf("[DEBUG] CheckOSCompatibility: output=%q, err=%v\n", output, err)

	result := &CompatibilityResult{
		Compatible:   []string{},
		Incompatible: []string{},
		NoConstraint: []string{},
	}

	if err != nil && output == "" {
		result.FetchFailed = true
		debug.Printf("[DEBUG] CheckOSCompatibility: early return with FetchFailed=true\n")
		return result, err
	}

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "✓") || strings.HasPrefix(trimmed, "[OK]") || strings.HasPrefix(trimmed, "+") {
			pkg := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(trimmed, "✓"), "[OK]"), "+"))
			if pkg != "" {
				result.Compatible = append(result.Compatible, pkg)
			}
		} else if strings.HasPrefix(trimmed, "✗") || strings.HasPrefix(trimmed, "[FAIL]") || strings.HasPrefix(trimmed, "x ") {
			pkg := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(trimmed, "✗"), "[FAIL]"), "x"))
			if pkg != "" {
				result.Incompatible = append(result.Incompatible, pkg)
			}
		} else if strings.HasPrefix(trimmed, "-") {
			pkg := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if pkg != "" && !strings.Contains(pkg, "Packages without") {
				result.NoConstraint = append(result.NoConstraint, pkg)
			}
		}
	}

	if strings.Contains(output, "Could not fetch") || strings.Contains(output, "fetch failed") {
		result.FetchFailed = true
	}

	debug.Printf("[DEBUG] CheckOSCompatibility: result=%+v\n", result)
	return result, nil
}

func (c *Client) UpgradeStreaming(onOutput func(line string)) error {
	cmd := fmt.Sprintf("%s upgrade -y", VellumBin)
	return c.executor.ExecuteStreaming(cmd, onOutput)
}

type UpgradeSimulationResult struct {
	Packages    []string
	HasUpgrades bool
}

func (c *Client) SimulateUpgrade() (*UpgradeSimulationResult, error) {
	cmd := fmt.Sprintf("%s upgrade --simulate", VellumBin)
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		return &UpgradeSimulationResult{}, nil
	}
	packages := parseUpgradeSimulationOutput(output)
	return &UpgradeSimulationResult{
		Packages:    packages,
		HasUpgrades: len(packages) > 0,
	}, nil
}

var upgradeLineRegex = regexp.MustCompile(`\(\s*\d+/\d+\)\s+(?:Upgrading|Installing)\s+([^\s]+)\s+\(`)

func parseUpgradeSimulationOutput(output string) []string {
	var packages []string
	for _, line := range strings.Split(output, "\n") {
		matches := upgradeLineRegex.FindStringSubmatch(line)
		if len(matches) >= 2 {
			packages = append(packages, matches[1])
		}
	}
	return packages
}

func (c *Client) IsPackageInstalled(pkg string) (bool, error) {
	installed, err := c.List()
	if err != nil {
		return false, err
	}
	for _, p := range installed {
		if p == pkg {
			return true, nil
		}
	}
	return false, nil
}

// FetchURLs returns the download URLs for packages and their dependencies
func (c *Client) FetchURLs(packages ...string) ([]string, error) {
	if len(packages) == 0 {
		return nil, nil
	}
	args := strings.Join(packages, " ")
	cmd := fmt.Sprintf("%s fetch --url --simulate %s", VellumBin, args)
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		return nil, err
	}

	var urls []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "https://") {
			urls = append(urls, line)
		}
	}
	return urls, nil
}

// SimulateAdd returns the list of packages that would actually be installed
// (excludes packages that are already installed)
func (c *Client) SimulateAdd(packages ...string) ([]string, error) {
	if len(packages) == 0 {
		return nil, nil
	}
	cmd := fmt.Sprintf("%s add --simulate %s", VellumBin, strings.Join(packages, " "))
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		if output != "" {
			return nil, fmt.Errorf("%s", strings.TrimSpace(output))
		}
		return nil, err
	}
	return parseSimulationOutput(output), nil
}

// SimulateDelResult contains the result of simulating a package deletion
type SimulateDelResult struct {
	Packages []string            // packages that would be removed
	Blocked  map[string][]string // pkg -> list of blocking packages (e.g. "xovi" -> ["hide-dev-mode-icon"])
}

// SimulateDel simulates package deletion and returns packages to remove and any blockers
func (c *Client) SimulateDel(packages ...string) (*SimulateDelResult, error) {
	if len(packages) == 0 {
		return &SimulateDelResult{}, nil
	}
	cmd := fmt.Sprintf("%s del -s %s", VellumBin, strings.Join(packages, " "))
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		if output != "" {
			return nil, fmt.Errorf("%s", strings.TrimSpace(output))
		}
		return nil, err
	}

	result := &SimulateDelResult{
		Packages: parseSimulationOutput(output),
		Blocked:  parseBlockedPackages(output),
	}
	return result, nil
}

// SimulateDelRecursive simulates recursive package deletion (includes dependents)
func (c *Client) SimulateDelRecursive(packages ...string) ([]string, error) {
	if len(packages) == 0 {
		return nil, nil
	}
	cmd := fmt.Sprintf("%s del -rs %s", VellumBin, strings.Join(packages, " "))
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		return nil, err
	}
	return parseSimulationOutput(output), nil
}

// DelRecursiveStreaming removes packages and their dependents with streaming output
func (c *Client) DelRecursiveStreaming(onOutput func(line string), packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	cmd := fmt.Sprintf("%s del -r %s", VellumBin, strings.Join(packages, " "))
	return c.executor.ExecuteStreaming(cmd, onOutput)
}

// parseSimulationOutput extracts package names from vellum simulation output
// Example lines: "( 1/20) Purging hide-dev-mode-icon (1.0.0-r0)"
//
//	"(10/20) Installing qt-resource-rebuilder (16.0.0-r0)"
var simulationLineRegex = regexp.MustCompile(`\(\s*\d+/\d+\)\s+(?:Installing|Purging)\s+([^\s]+)\s+\(`)

func parseSimulationOutput(output string) []string {
	var packages []string
	for _, line := range strings.Split(output, "\n") {
		matches := simulationLineRegex.FindStringSubmatch(line)
		if len(matches) >= 2 {
			packages = append(packages, matches[1])
		}
	}
	return packages
}

// parseBlockedPackages extracts blocked package info from vellum simulation output
// Example output (can wrap to multiple lines):
//
//	World updated, but the following packages are not removed due to:
//	  xovi: quicksettings-screenshot bettertoc hide-dev-mode-icon
//	        disable-selection-autoscroll gesture-toolbar-show
func parseBlockedPackages(output string) map[string][]string {
	blocked := make(map[string][]string)
	inBlockedSection := false
	var currentPkg string

	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "not removed due to:") {
			inBlockedSection = true
			continue
		}
		if inBlockedSection {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				inBlockedSection = false
				currentPkg = ""
				continue
			}

			// Check for "pkg: blockers" format (line starts with 2 spaces and has colon)
			if idx := strings.Index(line, ":"); idx > 0 && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "        ") {
				parts := strings.SplitN(trimmed, ":", 2)
				if len(parts) == 2 {
					currentPkg = strings.TrimSpace(parts[0])
					blockers := strings.Fields(parts[1])
					blocked[currentPkg] = append(blocked[currentPkg], blockers...)
				}
			} else if currentPkg != "" {
				// Continuation line - just indented blockers
				blockers := strings.Fields(trimmed)
				blocked[currentPkg] = append(blocked[currentPkg], blockers...)
			}
		}
	}
	return blocked
}

func (c *Client) UninstallVellum(removeAllPackages bool, onOutput func(line string)) error {
	var cmd string
	if removeAllPackages {
		onOutput("Removing Vellum and all packages...\n")
		cmd = fmt.Sprintf("%s self uninstall --all -y", VellumBin)
	} else {
		onOutput("Removing Vellum...\n")
		cmd = fmt.Sprintf("%s self uninstall -y", VellumBin)
	}
	return c.executor.ExecuteStreaming(cmd, onOutput)
}
