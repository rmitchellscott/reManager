package main

import (
	"fmt"
	"regexp"
	"strings"

	"reManager/internal/debug"
	"reManager/internal/executor"
	"reManager/internal/support"
	"reManager/internal/vellum"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"

	"reManager/internal/commands"
	"reManager/internal/component"
)

type DeviceService struct {
	*core
}

type XochitlStatus struct {
	Running         bool   `json:"running"`
	XoviActive      bool   `json:"xoviActive"`
	ActiveLauncher  string `json:"activeLauncher"`
	CurrentLauncher string `json:"currentLauncher"`
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

func (s *DeviceService) GetDeviceInfo() map[string]string {
	if s.inDemoMode() {
		return demoDeviceInfo()
	}
	s.mu.Lock()

	info := make(map[string]string)

	if s.client == nil {
		s.mu.Unlock()
		return info
	}

	info["machine"] = s.connectedDeviceType.DisplayName()
	info["firmware"] = s.connectedFirmware

	vellumClient := s.vellumClient
	s.mu.Unlock()

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

func (s *DeviceService) GetUpdateServiceStatus() (support.UpdateServiceStatus, error) {
	s.mu.Lock()
	demo, demoAutoUpdates := s.demo, s.demoAutoUpdates
	connected := s.client != nil
	s.mu.Unlock()

	status := support.UpdateServiceStatus{
		Enabled: false,
		Running: false,
	}

	if demo {
		return support.UpdateServiceStatus{Enabled: demoAutoUpdates, Running: demoAutoUpdates}, nil
	}

	if !connected {
		debug.Println("[DEBUG] GetUpdateServiceStatus: client is nil")
		return status, errNotConnected
	}

	output, err := s.runCommand("systemctl is-enabled update-engine.service")
	debug.Printf("[DEBUG] GetUpdateServiceStatus: is-enabled output=%q, err=%v\n", output, err)
	if isTransportError(err) {
		return status, err
	}
	if err == nil {
		status.Enabled = strings.TrimSpace(output) == "enabled"
	}

	output, err = s.runCommand("systemctl is-active update-engine.service")
	debug.Printf("[DEBUG] GetUpdateServiceStatus: is-active output=%q, err=%v\n", output, err)
	if isTransportError(err) {
		return status, err
	}
	if err == nil {
		status.Running = strings.TrimSpace(output) == "active"
	}

	output, err = s.runCommand("grep -m1 '^AutoUpdate=' /home/root/.config/remarkable/xochitl.conf")
	debug.Printf("[DEBUG] GetUpdateServiceStatus: AutoUpdate output=%q, err=%v\n", output, err)
	if isTransportError(err) {
		return status, err
	}
	if err == nil && strings.TrimSpace(output) == "AutoUpdate=false" {
		status.Enabled = false
	}

	debug.Printf("[DEBUG] GetUpdateServiceStatus: returning enabled=%v, running=%v\n", status.Enabled, status.Running)
	return status, nil
}

func (s *DeviceService) GetXochitlStatus() (XochitlStatus, error) {
	return s.xochitlStatus(true)
}

func (s *DeviceService) GetXochitlStatusQuick() (XochitlStatus, error) {
	return s.xochitlStatus(false)
}

func (s *DeviceService) xochitlStatus(includeLauncher bool) (XochitlStatus, error) {
	if s.inDemoMode() {
		return s.demoXochitlStatus(), nil
	}
	if s.getClient() == nil {
		return XochitlStatus{}, errNotConnected
	}

	probe := "systemctl is-active xochitl"
	if includeLauncher {
		probe = fmt.Sprintf("%s; echo %[2]s; [ -x %[3]s ] && %[3]s active-launcher 2>/dev/null; echo %[2]s; [ -x %[3]s ] && %[3]s current-launcher 2>/dev/null",
			probe, vellum.LauncherProbeMarker, vellum.LauncherctlBin)
	}
	output, err := s.runCommand(probe)
	if isTransportError(err) {
		return XochitlStatus{}, err
	}

	sections := strings.Split(output, vellum.LauncherProbeMarker)
	running := strings.TrimSpace(sections[0]) == "active"

	activeLauncher := ""
	if len(sections) > 1 {
		activeLauncher = vellum.ParseLauncherName(sections[1])
	}

	currentLauncher := ""
	if len(sections) > 2 {
		currentLauncher = vellum.ParseLauncherName(sections[2])
	}

	xoviActive := false
	if running {
		_, err := s.runCommand(`tr '\0' '\n' < /proc/$(pidof xochitl)/environ | grep -q LD_PRELOAD.*xovi`)
		if isTransportError(err) {
			return XochitlStatus{}, err
		}
		xoviActive = err == nil
	}

	return XochitlStatus{
		Running:         running,
		XoviActive:      xoviActive,
		ActiveLauncher:  activeLauncher,
		CurrentLauncher: currentLauncher,
	}, nil
}

func (s *DeviceService) CheckHashtabVersion() (HashtabVersionStatus, error) {
	status := HashtabVersionStatus{}

	s.mu.Lock()
	if s.demo {
		s.mu.Unlock()
		return demoHashtabStatus(), nil
	}
	if s.client == nil {
		s.mu.Unlock()
		return status, errNotConnected
	}
	s.mu.Unlock()

	output, err := s.runCommand("grep REMARKABLE_RELEASE_VERSION /usr/share/remarkable/update.conf")
	if err == nil {
		if parts := strings.SplitN(output, "=", 2); len(parts) == 2 {
			status.FirmwareVersion = strings.TrimSpace(parts[1])
		}
	} else if !isTransportError(err) {
		output, err = s.runCommand("grep IMG_VERSION /etc/os-release")
		if err == nil {
			if parts := strings.SplitN(output, "=", 2); len(parts) == 2 {
				status.FirmwareVersion = strings.Trim(strings.TrimSpace(parts[1]), "\"")
			}
		}
	}
	if isTransportError(err) {
		return status, err
	}

	checker := vellum.NewHashtabChecker(&wailsExecutor{core: s.core})

	exists, err := checker.CheckHashtabExists()
	if isTransportError(err) {
		return status, err
	}
	if err != nil || !exists {
		return status, nil
	}
	status.Installed = true

	hashtabVersion, err := checker.GetHashtabVersion()
	if isTransportError(err) {
		return status, err
	}
	if err != nil {
		return status, nil
	}
	status.HashtabVersion = hashtabVersion

	status.NeedsRebuild = status.FirmwareVersion != "" &&
		status.HashtabVersion != "" &&
		status.FirmwareVersion != status.HashtabVersion

	return status, nil
}

func (s *DeviceService) getDeviceTimezone() (string, error) {
	s.mu.Lock()
	demo, demoTimezone := s.demo, s.demoTimezone
	connected := s.client != nil
	s.mu.Unlock()

	if demo {
		return demoTimezone, nil
	}

	if !connected {
		return "", fmt.Errorf("not connected")
	}

	output, err := s.runCommand("readlink /etc/localtime | sed 's|.*/zoneinfo/||'")
	if err != nil {
		return "", fmt.Errorf("failed to get timezone: %w", err)
	}

	return strings.TrimSpace(output), nil
}

func (s *DeviceService) GetTimezoneStatus() (TimezoneStatus, error) {
	status := TimezoneStatus{}

	s.mu.Lock()
	if s.demo {
		status.DeviceTimezone = s.demoTimezone
		s.mu.Unlock()
		return status, nil
	}
	if s.client == nil {
		s.mu.Unlock()
		return status, errNotConnected
	}
	deviceID := s.connectedDeviceID
	s.mu.Unlock()

	output, err := s.runCommand("readlink /etc/localtime | sed 's|.*/zoneinfo/||'")
	if err != nil {
		if isTransportError(err) {
			return status, err
		}
		return status, nil
	}
	status.DeviceTimezone = strings.TrimSpace(output)

	if deviceID != "" && s.deviceStore != nil {
		device, err := s.deviceStore.Get(deviceID)
		if err == nil && device.Timezone != "" {
			status.SavedTimezone = device.Timezone
			status.NeedsUpdate = device.Timezone != status.DeviceTimezone
		}
	}

	return status, nil
}

func (s *DeviceService) SaveDeviceTimezone(timezone string) error {
	s.mu.Lock()
	deviceID := s.connectedDeviceID
	s.mu.Unlock()

	if deviceID == "" {
		return fmt.Errorf("no device connected")
	}
	if s.deviceStore == nil {
		return fmt.Errorf("device store not initialized")
	}

	return s.deviceStore.UpdateTimezone(deviceID, timezone)
}

var timezoneNamePattern = regexp.MustCompile(`^[A-Za-z0-9_+-]+(/[A-Za-z0-9_+-]+)*$`)

func (s *DeviceService) SetDeviceTimezone(opID, timezone, deviceType string) {
	s.runOperation(opID, "Setting the timezone", func(op *Operation) (any, error) {
		if !timezoneNamePattern.MatchString(timezone) {
			return nil, fmt.Errorf("%q is not a timezone name", timezone)
		}

		s.mu.Lock()
		demo := s.demo
		connected := s.client != nil
		s.mu.Unlock()
		if !connected && !demo {
			return nil, errNotConnected
		}

		dev := rmdevice.Type(deviceType)
		if err := s.claimWritableRoot(dev); err != nil {
			return nil, err
		}
		defer s.releaseWritableRootClaim(dev)

		cmdResults := []component.CommandResult{
			{
				Script:      fmt.Sprintf("systemctl restart systemd-timedated && timedatectl set-timezone %s", executor.QuoteArg(timezone)),
				Description: "Set reMarkable timezone",
			},
		}

		var operationErr error
		if s.logger != nil {
			s.operationLog = s.logger.StartCommandLog(s.connectedDeviceID, "set-timezone")
			defer func() {
				s.operationLog.WriteExitCode(operationErr)
				s.operationLog.Close()
				s.operationLog = nil
			}()
		}

		if operationErr = s.runOperationCommandsWithWritableRoot(op, dev, cmdResults); operationErr != nil {
			return nil, fmt.Errorf("could not set the timezone: %w", operationErr)
		}

		if demo {
			s.mu.Lock()
			s.demoTimezone = timezone
			s.mu.Unlock()
		}
		return timezone, nil
	})
}

func (s *DeviceService) GetDeviceArchitecture(deviceType string) string {
	if s.inDemoMode() {
		return string(rmdevice.Aarch64)
	}
	_, arch := s.getConnectedDevice()
	return commands.GetDownloadArch(arch)
}
