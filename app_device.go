package main

import (
	"fmt"
	"strings"

	"reManager/internal/debug"
	"reManager/internal/support"
	"reManager/internal/vellum"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	rmdevice "github.com/rmitchellscott/remarkable-go/device"

	"reManager/internal/commands"
	"reManager/internal/component"
)

type XochitlStatus struct {
	Running    bool `json:"running"`
	XoviActive bool `json:"xoviActive"`
}

type HashtabVersionStatus struct {
	Installed       bool   `json:"installed"`
	HashtabVersion  string `json:"hashtabVersion"`
	FirmwareVersion string `json:"firmwareVersion"`
	NeedsRebuild    bool   `json:"needsRebuild"`
}

type TimezoneStatus struct {
	DeviceTimezone string `json:"deviceTimezone"`
	SavedTimezone  string `json:"savedTimezone"`
	NeedsUpdate    bool   `json:"needsUpdate"`
}

type ReenableStatusResult struct {
	Status string `json:"status"`
}

func (a *App) GetDeviceInfo() map[string]string {
	a.mu.Lock()

	info := make(map[string]string)

	if a.client == nil {
		a.mu.Unlock()
		return info
	}

	info["machine"] = a.connectedDeviceType.DisplayName()
	info["firmware"] = a.connectedFirmware

	vellumClient := a.vellumClient
	a.mu.Unlock()

	if vellumClient != nil {
		installed, _ := vellumClient.IsInstalled()
		if installed {
			info["vellum_installed"] = "yes"
		} else {
			info["vellum_installed"] = "no"
		}
	}

	return info
}

func (a *App) GetUpdateServiceStatus() support.UpdateServiceStatus {
	a.mu.Lock()
	defer a.mu.Unlock()

	status := support.UpdateServiceStatus{
		Enabled: false,
		Running: false,
	}

	if a.client == nil {
		debug.Println("[DEBUG] GetUpdateServiceStatus: client is nil")
		return status
	}

	output, err := a.runCommand("systemctl is-enabled update-engine.service")
	debug.Printf("[DEBUG] GetUpdateServiceStatus: is-enabled output=%q, err=%v\n", output, err)
	if err == nil {
		status.Enabled = strings.TrimSpace(output) == "enabled"
	}

	output, err = a.runCommand("systemctl is-active update-engine.service")
	debug.Printf("[DEBUG] GetUpdateServiceStatus: is-active output=%q, err=%v\n", output, err)
	if err == nil {
		status.Running = strings.TrimSpace(output) == "active"
	}

	debug.Printf("[DEBUG] GetUpdateServiceStatus: returning enabled=%v, running=%v\n", status.Enabled, status.Running)
	return status
}

func (a *App) GetXochitlStatus() XochitlStatus {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client == nil {
		return XochitlStatus{}
	}

	output, err := a.runCommand("systemctl is-active xochitl")
	running := err == nil && strings.TrimSpace(output) == "active"

	_, err = a.runCommand("test -f /etc/systemd/system/xochitl.service.d/00-xovi.conf")
	xoviActive := err == nil

	return XochitlStatus{Running: running, XoviActive: xoviActive}
}

func (a *App) CheckHashtabVersion() HashtabVersionStatus {
	status := HashtabVersionStatus{}

	a.mu.Lock()
	if a.client == nil {
		a.mu.Unlock()
		return status
	}

	if output, err := a.runCommand("grep REMARKABLE_RELEASE_VERSION /usr/share/remarkable/update.conf"); err == nil {
		if parts := strings.SplitN(output, "=", 2); len(parts) == 2 {
			status.FirmwareVersion = strings.TrimSpace(parts[1])
		}
	} else if output, err := a.runCommand("grep IMG_VERSION /etc/os-release"); err == nil {
		if parts := strings.SplitN(output, "=", 2); len(parts) == 2 {
			status.FirmwareVersion = strings.Trim(strings.TrimSpace(parts[1]), "\"")
		}
	}
	a.mu.Unlock()

	checker := vellum.NewHashtabChecker(&wailsExecutor{app: a})

	exists, err := checker.CheckHashtabExists()
	if err != nil || !exists {
		return status
	}
	status.Installed = true

	hashtabVersion, err := checker.GetHashtabVersion()
	if err != nil {
		return status
	}
	status.HashtabVersion = hashtabVersion

	status.NeedsRebuild = status.FirmwareVersion != "" &&
		status.HashtabVersion != "" &&
		status.FirmwareVersion != status.HashtabVersion

	return status
}

func (a *App) GetDeviceTimezone() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client == nil {
		return "", fmt.Errorf("not connected")
	}

	output, err := a.runCommand("readlink /etc/localtime | sed 's|.*/zoneinfo/||'")
	if err != nil {
		return "", fmt.Errorf("failed to get timezone: %w", err)
	}

	return strings.TrimSpace(output), nil
}

func (a *App) GetTimezoneStatus() TimezoneStatus {
	status := TimezoneStatus{}

	a.mu.Lock()
	if a.client == nil {
		a.mu.Unlock()
		return status
	}

	output, err := a.runCommand("readlink /etc/localtime | sed 's|.*/zoneinfo/||'")
	if err != nil {
		a.mu.Unlock()
		return status
	}
	status.DeviceTimezone = strings.TrimSpace(output)

	deviceID := a.connectedDeviceID
	a.mu.Unlock()

	if deviceID != "" && a.deviceStore != nil {
		device, err := a.deviceStore.Get(deviceID)
		if err == nil && device.Timezone != "" {
			status.SavedTimezone = device.Timezone
			status.NeedsUpdate = device.Timezone != status.DeviceTimezone
		}
	}

	return status
}

func (a *App) SaveDeviceTimezone(timezone string) error {
	a.mu.Lock()
	deviceID := a.connectedDeviceID
	a.mu.Unlock()

	if deviceID == "" {
		return fmt.Errorf("no device connected")
	}
	if a.deviceStore == nil {
		return fmt.Errorf("device store not initialized")
	}

	return a.deviceStore.UpdateTimezone(deviceID, timezone)
}

func (a *App) SetDeviceTimezone(timezone string, deviceType string) {
	go func() {
		a.mu.Lock()
		if a.client == nil {
			a.mu.Unlock()
			runtime.EventsEmit(a.ctx, "timezone:error", "Not connected")
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}
		a.mu.Unlock()

		dev := rmdevice.Type(deviceType)
		if err := a.acquireWriteableRoot(dev); err != nil {
			runtime.EventsEmit(a.ctx, "timezone:error", err.Error())
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}
		defer a.releaseWriteableRoot(dev)

		cmdResults := []component.CommandResult{
			{
				Script:      fmt.Sprintf("systemctl restart systemd-timedated && timedatectl set-timezone %s", timezone),
				Description: "Set device timezone",
			},
		}

		if dev.IsPaperPro() {
			cmdResults = commands.WrapWithWriteableRoot(cmdResults, dev)
		}

		var operationErr error
		if a.logger != nil {
			a.operationLog = a.logger.StartCommandLog(a.connectedDeviceID, "set-timezone")
			defer func() {
				a.operationLog.WriteExitCode(operationErr)
				a.operationLog.Close()
				a.operationLog = nil
			}()
		}

		for _, cmd := range cmdResults {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", cmd.Script))

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

			a.RunCommandWithOutput(cmd.Script, cmd.RequiresPTY)
			success := <-done
			unsub()

			if !success {
				operationErr = fmt.Errorf("command failed: %s", cmd.Script)
				runtime.EventsEmit(a.ctx, "timezone:error", "Failed to set timezone")
				return
			}
		}

		runtime.EventsEmit(a.ctx, "timezone:complete", timezone)
	}()
}

func (a *App) GetReenableStatus() ReenableStatusResult {
	if a.vellumClient == nil {
		return ReenableStatusResult{}
	}
	status, err := a.vellumClient.ReenableStatus()
	if err != nil {
		debug.Printf("[DEBUG] GetReenableStatus error: %v\n", err)
		return ReenableStatusResult{}
	}
	debug.Printf("[DEBUG] GetReenableStatus: %s\n", status)
	return ReenableStatusResult{Status: status}
}

func (a *App) GetDeviceDisplayName(machine string) string {
	return a.connectedDeviceType.DisplayName()
}

func (a *App) GetDeviceArchitecture(deviceType string) string {
	return commands.GetDownloadArch(a.connectedDeviceArch)
}
