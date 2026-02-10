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

var installProgressRegex = regexp.MustCompile(`\(\s*(\d+)/(\d+)\)\s+Installing\s+([^\s]+)\s+\(`)

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
			Message:          "Installing packages...",
		})
	}

	var lastOutput string
	var currentPkg string
	var currentIdx int

	err := i.vellum.AddStreaming(func(line string) {
		lastOutput = line
		debug.Printf("[DEBUG] vellum output: %s\n", line)

		if strings.Contains(strings.ToLower(line), "dns:") {
			dnsError = true
		}

		if idx, total, pkgName := parseInstallProgress(line); pkgName != "" {
			currentPkg = pkgName
			currentIdx = idx - 1
			if onProgress != nil {
				onProgress(executor.ProgressInfo{
					CurrentComponent: pkgName,
					TotalComponents:  total,
					CurrentIndex:     currentIdx,
					Status:           executor.StatusInstalling,
					Message:          fmt.Sprintf("Installing %s (%d/%d)...", pkgName, idx, total),
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
		errMsg := fmt.Sprintf("Installation failed: %v (output: %s)", err, lastOutput)
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
	} else {
		debug.Printf("[DEBUG] All packages installed successfully\n")
		if onProgress != nil {
			onProgress(executor.ProgressInfo{
				CurrentComponent: packageNames[len(packageNames)-1],
				TotalComponents:  len(packageNames),
				CurrentIndex:     len(packageNames) - 1,
				Status:           executor.StatusCompleted,
				Message:          "All packages installed successfully",
			})
		}
	}

	// Run postInstall hooks for ALL packages (including dependencies)
	for _, pkgName := range allPackages {
		hooks := i.metadata.GetHooks(pkgName)
		if hooks != nil && hooks.PostInstall != "" {
			hookFunc := vellum.GetHook(hooks.PostInstall)
			if hookFunc != nil {
				debug.Printf("[DEBUG] Running postInstall hook for %s\n", pkgName)
				hookResult, err := hookFunc(ctx)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Post-install hook failed for %s: %v", pkgName, err))
					continue
				}
				if hookResult != nil && onHook != nil {
					if err := onHook(hookResult); err != nil {
						errors = append(errors, fmt.Sprintf("Hook callback failed for %s: %v", pkgName, err))
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
	ctx component.CommandContext,
	onProgress executor.ProgressCallback,
	onHook HookCallback,
) InstallResult {
	var errors []string

	debug.Printf("[DEBUG] Uninstall starting for packages: %v (all: %v, recursive: %v)\n", packageNames, allPackages, useRecursive)

	// Run preUninstall hooks for ALL packages first (before any actual uninstall)
	for _, pkgName := range allPackages {
		hooks := i.metadata.GetHooks(pkgName)
		if hooks != nil && hooks.PreUninstall != "" {
			hookFunc := vellum.GetHook(hooks.PreUninstall)
			if hookFunc != nil {
				debug.Printf("[DEBUG] Running preUninstall hook for %s\n", pkgName)
				hookResult, err := hookFunc(ctx)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Pre-uninstall hook failed for %s: %v", pkgName, err))
					continue
				}
				if hookResult != nil && onHook != nil {
					if err := onHook(hookResult); err != nil {
						errors = append(errors, fmt.Sprintf("Hook callback failed for %s: %v", pkgName, err))
						continue
					}
				}
			}
		}
	}

	// Perform actual uninstall for requested packages
	for idx, pkgName := range packageNames {
		pkg := i.metadata.GetPackage(pkgName)
		displayName := pkgName
		if pkg != nil {
			displayName = pkg.Name
		}

		if onProgress != nil {
			onProgress(executor.ProgressInfo{
				CurrentComponent: displayName,
				TotalComponents:  len(packageNames),
				CurrentIndex:     idx,
				Status:           executor.StatusInstalling,
				Message:          fmt.Sprintf("Uninstalling %s...", displayName),
			})
		}

		var err error
		if useRecursive {
			err = i.vellum.DelRecursiveStreaming(func(line string) {
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
		} else {
			err = i.vellum.DelStreaming(func(line string) {
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
		}

		if err != nil {
			errMsg := fmt.Sprintf("Uninstall failed for %s: %v", displayName, err)
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
				Message:          fmt.Sprintf("%s uninstalled successfully", displayName),
			})
		}
	}

	// Run postUninstall hooks for ALL packages (including orphaned deps)
	for _, pkgName := range allPackages {
		hooks := i.metadata.GetHooks(pkgName)
		if hooks != nil && hooks.PostUninstall != "" {
			hookFunc := vellum.GetHook(hooks.PostUninstall)
			if hookFunc != nil {
				debug.Printf("[DEBUG] Running postUninstall hook for %s\n", pkgName)
				hookResult, err := hookFunc(ctx)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Post-uninstall hook failed for %s: %v", pkgName, err))
					continue
				}
				if hookResult != nil && onHook != nil {
					if err := onHook(hookResult); err != nil {
						errors = append(errors, fmt.Sprintf("Hook callback failed for %s: %v", pkgName, err))
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
