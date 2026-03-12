package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"reManager/internal/component"
	"reManager/internal/device"
	"reManager/internal/executor"
	"reManager/internal/installer"
	"reManager/internal/vellum"
)

var (
	host     string
	keyPath  string
	password string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "remanager",
		Short: "Manage vellum packages on reMarkable devices",
		Long:  `reManager is a CLI tool for installing and managing vellum packages on reMarkable tablets.`,
	}

	rootCmd.PersistentFlags().StringVarP(&host, "host", "H", "10.11.99.1", "Device hostname or IP address")
	rootCmd.PersistentFlags().StringVarP(&keyPath, "key", "k", "", "Path to SSH private key")
	rootCmd.PersistentFlags().StringVarP(&password, "password", "p", "", "SSH password (will prompt if not provided)")

	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(installCmd())
	rootCmd.AddCommand(uninstallCmd())
	rootCmd.AddCommand(maintenanceCmd())
	rootCmd.AddCommand(bootstrapCmd())
	rootCmd.AddCommand(importPDFCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available packages",
		Run: func(cmd *cobra.Command, args []string) {
			metadata := vellum.NewMetadataStore()
			if err := metadata.Load(); err != nil {
				fmt.Printf("Error loading metadata: %v\n", err)
				return
			}

			packages := metadata.GetAllPackages()

			fmt.Println("Available packages:")
			fmt.Println()

			categories := make(map[string][]vellum.Package)
			for _, pkg := range packages {
				cats := pkg.Categories
				if len(cats) == 0 {
					cats = []string{"other"}
				}
				for _, cat := range cats {
					categories[cat] = append(categories[cat], pkg)
				}
			}

			for cat, pkgs := range categories {
				fmt.Printf("\n[%s]\n", cat)
				for _, pkg := range pkgs {
					deps := ""
					if len(pkg.Depends) > 0 {
						deps = fmt.Sprintf(" (requires: %s)", strings.Join(pkg.Depends, ", "))
					}
					fmt.Printf("  %-25s %s%s\n", pkg.Name, pkg.Description, deps)
					if pkg.UpstreamAuthor != "" {
						fmt.Printf("  %-25s by %s\n", "", pkg.UpstreamAuthor)
					}
				}
			}
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show installed packages on connected device",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, deviceType, err := connect()
			if err != nil {
				return err
			}
			defer client.Close()

			exec := executor.NewSSHExecutor(client)
			vellumClient := vellum.NewClient(exec)

			fmt.Printf("Connected to %s (%s)\n\n", host, device.GetDisplayName(deviceType))

			installed, err := vellumClient.IsInstalled()
			if err != nil {
				return fmt.Errorf("error checking vellum: %w", err)
			}

			if !installed {
				fmt.Println("Vellum is not installed. Run 'remanager bootstrap' to install it.")
				return nil
			}

			packages, err := vellumClient.List()
			if err != nil {
				return fmt.Errorf("error listing packages: %w", err)
			}

			fmt.Println("Installed packages:")
			fmt.Println()
			for _, pkg := range packages {
				fmt.Printf("  %s\n", pkg)
			}

			return nil
		},
	}
}

func bootstrapCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bootstrap",
		Short: "Install vellum package manager on the device",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, deviceType, err := connect()
			if err != nil {
				return err
			}
			defer client.Close()

			exec := executor.NewSSHExecutor(client)
			vellumClient := vellum.NewClient(exec)

			fmt.Printf("Connected to %s (%s)\n\n", host, device.GetDisplayName(deviceType))

			installed, err := vellumClient.IsInstalled()
			if err != nil {
				return fmt.Errorf("error checking vellum: %w", err)
			}

			if installed {
				fmt.Println("Vellum is already installed.")
				return nil
			}

			arch := device.GetArchitecture(component.DeviceType(deviceType))

			fmt.Println("Installing vellum...")
			err = vellumClient.BootstrapOfflineWithPackages(client, arch, func(line string) {
				fmt.Print(line)
			})

			if err != nil {
				return fmt.Errorf("bootstrap failed: %w", err)
			}

			fmt.Println("\nVellum installed successfully!")
			return nil
		},
	}
}

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install [packages...]",
		Short: "Install packages on the device",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, deviceType, err := connect()
			if err != nil {
				return err
			}
			defer client.Close()

			exec := executor.NewSSHExecutor(client)
			vellumClient := vellum.NewClient(exec)

			installed, err := vellumClient.IsInstalled()
			if err != nil {
				return fmt.Errorf("error checking vellum: %w", err)
			}

			if !installed {
				return fmt.Errorf("vellum is not installed. Run 'remanager bootstrap' first")
			}

			metadata := vellum.NewMetadataStore()
			if err := metadata.Load(); err != nil {
				return fmt.Errorf("error loading metadata: %w", err)
			}

			arch := device.GetArchitecture(component.DeviceType(deviceType))
			ctx := component.CommandContext{
				Arch:   arch,
				Device: component.DeviceType(deviceType),
			}

			inst := installer.NewInstaller(vellumClient, metadata, exec)

			// Get full package list including dependencies via simulation
			allPackages, err := vellumClient.FetchURLs(args...)
			if err != nil {
				fmt.Printf("Warning: could not get dependency list: %v\n", err)
				allPackages = nil
			}
			// Parse package names from URLs (simplified - just use args if parsing fails)
			var allPkgNames []string
			if allPackages == nil {
				allPkgNames = args
			} else {
				// For CLI, just use the requested packages since we don't have proxy logic here
				allPkgNames = args
			}

			result := inst.Install(
				args,
				allPkgNames,
				ctx,
				func(progress executor.ProgressInfo) {
					fmt.Printf("[%d/%d] %s: %s\n", progress.CurrentIndex+1, progress.TotalComponents, progress.CurrentComponent, progress.Message)
				},
				func(hookResult *component.HookExecutionResult) error {
					if hookResult.DialogConfig != nil {
						fmt.Printf("\n%s\n", hookResult.DialogConfig.Title)
						fmt.Println(hookResult.DialogConfig.Message)
						for _, step := range hookResult.DialogConfig.Steps {
							fmt.Printf("  - %s\n", step)
						}
						fmt.Print("\nProceed? [y/N]: ")

						var response string
						fmt.Scanln(&response)
						if strings.ToLower(response) != "y" {
							return fmt.Errorf("user cancelled")
						}

						if hookResult.Command != nil {
							fmt.Printf("$ %s\n", hookResult.Command.Script)
							if err := exec.Execute([]component.CommandResult{*hookResult.Command}); err != nil {
								return err
							}
						}
					}
					return nil
				},
			)

			if result.Success {
				fmt.Println("\nInstallation complete!")
			} else {
				fmt.Println("\nInstallation failed:")
				for _, err := range result.Errors {
					fmt.Printf("  - %s\n", err)
				}
				return fmt.Errorf("installation failed")
			}

			return nil
		},
	}
}

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall [packages...]",
		Short: "Uninstall packages from the device",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, deviceType, err := connect()
			if err != nil {
				return err
			}
			defer client.Close()

			exec := executor.NewSSHExecutor(client)
			vellumClient := vellum.NewClient(exec)

			installed, err := vellumClient.IsInstalled()
			if err != nil {
				return fmt.Errorf("error checking vellum: %w", err)
			}

			if !installed {
				return fmt.Errorf("vellum is not installed")
			}

			metadata := vellum.NewMetadataStore()
			if err := metadata.Load(); err != nil {
				return fmt.Errorf("error loading metadata: %w", err)
			}

			arch := device.GetArchitecture(component.DeviceType(deviceType))
			ctx := component.CommandContext{
				Arch:   arch,
				Device: component.DeviceType(deviceType),
			}

			inst := installer.NewInstaller(vellumClient, metadata, exec)

			// Get full package list including orphaned deps via simulation
			var allPackages []string
			simResult, err := vellumClient.SimulateDel(args...)
			if err != nil {
				fmt.Printf("Warning: could not get full uninstall list: %v\n", err)
				allPackages = args
			} else if len(simResult.Blocked) > 0 {
				// Packages blocked by dependents - for CLI, prompt user
				fmt.Println("\nThe following packages cannot be removed due to dependencies:")
				for pkg, blockers := range simResult.Blocked {
					fmt.Printf("  %s is required by: %s\n", pkg, strings.Join(blockers, ", "))
				}
				fmt.Print("\nRemove these dependent packages as well? [y/N]: ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" {
					return fmt.Errorf("uninstall cancelled")
				}
				// Use recursive deletion
				allPackages, err = vellumClient.SimulateDelRecursive(args...)
				if err != nil {
					fmt.Printf("Warning: could not get recursive uninstall list: %v\n", err)
					allPackages = args
				}
			} else {
				allPackages = simResult.Packages
				if len(allPackages) == 0 {
					allPackages = args
				}
			}

			// Determine if we need recursive deletion
			useRecursive := len(simResult.Blocked) > 0

			result := inst.Uninstall(
				args,
				allPackages,
				useRecursive,
				false,
				ctx,
				func(progress executor.ProgressInfo) {
					fmt.Printf("[%d/%d] %s: %s\n", progress.CurrentIndex+1, progress.TotalComponents, progress.CurrentComponent, progress.Message)
				},
				func(hookResult *component.HookExecutionResult) error {
					if hookResult.DialogConfig != nil {
						fmt.Printf("\n%s\n", hookResult.DialogConfig.Title)
						fmt.Println(hookResult.DialogConfig.Message)
						for _, step := range hookResult.DialogConfig.Steps {
							fmt.Printf("  - %s\n", step)
						}
						fmt.Print("\nProceed? [y/N]: ")

						var response string
						fmt.Scanln(&response)
						if strings.ToLower(response) != "y" {
							return fmt.Errorf("user cancelled")
						}

						if hookResult.Command != nil {
							fmt.Printf("$ %s\n", hookResult.Command.Script)
							if err := exec.Execute([]component.CommandResult{*hookResult.Command}); err != nil {
								return err
							}
						}
					}
					return nil
				},
			)

			if result.Success {
				fmt.Println("\nUninstall complete!")
			} else {
				fmt.Println("\nUninstall failed:")
				for _, err := range result.Errors {
					fmt.Printf("  - %s\n", err)
				}
				return fmt.Errorf("uninstall failed")
			}

			return nil
		},
	}
}

func maintenanceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "maintenance <package> <command>",
		Short: "Run maintenance command for a package",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgName := args[0]
			commandID := args[1]

			metadata := vellum.NewMetadataStore()
			if err := metadata.Load(); err != nil {
				return fmt.Errorf("error loading metadata: %w", err)
			}

			commands := metadata.GetMaintenanceCommands(pkgName)
			if commands == nil {
				return fmt.Errorf("no maintenance commands for package: %s", pkgName)
			}

			var maintCmd *vellum.MaintenanceCommand
			for i := range commands {
				if commands[i].ID == commandID {
					maintCmd = &commands[i]
					break
				}
			}
			if maintCmd == nil {
				fmt.Printf("Available maintenance commands for %s:\n", pkgName)
				for _, c := range commands {
					fmt.Printf("  %s - %s\n", c.ID, c.Description)
				}
				return fmt.Errorf("command not found: %s", commandID)
			}

			client, deviceType, err := connect()
			if err != nil {
				return err
			}
			defer client.Close()

			exec := executor.NewSSHExecutor(client)

			if maintCmd.Hook != "" {
				hookFunc := vellum.GetHook(maintCmd.Hook)
				if hookFunc != nil {
					arch := device.GetArchitecture(component.DeviceType(deviceType))
					ctx := component.CommandContext{
						Arch:   arch,
						Device: component.DeviceType(deviceType),
					}

					hookResult, err := hookFunc(ctx)
					if err != nil {
						return fmt.Errorf("hook error: %w", err)
					}

					if hookResult != nil && hookResult.DialogConfig != nil {
						fmt.Printf("\n%s\n", hookResult.DialogConfig.Title)
						fmt.Println(hookResult.DialogConfig.Message)
						for _, step := range hookResult.DialogConfig.Steps {
							fmt.Printf("  - %s\n", step)
						}
						fmt.Print("\nProceed? [y/N]: ")

						var response string
						fmt.Scanln(&response)
						if strings.ToLower(response) != "y" {
							return fmt.Errorf("user cancelled")
						}
					}
				}
			}

			fmt.Printf("Running %s %s...\n", pkgName, commandID)
			fmt.Printf("$ %s\n", maintCmd.Command)

			if maintCmd.RequiresTerminal {
				if err := exec.ExecuteStreaming(maintCmd.Command, func(line string) {
					fmt.Print(line)
				}); err != nil {
					return fmt.Errorf("command failed: %w", err)
				}
			} else {
				output, err := exec.ExecuteWithOutput(maintCmd.Command)
				if err != nil {
					return fmt.Errorf("command failed: %w", err)
				}
				if output != "" {
					fmt.Print(output)
				}
			}

			fmt.Println("\nDone!")
			return nil
		},
	}
}

func connect() (*ssh.Client, string, error) {
	var authMethods []ssh.AuthMethod

	if keyPath != "" {
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read key file: %w", err)
		}

		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			if strings.Contains(err.Error(), "passphrase") {
				fmt.Print("Enter key passphrase: ")
				passphrase, err := term.ReadPassword(int(syscall.Stdin))
				fmt.Println()
				if err != nil {
					return nil, "", fmt.Errorf("failed to read passphrase: %w", err)
				}
				signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, passphrase)
				if err != nil {
					return nil, "", fmt.Errorf("failed to parse key: %w", err)
				}
			} else {
				return nil, "", fmt.Errorf("failed to parse key: %w", err)
			}
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else {
		if password == "" {
			fmt.Print("Enter SSH password: ")
			pwBytes, err := term.ReadPassword(int(syscall.Stdin))
			fmt.Println()
			if err != nil {
				return nil, "", fmt.Errorf("failed to read password: %w", err)
			}
			password = string(pwBytes)
		}
		authMethods = append(authMethods, ssh.Password(password))
	}

	config := &ssh.ClientConfig{
		User:            "root",
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Config: ssh.Config{
			KeyExchanges: []string{
				"curve25519-sha256",
				"curve25519-sha256@libssh.org",
				"ecdh-sha2-nistp256",
				"ecdh-sha2-nistp384",
				"ecdh-sha2-nistp521",
				"diffie-hellman-group14-sha256",
				"diffie-hellman-group14-sha1",
				"diffie-hellman-group1-sha1",
			},
			Ciphers: []string{
				"chacha20-poly1305@openssh.com",
				"aes256-ctr",
				"aes128-ctr",
				"aes256-cbc",
				"aes128-cbc",
				"3des-cbc",
			},
			MACs: []string{
				"hmac-sha2-256",
				"hmac-sha1",
			},
		},
		// Explicit host key algorithms required for Dropbear 2022.83 compatibility
		HostKeyAlgorithms: []string{
			ssh.KeyAlgoED25519,
			ssh.KeyAlgoRSASHA256,
			ssh.KeyAlgoRSA,
		},
	}

	addr := host
	if !strings.Contains(host, ":") {
		addr = host + ":22"
	}

	fmt.Printf("Connecting to %s...\n", addr)

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, "", fmt.Errorf("failed to connect: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput("cat /sys/devices/soc0/machine")
	if err != nil {
		return client, "unknown", nil
	}

	machine := strings.TrimSpace(string(output))
	var deviceType string
	switch {
	case strings.Contains(machine, "reMarkable 1"):
		deviceType = "rm1"
	case strings.Contains(machine, "reMarkable 2"):
		deviceType = "rm2"
	case strings.Contains(machine, "Ferrari"):
		deviceType = "rmpp"
	case strings.Contains(machine, "Chiappa"):
		deviceType = "rmppm"
	default:
		deviceType = machine
	}

	return client, deviceType, nil
}
