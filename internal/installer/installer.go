package installer

import (
	"fmt"
	"regexp"
	"strings"

	"reManager/internal/component"
	"reManager/internal/debug"
	"reManager/internal/executor"
	"reManager/internal/vellum"
)

var installProgressRegex = regexp.MustCompile(`\(\s*(\d+)/(\d+)\)\s+(?:Installing|Upgrading|Downgrading)\s+([^\s]+)\s+\(`)

func parseInstallProgress(line string) (idx int, total int, pkgName string) {
	matches := installProgressRegex.FindStringSubmatch(line)
	if len(matches) >= 4 {
		fmt.Sscanf(matches[1], "%d", &idx)
		fmt.Sscanf(matches[2], "%d", &total)
		pkgName = matches[3]
	}
	return
}

type InstallResult struct {
	Success  bool
	Errors   []string
	DNSError bool
}

var ErrHookDeclined = fmt.Errorf("user declined hook")

type HookCallback func(result *component.HookExecutionResult) error

type Installer struct {
	vellum   *vellum.Client
	metadata *vellum.MetadataStore
	executor executor.CommandExecutor
}

func NewInstaller(v *vellum.Client, meta *vellum.MetadataStore, exec executor.CommandExecutor) *Installer {
	return &Installer{
		vellum:   v,
		metadata: meta,
		executor: exec,
	}
}

func (i *Installer) Install(
	packageNames []string,
	allPackages []string,
	ctx component.CommandContext,
	onProgress executor.ProgressCallback,
	onHook HookCallback,
) InstallResult {
	var errors []string
	var dnsError bool

	debug.Printf("[DEBUG] Install starting for packages: %v (all: %v)\n", packageNames, allPackages)

	if onProgress != nil {
		onProgress(executor.ProgressInfo{
			CurrentComponent: packageNames[0],
			TotalComponents:  len(packageNames),
			CurrentIndex:     0,
			Status:           executor.StatusInstalling,
			Message:          "Installing packages…",
		})
	}

	var diagnostics vellumDiagnostics
	var currentPkg string
	var currentIdx int

	err := i.vellum.AddStreaming(func(line string) {
		diagnostics.observe(line)
		debug.Printf("[DEBUG] vellum output: %s\n", line)

		if strings.Contains(strings.ToLower(line), "dns:") {
			dnsError = true
		}

		if idx, total, pkgName := parseInstallProgress(line); pkgName != "" {
			// apk installs one package at a time, so announcing the next means the previous one is done.
			if currentPkg != "" && onProgress != nil {
				onProgress(executor.ProgressInfo{
					CurrentComponent: currentPkg,
					TotalComponents:  total,
					CurrentIndex:     currentIdx,
					Status:           executor.StatusCompleted,
				})
			}
			currentPkg = pkgName
			currentIdx = idx - 1
			if onProgress != nil {
				onProgress(executor.ProgressInfo{
					CurrentComponent: pkgName,
					TotalComponents:  total,
					CurrentIndex:     currentIdx,
					Status:           executor.StatusInstalling,
					Message:          fmt.Sprintf("Installing %s (%d/%d)…", pkgName, idx, total),
				})
			}
		} else if onProgress != nil && currentPkg != "" {
			onProgress(executor.ProgressInfo{
				CurrentComponent: currentPkg,
				TotalComponents:  len(packageNames),
				CurrentIndex:     currentIdx,
				Status:           executor.StatusInstalling,
				Message:          line,
			})
		}
	}, packageNames...)

	if err != nil {
		errMsg := fmt.Sprintf("Installation failed: %s", diagnostics.explain(err))
		debug.Printf("[DEBUG] Install error: %s\n", errMsg)
		errors = append(errors, errMsg)
		if onProgress != nil {
			onProgress(executor.ProgressInfo{
				CurrentComponent: currentPkg,
				TotalComponents:  len(packageNames),
				CurrentIndex:     currentIdx,
				Status:           executor.StatusError,
				Message:          errMsg,
			})
		}
		// A hook for a package apk did not install would act on something not there, such as a hashtable for a missing xovi.
		return InstallResult{Success: false, Errors: errors, DNSError: dnsError}
	} else {
		debug.Printf("[DEBUG] All packages installed successfully\n")
		if onProgress != nil {
			lastPkg, lastIdx := currentPkg, currentIdx
			if lastPkg == "" {
				lastPkg, lastIdx = packageNames[len(packageNames)-1], len(packageNames)-1
			}
			onProgress(executor.ProgressInfo{
				CurrentComponent: lastPkg,
				TotalComponents:  len(packageNames),
				CurrentIndex:     lastIdx,
				Status:           executor.StatusCompleted,
				Message:          "All packages installed",
			})
		}
	}

	for _, pkgName := range allPackages {
		hooks := i.metadata.GetHooks(pkgName)
		if hooks != nil && hooks.PostInstall != "" {
			hookFunc := vellum.GetHook(hooks.PostInstall)
			if hookFunc != nil {
				debug.Printf("[DEBUG] Running postInstall hook for %s\n", pkgName)
				hookResult, err := hookFunc(ctx)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Post-install script failed for %s: %v", pkgName, err))
					continue
				}
				if hookResult != nil && hookResult.DialogConfig != nil {
					hookResult.DialogConfig.InstallFlow = true
					hookResult.DialogConfig.Title = "Build hashtable"
					hookResult.DialogConfig.ConfirmText = "Build"
					hookResult.DialogConfig.CancelText = "Later"
				}
				if hookResult != nil && onHook != nil {
					if err := onHook(hookResult); err != nil {
						if err != ErrHookDeclined {
							errors = append(errors, fmt.Sprintf("Script callback failed for %s: %v", pkgName, err))
						}
						continue
					}
				}
			}
		}
	}

	return InstallResult{
		Success:  len(errors) == 0,
		Errors:   errors,
		DNSError: dnsError,
	}
}

func (i *Installer) Uninstall(
	packageNames []string,
	allPackages []string,
	useRecursive bool,
	batch bool,
	ctx component.CommandContext,
	onProgress executor.ProgressCallback,
	onHook HookCallback,
) InstallResult {
	var errors []string

	debug.Printf("[DEBUG] Uninstall starting for packages: %v (all: %v, recursive: %v, batch: %v)\n", packageNames, allPackages, useRecursive, batch)

	for _, pkgName := range allPackages {
		hooks := i.metadata.GetHooks(pkgName)
		if hooks != nil && hooks.PreUninstall != "" {
			hookFunc := vellum.GetHook(hooks.PreUninstall)
			if hookFunc != nil {
				debug.Printf("[DEBUG] Running preUninstall hook for %s\n", pkgName)
				hookResult, err := hookFunc(ctx)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Pre-removal script failed for %s: %v", pkgName, err))
					continue
				}
				if hookResult != nil && onHook != nil {
					if err := onHook(hookResult); err != nil {
						if err != ErrHookDeclined {
							errors = append(errors, fmt.Sprintf("Script callback failed for %s: %v", pkgName, err))
						}
						continue
					}
				}
			}
		}
	}

	errorsBeforeRemoval := len(errors)
	if batch {
		label := strings.Join(packageNames, ", ")
		if onProgress != nil {
			onProgress(executor.ProgressInfo{
				CurrentComponent: label,
				TotalComponents:  1,
				CurrentIndex:     0,
				Status:           executor.StatusInstalling,
				Message:          fmt.Sprintf("Removing %d packages…", len(packageNames)),
			})
		}

		var diagnostics vellumDiagnostics
		err := i.vellum.DelStreaming(func(line string) {
			diagnostics.observe(line)
			if onProgress != nil {
				onProgress(executor.ProgressInfo{
					CurrentComponent: label,
					TotalComponents:  1,
					CurrentIndex:     0,
					Status:           executor.StatusInstalling,
					Message:          line,
				})
			}
		}, packageNames...)

		if err != nil {
			errMsg := fmt.Sprintf("Removal failed: %s", diagnostics.explain(err))
			errors = append(errors, errMsg)
			reportError(onProgress, label, 1, 0, errMsg)
		} else if onProgress != nil {
			onProgress(executor.ProgressInfo{
				CurrentComponent: label,
				TotalComponents:  1,
				CurrentIndex:     0,
				Status:           executor.StatusCompleted,
				Message:          "Packages removed",
			})
		}
	} else if !useRecursive {
		i.uninstallTogether(packageNames, onProgress, &errors)
	} else {
		removed := map[string]bool{}
		for idx, pkgName := range packageNames {
			pkg := i.metadata.GetPackage(pkgName)
			displayName := pkgName
			if pkg != nil {
				displayName = pkg.Name
			}

			if removed[pkgName] {
				if onProgress != nil {
					onProgress(executor.ProgressInfo{
						CurrentComponent: displayName,
						TotalComponents:  len(packageNames),
						CurrentIndex:     idx,
						Status:           executor.StatusCompleted,
						Message:          fmt.Sprintf("%s was already removed", displayName),
					})
				}
				continue
			}

			if onProgress != nil {
				onProgress(executor.ProgressInfo{
					CurrentComponent: displayName,
					TotalComponents:  len(packageNames),
					CurrentIndex:     idx,
					Status:           executor.StatusInstalling,
					Message:          fmt.Sprintf("Removing %s…", displayName),
				})
			}

			var perPkgDiagnostics vellumDiagnostics
			err := i.vellum.DelRecursiveStreaming(func(line string) {
				perPkgDiagnostics.observe(line)
				if name := parsePurgedPackage(line); name != "" {
					removed[name] = true
				}
				if onProgress != nil {
					onProgress(executor.ProgressInfo{
						CurrentComponent: displayName,
						TotalComponents:  len(packageNames),
						CurrentIndex:     idx,
						Status:           executor.StatusInstalling,
						Message:          line,
					})
				}
			}, pkgName)

			if err != nil {
				errMsg := fmt.Sprintf("Could not remove %s: %s", displayName, perPkgDiagnostics.explain(err))
				errors = append(errors, errMsg)
				reportError(onProgress, displayName, len(packageNames), idx, errMsg)
				continue
			}

			if onProgress != nil {
				onProgress(executor.ProgressInfo{
					CurrentComponent: displayName,
					TotalComponents:  len(packageNames),
					CurrentIndex:     idx,
					Status:           executor.StatusCompleted,
					Message:          fmt.Sprintf("%s removed", displayName),
				})
			}
		}
	}

	if len(errors) > errorsBeforeRemoval {
		return InstallResult{Success: false, Errors: errors}
	}

	for _, pkgName := range allPackages {
		hooks := i.metadata.GetHooks(pkgName)
		if hooks != nil && hooks.PostUninstall != "" {
			hookFunc := vellum.GetHook(hooks.PostUninstall)
			if hookFunc != nil {
				debug.Printf("[DEBUG] Running postUninstall hook for %s\n", pkgName)
				hookResult, err := hookFunc(ctx)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Post-removal script failed for %s: %v", pkgName, err))
					continue
				}
				if hookResult != nil && onHook != nil {
					if err := onHook(hookResult); err != nil {
						if err != ErrHookDeclined {
							errors = append(errors, fmt.Sprintf("Script callback failed for %s: %v", pkgName, err))
						}
						continue
					}
				}
			}
		}
	}

	return InstallResult{
		Success: len(errors) == 0,
		Errors:  errors,
	}
}

// One apk transaction: removing packages one at a time fails when an earlier one is still needed by a later one (appload before pitwall).
func (i *Installer) uninstallTogether(packageNames []string, onProgress executor.ProgressCallback, errors *[]string) {
	index := make(map[string]int, len(packageNames))
	for idx, name := range packageNames {
		index[name] = idx
	}
	removed := map[string]bool{}
	nextPending := func() string {
		for _, name := range packageNames {
			if !removed[name] {
				return name
			}
		}
		return ""
	}
	report := func(name string, status executor.ProgressStatus, message string) {
		if onProgress != nil {
			onProgress(executor.ProgressInfo{
				CurrentComponent: name,
				TotalComponents:  len(packageNames),
				CurrentIndex:     index[name],
				Status:           status,
				Message:          message,
			})
		}
	}

	report(packageNames[0], executor.StatusInstalling, fmt.Sprintf("Removing %s…", strings.Join(packageNames, ", ")))

	var diagnostics vellumDiagnostics
	err := i.vellum.DelStreaming(func(line string) {
		diagnostics.observe(line)
		if name := parsePurgedPackage(line); name != "" {
			if _, requested := index[name]; requested && !removed[name] {
				removed[name] = true
				report(name, executor.StatusCompleted, fmt.Sprintf("%s removed", name))
				return
			}
		}
		if pending := nextPending(); pending != "" {
			report(pending, executor.StatusInstalling, line)
		}
	}, packageNames...)

	if err != nil {
		errMsg := fmt.Sprintf("Could not remove %s: %s", strings.Join(packageNames, ", "), diagnostics.explain(err))
		*errors = append(*errors, errMsg)
		for _, name := range packageNames {
			if !removed[name] {
				report(name, executor.StatusError, errMsg)
			}
		}
		return
	}
	for _, name := range packageNames {
		if !removed[name] {
			report(name, executor.StatusCompleted, fmt.Sprintf("%s removed", name))
		}
	}
}

func reportError(onProgress executor.ProgressCallback, name string, total, idx int, msg string) {
	if onProgress != nil {
		onProgress(executor.ProgressInfo{
			CurrentComponent: name,
			TotalComponents:  total,
			CurrentIndex:     idx,
			Status:           executor.StatusError,
			Message:          msg,
		})
	}
}

const maxDiagnosticLines = 5

var purgedLineRegex = regexp.MustCompile(`\(\s*\d+/\d+\)\s+(?:Purging|Removing)\s+(\S+)`)

func parsePurgedPackage(line string) string {
	if m := purgedLineRegex.FindStringSubmatch(line); len(m) >= 2 {
		return m[1]
	}
	return ""
}

type vellumDiagnostics struct {
	lines    []string
	lastLine string
}

func (d *vellumDiagnostics) observe(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	d.lastLine = trimmed
	if strings.HasPrefix(trimmed, "ERROR:") || strings.HasPrefix(trimmed, "WARNING:") {
		d.lines = append(d.lines, trimmed)
		if len(d.lines) > maxDiagnosticLines {
			d.lines = d.lines[len(d.lines)-maxDiagnosticLines:]
		}
	}
}

func (d *vellumDiagnostics) explain(err error) string {
	if len(d.lines) > 0 {
		return strings.Join(d.lines, "\n")
	}
	if d.lastLine != "" {
		return fmt.Sprintf("%v (output: %s)", err, d.lastLine)
	}
	return fmt.Sprintf("%v", err)
}
