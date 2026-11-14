package vellum

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"reManager/internal/debug"
	"reManager/internal/executor"
)

const (
	VellumRoot = "/home/root/.vellum"
	VellumBin  = "/home/root/.vellum/bin/vellum"
)

var CorePackages = []string{"vellum", "remarkable-os"}

var VirtualPackages = map[string]bool{
	"remarkable-os": true,
	"rm1":           true,
	"rm2":           true,
	"rmpp":          true,
	"rmppmove":      true,
	"rmppure":       true,
}

func IsVirtualPackage(name string) bool {
	return VirtualPackages[name]
}

var (
	packageNameRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
	osVersionRegex   = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)
)

func IsValidPackageName(name string) bool {
	return packageNameRegex.MatchString(name)
}

func IsValidOSVersion(version string) bool {
	return osVersionRegex.MatchString(version)
}

type Client struct {
	executor executor.CommandExecutor

	cacheMu      sync.Mutex
	listResult   *ListResult
	listAt       time.Time
	withVersions map[string]string
	versionsAt   time.Time
	osState      *OSVersionState
	osStateAt    time.Time
}

const installedCacheTTL = 5 * time.Second

func (c *Client) InvalidateInstalledCache() {
	c.cacheMu.Lock()
	c.listResult = nil
	c.withVersions = nil
	c.osState = nil
	c.cacheMu.Unlock()
}

func NewClient(exec executor.CommandExecutor) *Client {
	return &Client{executor: oneVellumAtATime{exec}}
}

// vellum rewrites its world file with no lock while it simulates an upgrade, then writes the
// saved copy back, so two vellum processes that overlap on the tablet can undo a removal or tear
// the file. The lock is package-wide because the app keeps more than one Client for a device.
var vellumOnDevice sync.Mutex

type oneVellumAtATime struct {
	executor.CommandExecutor
}

func (e oneVellumAtATime) ExecuteWithOutput(cmd string) (string, error) {
	if !strings.Contains(cmd, VellumBin) {
		return e.CommandExecutor.ExecuteWithOutput(cmd)
	}
	vellumOnDevice.Lock()
	defer vellumOnDevice.Unlock()
	return e.CommandExecutor.ExecuteWithOutput(cmd)
}

func (e oneVellumAtATime) ExecuteStreaming(cmd string, onOutput func(line string)) error {
	if !strings.Contains(cmd, VellumBin) {
		return e.CommandExecutor.ExecuteStreaming(cmd, onOutput)
	}
	vellumOnDevice.Lock()
	defer vellumOnDevice.Unlock()
	return e.CommandExecutor.ExecuteStreaming(cmd, onOutput)
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

func (c *Client) AddStreaming(onOutput func(line string), packages ...string) error {
	defer c.InvalidateInstalledCache()
	if len(packages) == 0 {
		return nil
	}
	cmd := fmt.Sprintf("%s add %s", VellumBin, executor.QuoteArgs(packages))
	debug.Printf("[DEBUG] AddStreaming running: %s\n", cmd)
	err := c.executor.ExecuteStreaming(cmd, onOutput)
	debug.Printf("[DEBUG] AddStreaming result: err=%v\n", err)
	return err
}

func (c *Client) DelStreaming(onOutput func(line string), packages ...string) error {
	defer c.InvalidateInstalledCache()
	if len(packages) == 0 {
		return nil
	}
	cmd := fmt.Sprintf("%s del %s", VellumBin, executor.QuoteArgs(packages))
	return c.executor.ExecuteStreaming(cmd, onOutput)
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
var installedPkgRegex = regexp.MustCompile(`^(.+)-(\d+(?:\.\d+)*(?:_\w+)?(?:-r\d+)?)\s+\S+\s+\{`)

func (c *Client) ListInstalledWithTimes() (map[string]int64, error) {
	cmd := fmt.Sprintf("cat %s/lib/apk/db/installed", VellumRoot)
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		return nil, err
	}

	result := make(map[string]int64)
	var currentPkg string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "P:") {
			currentPkg = line[2:]
		} else if strings.HasPrefix(line, "t:") && currentPkg != "" {
			var ts int64
			fmt.Sscanf(line[2:], "%d", &ts)
			result[currentPkg] = ts
			currentPkg = ""
		} else if line == "" {
			currentPkg = ""
		}
	}
	return result, nil
}

func (c *Client) ListInstalledWithVersions() (map[string]string, error) {
	c.cacheMu.Lock()
	if c.withVersions != nil && time.Since(c.versionsAt) < installedCacheTTL {
		cached := c.withVersions
		c.cacheMu.Unlock()
		return cached, nil
	}
	c.cacheMu.Unlock()

	cmd := fmt.Sprintf("%s list -I", VellumBin)
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if matches := installedPkgRegex.FindStringSubmatch(line); len(matches) >= 3 {
			pkgName := matches[1]
			version := matches[2]
			result[pkgName] = version
		}
	}

	c.cacheMu.Lock()
	c.withVersions = result
	c.versionsAt = time.Now()
	c.cacheMu.Unlock()

	return result, nil
}

func (c *Client) ListWithOsCheck() (*ListResult, error) {
	c.cacheMu.Lock()
	if c.listResult != nil && time.Since(c.listAt) < installedCacheTTL {
		cached := c.listResult
		c.cacheMu.Unlock()
		return cached, nil
	}
	c.cacheMu.Unlock()

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

	c.cacheMu.Lock()
	c.listResult = result
	c.listAt = time.Now()
	c.cacheMu.Unlock()

	return result, nil
}

type ReenableStatus string

const (
	ReenableNeeded   ReenableStatus = "needed"
	ReenableOK       ReenableStatus = "ok"
	ReenableUnneeded ReenableStatus = "unneeded"
)

func (c *Client) ReenableStatus() (ReenableStatus, error) {
	cmd := fmt.Sprintf("%s reenable status", VellumBin)
	output, err := c.executor.ExecuteWithOutput(cmd)

	for _, line := range strings.Split(output, "\n") {
		switch status := ReenableStatus(strings.TrimSpace(line)); status {
		case ReenableNeeded, ReenableOK, ReenableUnneeded:
			return status, nil
		}
	}

	if err != nil {
		return "", err
	}
	return "", nil
}

func (c *Client) ReenableStreaming(onOutput func(line string)) error {
	defer c.InvalidateInstalledCache()
	cmd := fmt.Sprintf("%s reenable", VellumBin)
	return c.executor.ExecuteStreaming(cmd, onOutput)
}

// Cached like the installed list: the Plan's checks ask for it several times per change and each read is up to three SSH round trips; a sync invalidates the cache.
func (c *Client) GetOSVersionState() (*OSVersionState, error) {
	c.cacheMu.Lock()
	if c.osState != nil && time.Since(c.osStateAt) < installedCacheTTL {
		cached := *c.osState
		c.cacheMu.Unlock()
		return &cached, nil
	}
	c.cacheMu.Unlock()

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

	// Not cached on failure: an empty version reads as no mismatch and would skip the pre-upgrade compatibility check. A missing osver exits non-zero and is a real empty value.
	if currentVersion == "" {
		return nil, fmt.Errorf("could not read the OS version")
	}
	storedOutput, storedErr := c.executor.ExecuteWithOutput(fmt.Sprintf("cat %s/state/osver 2>/dev/null", VellumRoot))
	var exitErr *ssh.ExitError
	if storedErr != nil && !errors.As(storedErr, &exitErr) {
		return nil, fmt.Errorf("could not read the stored OS version: %w", storedErr)
	}
	storedVersion := strings.TrimSpace(storedOutput)

	state := &OSVersionState{
		CurrentVersion: currentVersion,
		StoredVersion:  storedVersion,
		Mismatch:       currentVersion != "" && storedVersion != "" && currentVersion != storedVersion,
	}
	c.cacheMu.Lock()
	c.osState = state
	c.osStateAt = time.Now()
	c.cacheMu.Unlock()
	cached := *state
	return &cached, nil
}

func (c *Client) UpdateIndex() error {
	cmd := fmt.Sprintf("%s update", VellumBin)
	debug.Printf("[DEBUG] UpdateIndex: running '%s'\n", cmd)
	_, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		debug.Printf("[DEBUG] UpdateIndex: err=%v\n", err)
	}
	return err
}

func (c *Client) CheckOSCompatibility(targetOS string) (*CompatibilityResult, error) {
	cmd := fmt.Sprintf("%s check-os %s", VellumBin, executor.QuoteArg(targetOS))
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

// Named packages go to apk upgrade, which touches only those and leaves /etc/apk/world alone; using "add" would promote a transitive dependency to explicitly installed. No names upgrades everything.
func (c *Client) UpgradeStreaming(onOutput func(line string), packages ...string) error {
	defer c.InvalidateInstalledCache()
	cmd := fmt.Sprintf("%s upgrade -y", VellumBin)
	if len(packages) > 0 {
		cmd = fmt.Sprintf("%s %s", cmd, executor.QuoteArgs(packages))
	}
	return c.executor.ExecuteStreaming(cmd, onOutput)
}

// apk announces each package as it starts, the only per-package signal the upgrade path produces; keeps the counters so a live run can report its position.
var upgradeProgressRegex = regexp.MustCompile(`\(\s*(\d+)/(\d+)\s*\)\s+(Upgrading|Installing|Downgrading|Purging)\s+([^\s]+)\s+\(`)

func ParseUpgradeProgress(line string) (pkg string, index, total int, ok bool) {
	m := upgradeProgressRegex.FindStringSubmatch(line)
	if m == nil {
		return "", 0, 0, false
	}
	current, err1 := strconv.Atoi(m[1])
	count, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil {
		return "", 0, 0, false
	}
	return m[4], current - 1, count, true
}

var upgradeLineRegex = regexp.MustCompile(`\(\s*\d+/\d+\)\s+(Upgrading|Installing|Downgrading)\s+([^\s]+)\s+\(`)
var downgradeTargetRegex = regexp.MustCompile(`->\s+([^\s)]+)`)
var upgradeVersionRegex = regexp.MustCompile(`\(\s*\d+/\d+\)\s+(?:Upgrading|Installing|Downgrading)\s+[^\s]+\s+\(([^)]+)\)`)

func parseUpgradeSimulationOutputWithVersions(output string) []SimulatedPackage {
	var packages []SimulatedPackage
	for _, line := range strings.Split(output, "\n") {
		matches := upgradeLineRegex.FindStringSubmatch(line)
		if len(matches) < 3 {
			continue
		}
		pkgName := matches[2]

		var version string
		if target := downgradeTargetRegex.FindStringSubmatch(line); len(target) >= 2 {
			version = target[1]
		} else if vMatch := upgradeVersionRegex.FindStringSubmatch(line); len(vMatch) >= 2 {
			version = vMatch[1]
		}

		if version != "" {
			packages = append(packages, SimulatedPackage{Name: pkgName, Version: version})
		}
	}
	return packages
}

// Passing names lets apk decide the full set, so a dependency the solver pulls in comes back in the result rather than being guessed here.
func (c *Client) SimulateUpgradeWithVersions(packages ...string) ([]SimulatedPackage, error) {
	cmd := fmt.Sprintf("%s upgrade --simulate --force-missing-repositories", VellumBin)
	if len(packages) > 0 {
		cmd = fmt.Sprintf("%s %s", cmd, executor.QuoteArgs(packages))
	}
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		// Callers treat an empty slice as "nothing to upgrade", so swallowing this error would report an unreachable device as up to date.
		return nil, fmt.Errorf("upgrade simulation failed: %w", err)
	}
	return parseUpgradeSimulationOutputWithVersions(output), nil
}

// Packages that are already installed are left out of the result.
func (c *Client) SimulateAdd(packages ...string) ([]string, error) {
	if len(packages) == 0 {
		return nil, nil
	}
	output, err := c.simulateAddOutput(packages)
	if err != nil {
		return nil, err
	}
	return parseSimulationOutput(output), nil
}

func (c *Client) simulateAddOutput(args []string) (string, error) {
	cmd := fmt.Sprintf("%s add --simulate %s", VellumBin, executor.QuoteArgs(args))
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		if output != "" {
			return "", fmt.Errorf("%s", strings.TrimSpace(output))
		}
		return "", err
	}
	return output, nil
}

type AddSimulation struct {
	Packages       []string
	VersionChanges []VersionChange
}

// Solves an add as if the removed packages were gone: each is passed as an apk "!name" constraint, and the purge that implies is left out of the result.
func (c *Client) SimulateAddWithout(packages []string, removed []string) (*AddSimulation, error) {
	if len(packages) == 0 {
		return &AddSimulation{}, nil
	}
	args := append([]string{}, packages...)
	for _, name := range removed {
		args = append(args, "!"+name)
	}
	output, err := c.simulateAddOutput(args)
	if err != nil {
		return nil, err
	}
	isRemoved := make(map[string]bool, len(removed))
	for _, name := range removed {
		isRemoved[name] = true
	}
	simulation := &AddSimulation{VersionChanges: parseVersionChanges(output)}
	for _, name := range parseSimulationOutput(output) {
		if !isRemoved[name] {
			simulation.Packages = append(simulation.Packages, name)
		}
	}
	return simulation, nil
}

// Same transaction as SimulateAddWithout, returning only what it would purge: each removed package plus whatever nothing left in world still needs.
func (c *Client) SimulateAddPurges(packages []string, removed []string) ([]string, error) {
	if len(removed) == 0 {
		return nil, nil
	}
	args := append([]string{}, packages...)
	for _, name := range removed {
		args = append(args, "!"+name)
	}
	cmd := fmt.Sprintf("%s add --simulate %s", VellumBin, executor.QuoteArgs(args))
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		if output != "" {
			return nil, fmt.Errorf("%s", strings.TrimSpace(output))
		}
		return nil, err
	}
	var purged []string
	for _, line := range strings.Split(output, "\n") {
		if matches := purgeLineRegex.FindStringSubmatch(line); len(matches) >= 2 {
			purged = append(purged, matches[1])
		}
	}
	return purged, nil
}

var conflictSubjectRegex = regexp.MustCompile(`^\s{2}(\S+?)-[^-\s]+-r\d+:\s*$`)
var conflictBreaksRegex = regexp.MustCompile(`breaks:\s+(\S+?)-[^-\s\[]+-r\d+`)
var unselectedVirtualRegex = regexp.MustCompile(`^\s+(\S+) \(virtual\):\s*$`)

type ResolutionConflict struct {
	Package string
	Breaks  string
}

func ParseUnselectedVirtuals(message string) []string {
	var virtuals []string
	seen := make(map[string]bool)

	for _, line := range strings.Split(message, "\n") {
		match := unselectedVirtualRegex.FindStringSubmatch(line)
		if match == nil || seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		virtuals = append(virtuals, match[1])
	}
	return virtuals
}

func ParseResolutionConflicts(message string) []ResolutionConflict {
	var conflicts []ResolutionConflict
	seen := make(map[string]bool)
	subject := ""

	for _, line := range strings.Split(message, "\n") {
		if match := conflictSubjectRegex.FindStringSubmatch(line); match != nil {
			subject = match[1]
			continue
		}
		match := conflictBreaksRegex.FindStringSubmatch(line)
		if match == nil || subject == "" {
			continue
		}

		key := subject + "\x00" + match[1]
		if subject > match[1] {
			key = match[1] + "\x00" + subject
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		conflicts = append(conflicts, ResolutionConflict{Package: subject, Breaks: match[1]})
	}
	return conflicts
}

func ResolutionFailure(err error) string {
	if err == nil {
		return ""
	}

	message := err.Error()
	if !strings.Contains(message, "unable to select packages") &&
		!strings.Contains(message, "unable to satisfy") {
		return ""
	}

	var lines []string
	for _, line := range strings.Split(message, "\n") {
		if strings.Contains(line, "satisfies: world[") {
			continue
		}
		lines = append(lines, strings.TrimPrefix(line, "ERROR: "))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func IsMissingPackage(failure string) bool {
	return strings.Contains(failure, "(no such package)")
}

type SimulateDelResult struct {
	Packages []string
	Blocked  map[string][]string // pkg -> list of blocking packages (e.g. "xovi" -> ["hide-dev-mode-icon"])
}

func (c *Client) SimulateDel(packages ...string) (*SimulateDelResult, error) {
	if len(packages) == 0 {
		return &SimulateDelResult{}, nil
	}
	cmd := fmt.Sprintf("%s del -s %s", VellumBin, executor.QuoteArgs(packages))
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

func (c *Client) SimulateDelRecursive(packages ...string) ([]string, error) {
	if len(packages) == 0 {
		return nil, nil
	}
	cmd := fmt.Sprintf("%s del -rs %s", VellumBin, executor.QuoteArgs(packages))
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		return nil, err
	}
	return parseSimulationOutput(output), nil
}

func (c *Client) DelRecursiveStreaming(onOutput func(line string), packages ...string) error {
	defer c.InvalidateInstalledCache()
	if len(packages) == 0 {
		return nil
	}
	cmd := fmt.Sprintf("%s del -r %s", VellumBin, executor.QuoteArgs(packages))
	return c.executor.ExecuteStreaming(cmd, onOutput)
}

// Matches apk simulation lines such as:
//
//	"( 1/20) Purging hide-dev-mode-icon (1.0.0-r0)"
//
//	"(10/20) Installing qt-resource-rebuilder (16.0.0-r0)"
var simulationLineRegex = regexp.MustCompile(`\(\s*\d+/\d+\)\s+(?:Installing|Upgrading|Downgrading|Purging)\s+([^\s]+)\s+\(`)

var purgeLineRegex = regexp.MustCompile(`\(\s*\d+/\d+\)\s+Purging\s+([^\s]+)\s+\(`)

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

type SimulatedPackage struct {
	Name    string
	Version string
}

type VersionChange struct {
	Name string
	From string
	To   string
}

var versionChangeLineRegex = regexp.MustCompile(`\(\s*\d+/\d+\)\s+(?:Upgrading|Downgrading)\s+([^\s]+)\s+\(([^\s)]+)\s+->\s+([^\s)]+)\)`)

func parseVersionChanges(output string) []VersionChange {
	var changes []VersionChange
	for _, line := range strings.Split(output, "\n") {
		if matches := versionChangeLineRegex.FindStringSubmatch(line); len(matches) >= 4 {
			changes = append(changes, VersionChange{Name: matches[1], From: matches[2], To: matches[3]})
		}
	}
	return changes
}

// Purging lines are left out: a purged package is not downloaded, installed or given its install hooks.
var simulationLineWithVersionRegex = regexp.MustCompile(`\(\s*\d+/\d+\)\s+(?:Installing|Upgrading|Downgrading)\s+([^\s]+)\s+\(([^)]+)\)`)

func parseSimulationOutputWithVersions(output string) []SimulatedPackage {
	var packages []SimulatedPackage
	for _, line := range strings.Split(output, "\n") {
		matches := simulationLineWithVersionRegex.FindStringSubmatch(line)
		if len(matches) >= 3 {
			version := matches[2]
			if parts := strings.Split(version, " -> "); len(parts) == 2 {
				version = parts[1]
			}
			packages = append(packages, SimulatedPackage{Name: matches[1], Version: version})
		}
	}
	return packages
}

func (c *Client) SimulateAddWithVersions(packages ...string) ([]SimulatedPackage, error) {
	if len(packages) == 0 {
		return nil, nil
	}
	cmd := fmt.Sprintf("%s add --simulate %s", VellumBin, executor.QuoteArgs(packages))
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		if output != "" {
			return nil, fmt.Errorf("%s", strings.TrimSpace(output))
		}
		return nil, err
	}
	return parseSimulationOutputWithVersions(output), nil
}

// The blockers list can wrap to more lines:
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

			if idx := strings.Index(line, ":"); idx > 0 && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "        ") {
				parts := strings.SplitN(trimmed, ":", 2)
				if len(parts) == 2 {
					currentPkg = strings.TrimSpace(parts[0])
					blockers := strings.Fields(parts[1])
					blocked[currentPkg] = append(blocked[currentPkg], blockers...)
				}
			} else if currentPkg != "" {
				blockers := strings.Fields(trimmed)
				blocked[currentPkg] = append(blocked[currentPkg], blockers...)
			}
		}
	}
	return blocked
}

func (c *Client) GetWorldPackages() ([]string, error) {
	cmd := fmt.Sprintf("cat %s/etc/apk/world 2>/dev/null", VellumRoot)
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		return nil, err
	}
	return parseWorldPackages(output), nil
}

// A world entry can carry a repository tag, a version constraint, or the "><checksum" pin apk writes for a package added from a local file.
func parseWorldPackages(output string) []string {
	var packages []string
	for _, line := range strings.Split(output, "\n") {
		pkg := strings.TrimSpace(line)
		if pkg == "" {
			continue
		}
		if idx := strings.IndexAny(pkg, "@=<>~"); idx > 0 {
			pkg = pkg[:idx]
		}
		packages = append(packages, pkg)
	}
	return packages
}

func (c *Client) GetReverseDeps(pkg string) ([]string, error) {
	cmd := fmt.Sprintf("%s info -r %s 2>/dev/null", VellumBin, executor.QuoteArg(pkg))
	output, err := c.executor.ExecuteWithOutput(cmd)
	if err != nil {
		return nil, err
	}

	var deps []string
	inDepList := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasSuffix(trimmed, "is required by:") {
			inDepList = true
			continue
		}
		if inDepList {
			name := parsePackageNameFromInfo(trimmed)
			if name != "" {
				deps = append(deps, name)
			}
		}
	}
	return deps, nil
}

var infoPackageRegex = regexp.MustCompile(`^(.+)-\d+[\d.]*`)

func parsePackageNameFromInfo(line string) string {
	matches := infoPackageRegex.FindStringSubmatch(line)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func (c *Client) UninstallVellum(removeAllPackages bool, onOutput func(line string)) error {
	var cmd string
	if removeAllPackages {
		onOutput("Removing Vellum and everything it installed…\n")
		cmd = fmt.Sprintf("%s self uninstall --all -y", VellumBin)
	} else {
		onOutput("Removing Vellum…\n")
		cmd = fmt.Sprintf("%s self uninstall -y", VellumBin)
	}
	return c.executor.ExecuteStreaming(cmd, onOutput)
}
