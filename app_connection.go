package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"reManager/internal/debug"
	apperrors "reManager/internal/errors"
	"reManager/internal/sshagent"
	"reManager/internal/sshconfig"
	"reManager/internal/storage"
	"reManager/internal/vellum"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"
	rmexecutor "github.com/rmitchellscott/remarkable-go/executor"
	rmfilesystem "github.com/rmitchellscott/remarkable-go/filesystem"
)

type ConnectionResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	Code      string `json:"code,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
	Device    string `json:"device,omitempty"`
}

type SSHKey struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type SavedDeviceInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	AuthType      string `json:"authType"`
	KeyPath       string `json:"keyPath,omitempty"`
	LastConnected int64  `json:"lastConnected,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
}

func (a *App) GetDefaultSSHKeys() []SSHKey {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	sshDir := filepath.Join(home, ".ssh")
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return nil
	}

	var keys []SSHKey
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "id_") && !strings.HasSuffix(name, ".pub") {
			keyPath := filepath.Join(sshDir, name)
			if info, err := os.Stat(keyPath); err == nil && info.Mode().IsRegular() {
				keys = append(keys, SSHKey{
					Path: keyPath,
					Name: name,
				})
			}
		}
	}

	return keys
}

func (a *App) SelectKeyFile() string {
	path, err := openFileDialog(a.ctx, "Select SSH Private Key", "")
	if err != nil {
		return ""
	}
	return path
}

func (a *App) GetSavedDevices() []SavedDeviceInfo {
	if a.deviceStore == nil {
		return []SavedDeviceInfo{}
	}

	devices, err := a.deviceStore.GetAll()
	if err != nil {
		fmt.Printf("Error getting saved devices: %v\n", err)
		return []SavedDeviceInfo{}
	}

	result := make([]SavedDeviceInfo, len(devices))
	for i, d := range devices {
		result[i] = SavedDeviceInfo{
			ID:            d.ID,
			Name:          d.Name,
			Host:          d.Host,
			AuthType:      d.AuthType,
			KeyPath:       d.KeyPath,
			LastConnected: d.LastConnected,
			Timezone:      d.Timezone,
		}
	}
	return result
}

func (a *App) SaveDevice(id, name, host, authType, password, keyPath, keyPassphrase string) (string, error) {
	if a.deviceStore == nil {
		return "", fmt.Errorf("device store not initialized")
	}

	device := storage.SavedDevice{
		ID:            id,
		Name:          name,
		Host:          host,
		AuthType:      authType,
		KeyPath:       keyPath,
		LastConnected: time.Now().Unix(),
	}

	return a.deviceStore.Save(device, password, keyPassphrase)
}

func (a *App) DeleteSavedDevice(id string) error {
	if a.deviceStore == nil {
		return fmt.Errorf("device store not initialized")
	}
	if err := a.deviceStore.Delete(id); err != nil {
		return err
	}
	if a.deviceInfoCache != nil {
		_ = a.deviceInfoCache.Delete(id)
	}
	return nil
}

func (a *App) UpdateDeviceName(id string, name string) error {
	if a.deviceStore == nil {
		return fmt.Errorf("device store not initialized")
	}
	return a.deviceStore.UpdateName(id, name)
}

func (a *App) ConnectToSavedDevice(id string) ConnectionResult {
	if a.deviceStore == nil {
		return ConnectionResult{
			Success:   false,
			Message:   "Device store not initialized.",
			Code:      apperrors.ErrStorageFailed,
			Retryable: false,
		}
	}

	device, err := a.deviceStore.Get(id)
	if err != nil {
		return ConnectionResult{
			Success:   false,
			Message:   "Saved device not found. It may have been deleted.",
			Code:      apperrors.ErrDeviceNotFound,
			Retryable: false,
		}
	}

	var result ConnectionResult
	if device.AuthType == "agent" {
		result = a.ConnectWithAuth(device.Host, "agent", "", "")
	} else if device.AuthType == "key" {
		passphrase, _ := a.deviceStore.GetKeyPassphrase(id)
		result = a.ConnectWithAuth(device.Host, "key", passphrase, device.KeyPath)
	} else {
		password, err := a.deviceStore.GetPassword(id)
		if err != nil {
			return ConnectionResult{
				Success:   false,
				Message:   "Could not retrieve password. Please reconnect and save the device again.",
				Code:      apperrors.ErrKeyringFailed,
				Retryable: false,
			}
		}
		result = a.ConnectWithAuth(device.Host, "password", password, "")
	}

	if result.Success {
		a.deviceStore.UpdateLastConnected(id, time.Now().Unix())

		a.mu.Lock()
		a.connectedDeviceID = id
		a.mu.Unlock()

		a.startConnectionMonitor()

		if settings, err := a.settingsStore.Load(); err == nil && settings.PreventSleep {
			debug.Println("[DEBUG] ConnectToSavedDevice: auto-starting prevent sleep")
			if err := a.StartPreventSleep(); err != nil {
				debug.Printf("[DEBUG] ConnectToSavedDevice: StartPreventSleep failed: %v\n", err)
			}
		}
	}

	return result
}

func (a *App) CheckVellumInstalled() bool {
	if a.vellumClient == nil {
		return false
	}
	installed, err := a.vellumClient.IsInstalled()
	if err != nil {
		return false
	}
	return installed
}

func (a *App) BootstrapVellum() {
	go func() {
		a.mu.Lock()
		sshClient := a.client
		a.mu.Unlock()

		if sshClient == nil {
			runtime.EventsEmit(a.ctx, "vellum:bootstrap-error", "Not connected")
			return
		}

		if a.vellumClient == nil {
			a.vellumClient = vellum.NewClient(&wailsExecutor{app: a})
		}

		_, arch, _, err := a.detectDevice()
		if err != nil {
			runtime.EventsEmit(a.ctx, "vellum:bootstrap-error", fmt.Sprintf("Failed to detect device: %v", err))
			return
		}

		runtime.EventsEmit(a.ctx, "vellum:bootstrap-start", nil)

		err = a.vellumClient.BootstrapOfflineWithPackages(sshClient, arch, func(line string) {
			runtime.EventsEmit(a.ctx, "vellum:bootstrap-output", line)
		})

		if err != nil {
			runtime.EventsEmit(a.ctx, "vellum:bootstrap-error", err.Error())
			return
		}

		runtime.EventsEmit(a.ctx, "vellum:bootstrap-complete", nil)
	}()
}

func (a *App) CheckPackageInstalled(pkgName string) bool {
	if a.vellumClient == nil {
		return false
	}
	installed, err := a.vellumClient.IsPackageInstalled(pkgName)
	if err != nil {
		return false
	}
	return installed
}

func (a *App) Connect(host, password string) ConnectionResult {
	return a.ConnectWithAuth(host, "password", password, "")
}

func (a *App) ConnectWithKey(host, keyPath, passphrase string) ConnectionResult {
	return a.ConnectWithAuth(host, "key", passphrase, keyPath)
}

func (a *App) ConnectWithAgent(host string) ConnectionResult {
	return a.ConnectWithAuth(host, "agent", "", "")
}

func (a *App) IsSSHAgentAvailable() bool {
	return sshagent.IsAvailable(a.sshAgentSocketPath())
}

func (a *App) sshAgentSocketPath() string {
	if a.settingsStore == nil {
		return ""
	}
	settings, err := a.settingsStore.Load()
	if err != nil {
		return ""
	}
	return settings.SSHAgentSocketPath
}

func (a *App) CancelConnect() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.connectCancel != nil {
		a.connectCancel()
		a.connectCancel = nil
	}
}

func (a *App) dialWithContext(ctx context.Context, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	timeout := 10 * time.Second
	if a.fastDialMode {
		timeout = 5 * time.Second
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	nonRetryableKeywords := []string{
		"passphrase",
		"permission denied",
		"unable to authenticate",
		"ssh: handshake failed",
		"no auth",
		"authentication failed",
	}

	for _, keyword := range nonRetryableKeywords {
		if strings.Contains(errStr, keyword) {
			return false
		}
	}

	retryableKeywords := []string{
		"no route to host",
		"connection refused",
		"i/o timeout",
		"connection reset by peer",
		"network is unreachable",
		"host is down",
		"connection timed out",
		"broken pipe",
	}

	for _, keyword := range retryableKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() || netErr.Temporary() {
			return true
		}
	}

	return false
}

func (a *App) dialWithContextWithRetry(ctx context.Context, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	var maxRetries int
	var backoffDurations []time.Duration

	if a.fastDialMode {
		maxRetries = 0
		backoffDurations = []time.Duration{}
	} else {
		maxRetries = 3
		backoffDurations = []time.Duration{
			2 * time.Second,
			4 * time.Second,
			8 * time.Second,
		}
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		debug.Printf("[%s] dialWithContextWithRetry attempt %d/%d to %s\n", time.Now().Format("15:04:05.000"), attempt+1, maxRetries+1, addr)
		client, err := a.dialWithContext(ctx, addr, config)
		if err == nil {
			debug.Printf("[%s] dialWithContextWithRetry attempt %d/%d succeeded\n", time.Now().Format("15:04:05.000"), attempt+1, maxRetries+1)
			return client, nil
		}

		debug.Printf("[%s] dialWithContextWithRetry attempt %d/%d failed: %v\n", time.Now().Format("15:04:05.000"), attempt+1, maxRetries+1, err)
		lastErr = err

		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		if !isRetryableError(err) {
			debug.Printf("[%s] Error not retryable, giving up\n", time.Now().Format("15:04:05.000"))
			return nil, err
		}

		if attempt == maxRetries {
			break
		}

		backoffDuration := backoffDurations[attempt]
		debug.Printf("[%s] dialWithContextWithRetry waiting %v before retry\n", time.Now().Format("15:04:05.000"), backoffDuration)

		select {
		case <-time.After(backoffDuration):
			continue
		case <-ctx.Done():
			return nil, context.Canceled
		}
	}

	return nil, lastErr
}

func (a *App) ConnectWithAuth(host, authType, secret, keyPath string) ConnectionResult {
	a.stopConnectionMonitor()

	a.mu.Lock()

	a.connectedDeviceID = ""

	if a.connectCancel != nil {
		a.connectCancel()
	}

	if a.client != nil {
		a.client.Close()
		a.client = nil
	}
	if a.agentConn != nil {
		a.agentConn.Close()
		a.agentConn = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.connectCancel = cancel

	a.mu.Unlock()

	var authMethods []ssh.AuthMethod

	if authType == "agent" {
		conn, authMethod, err := sshagent.Connect(a.sshAgentSocketPath())
		if err != nil {
			return ConnectionResult{
				Success:   false,
				Message:   err.Error(),
				Code:      apperrors.ErrAuthFailed,
				Retryable: false,
			}
		}
		a.mu.Lock()
		a.agentConn = conn
		a.mu.Unlock()
		authMethods = append(authMethods, authMethod)
	} else if authType == "key" {
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			ue := apperrors.Classify(err)
			return ConnectionResult{
				Success:   false,
				Message:   ue.Message,
				Code:      ue.Code,
				Retryable: ue.Retryable,
			}
		}

		var signer ssh.Signer
		if secret != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(secret))
		} else {
			signer, err = ssh.ParsePrivateKey(keyData)
		}
		if err != nil {
			ue := apperrors.Classify(err)
			return ConnectionResult{
				Success:   false,
				Message:   ue.Message,
				Code:      ue.Code,
				Retryable: ue.Retryable,
			}
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else {
		authMethods = append(authMethods, ssh.Password(secret))
	}

	config := sshconfig.NewClientConfig("root", authMethods)

	addr := host
	if !strings.Contains(host, ":") {
		addr = host + ":22"
	}

	client, err := a.dialWithContextWithRetry(ctx, addr, config)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return ConnectionResult{
				Success:   false,
				Message:   "Connection cancelled.",
				Code:      apperrors.ErrOperationCancelled,
				Retryable: false,
			}
		}
		ue := apperrors.Classify(err)
		return ConnectionResult{
			Success:   false,
			Message:   ue.Message,
			Code:      ue.Code,
			Retryable: ue.Retryable,
		}
	}

	a.mu.Lock()
	a.client = client
	a.connectCancel = nil
	debug.Println("[DEBUG] SSH connected, creating vellum client")
	a.vellumClient = vellum.NewClient(&wailsExecutor{app: a})
	a.mu.Unlock()

	if a.logger != nil {
		a.logger.LogConnection("connected", host)
	}

	debug.Println("[DEBUG] Detecting device...")
	deviceType, deviceArch, firmware, err := a.detectDevice()
	debug.Printf("[DEBUG] Device detected: %s (%s), err: %v\n", deviceType, deviceArch, err)
	if err != nil {
		return ConnectionResult{
			Success: true,
			Message: "Connected (could not detect device type)",
			Device:  "unknown",
		}
	}

	go func() {
		a.mu.Lock()
		vc := a.vellumClient
		a.mu.Unlock()
		if vc == nil {
			return
		}

		if err := a.metadata.Refresh(); err != nil {
			debug.Printf("[DEBUG] Metadata refresh failed: %v\n", err)
		}

		debug.Println("[DEBUG] Checking if vellum is installed...")
		installed, err := vc.IsInstalled()
		debug.Printf("[DEBUG] Vellum installed: %v, err: %v\n", installed, err)

		warnings := map[string]interface{}{}
		vellumReady := false
		var osVersionStored string

		if err == nil && !installed {
			runtime.EventsEmit(a.ctx, "vellum:bootstrap-prompt", nil)
		} else if err == nil && installed {
			valid, missing, verr := vc.ValidateInstall()
			debug.Printf("[DEBUG] Vellum validation: valid=%v, missing=%v, err=%v\n", valid, missing, verr)
			runtime.EventsEmit(a.ctx, "vellum:ready", nil)
			if verr == nil && !valid {
				runtime.EventsEmit(a.ctx, "vellum:broken-install", missing)
			} else {
				vellumReady = true

				reenableStatus, reenableErr := vc.ReenableStatus()
				if reenableErr == nil && reenableStatus != "" {
					debug.Printf("[DEBUG] Reenable status: %s\n", reenableStatus)
					warnings["reenableStatus"] = reenableStatus
				}
			}
		}

		hashtabStatus := a.CheckHashtabVersion()
		debug.Printf("[DEBUG] Hashtab check: installed=%v, hashtabVersion=%s, firmwareVersion=%s, needsRebuild=%v\n",
			hashtabStatus.Installed, hashtabStatus.HashtabVersion, hashtabStatus.FirmwareVersion, hashtabStatus.NeedsRebuild)
		if hashtabStatus.NeedsRebuild {
			warnings["hashtabMismatch"] = hashtabStatus
		}

		var installedVersions map[string]string
		if vellumReady {
			installedVersions, _ = vc.ListInstalledWithVersions()
		}

		if !hashtabStatus.Installed && installedVersions != nil {
			if _, hasQRB := installedVersions["qt-resource-rebuilder"]; hasQRB {
				warnings["hashtabMissing"] = true
			}
		}

		updateStatus := a.GetUpdateServiceStatus()
		debug.Printf("[DEBUG] Auto-update check: enabled=%v, running=%v\n", updateStatus.Enabled, updateStatus.Running)
		if updateStatus.Enabled || updateStatus.Running {
			warnings["autoUpdateEnabled"] = updateStatus
		}

		xochitlStatus := a.GetXochitlStatus()
		debug.Printf("[DEBUG] Xochitl check: running=%v, xoviActive=%v\n", xochitlStatus.Running, xochitlStatus.XoviActive)
		if !xochitlStatus.Running {
			warnings["xochitlNotRunning"] = true
		}
		if xochitlStatus.Running && !xochitlStatus.XoviActive && !hashtabStatus.NeedsRebuild && hashtabStatus.Installed && installedVersions != nil {
			if _, hasXovi := installedVersions["xovi"]; hasXovi {
				warnings["xoviNotRunning"] = true
			}
		}

		timezoneStatus := a.GetTimezoneStatus()
		debug.Printf("[DEBUG] Timezone check: device=%s, saved=%s, needsUpdate=%v\n",
			timezoneStatus.DeviceTimezone, timezoneStatus.SavedTimezone, timezoneStatus.NeedsUpdate)

		if timezoneStatus.DeviceTimezone != "" && timezoneStatus.SavedTimezone == "" {
			a.mu.Lock()
			deviceID := a.connectedDeviceID
			a.mu.Unlock()
			if deviceID != "" && a.deviceStore != nil {
				if err := a.deviceStore.UpdateTimezone(deviceID, timezoneStatus.DeviceTimezone); err == nil {
					debug.Printf("[DEBUG] Auto-saved device timezone: %s\n", timezoneStatus.DeviceTimezone)
					timezoneStatus.SavedTimezone = timezoneStatus.DeviceTimezone
				}
			}
		}
		if timezoneStatus.DeviceTimezone != "" {
			warnings["timezoneStatus"] = timezoneStatus
		}
		if timezoneStatus.NeedsUpdate {
			warnings["timezoneMismatch"] = timezoneStatus
		}

		if vellumReady {
			osState, err := vc.GetOSVersionState()
			debug.Printf("[DEBUG] GetOSVersionState: stored=%q, current=%q, mismatch=%v, err=%v\n", osState.StoredVersion, osState.CurrentVersion, osState.Mismatch, err)
			if err == nil {
				osVersionStored = osState.StoredVersion
			}
			if err == nil && osState.Mismatch {
				debug.Printf("[DEBUG] OS mismatch detected: stored=%s, current=%s\n",
					osState.StoredVersion, osState.CurrentVersion)
				warnings["osMismatch"] = map[string]string{
					"prevVersion": osState.StoredVersion,
					"newVersion":  osState.CurrentVersion,
				}

				var filteredInstalled []string
				if installedVersions != nil {
					for name := range installedVersions {
						if !hiddenPackages[name] {
							filteredInstalled = append(filteredInstalled, name)
						}
					}
				}

				compat, compatErr := vc.CheckOSCompatibility(osState.CurrentVersion)

				compatStatus := PackageCompatibilityStatus{
					InstalledPackages: filteredInstalled,
					CurrentOsVersion:  osState.CurrentVersion,
					StoredOsVersion:   osState.StoredVersion,
				}

				if compatErr != nil || compat == nil || compat.FetchFailed {
					compatStatus.FetchFailed = true
					compatStatus.CompatiblePackages = filteredInstalled
					compatStatus.IncompatiblePackages = []string{}
				} else {
					statusMap := make(map[string]string)
					for _, name := range append(append(compat.Compatible, compat.Incompatible...), compat.NoConstraint...) {
						pkg := a.metadata.GetPackage(name)
						if pkg != nil && pkg.Status != "" && pkg.Status != "maintained" {
							statusMap[name] = pkg.Status
						}
					}
					compatStatus.CompatiblePackages = append(compat.Compatible, compat.NoConstraint...)
					compatStatus.IncompatiblePackages = compat.Incompatible
					compatStatus.StatusMap = statusMap
				}

				warnings["compatibilityStatus"] = compatStatus
			}

			runtime.EventsEmit(a.ctx, "connect:warnings", warnings)

			if _, hasMismatch := warnings["osMismatch"]; !hasMismatch {
				settings, _ := a.settingsStore.Load()
				proxyEnabled := settings == nil || settings.ProxyMode

				if proxyEnabled {
					a.mu.Lock()
					sshClient := a.client
					a.mu.Unlock()
					if sshClient != nil {
						proxy := vellum.NewProxy(vc, sshClient, string(deviceArch))
						_ = proxy.UploadAPKINDEX(func(msg string) {
							debug.Printf("[DEBUG] Upgrade check APKINDEX: %s\n", msg)
						})
					}
				}

				upgradeResult, simErr := vc.SimulateUpgrade()

				if simErr == nil && upgradeResult.HasUpgrades {
					runtime.EventsEmit(a.ctx, "packages:upgrades-available", map[string]interface{}{
						"packages": upgradeResult.Packages,
					})
				}
			}
		}

		if a.deviceInfoCache != nil {
			a.mu.Lock()
			cacheDeviceID := a.connectedDeviceID
			a.mu.Unlock()
			if cacheDeviceID != "" {
				cached := storage.CachedDeviceInfo{
					Timezone:             timezoneStatus.DeviceTimezone,
					HashtabInstalled:     hashtabStatus.Installed,
					HashtabVersion:       hashtabStatus.HashtabVersion,
					HashtabNeedsRebuild:  hashtabStatus.NeedsRebuild,
					UpdateServiceEnabled: updateStatus.Enabled,
					UpdateServiceRunning: updateStatus.Running,
					OSVersion:            osVersionStored,
					CachedAt:             time.Now().Unix(),
				}
				a.mu.Lock()
				cached.MachineType = a.connectedDeviceType.DisplayName()
				cached.FirmwareVersion = a.connectedFirmware
				a.mu.Unlock()
				if installedVersions != nil {
					cached.InstalledPackages = installedVersions
				}
				if partInfo, err := a.GetPartitionInfo(); err == nil {
					cached.PartitionInfo = partInfo
				}
				if err := a.deviceInfoCache.Set(cacheDeviceID, cached); err != nil {
					debug.Printf("[DEBUG] Failed to cache device info: %v\n", err)
				}
			}
		}

		if !vellumReady {
			runtime.EventsEmit(a.ctx, "connect:warnings", warnings)
		}

		if a.settingsStore != nil {
			guideSettings, _ := a.settingsStore.Load()
			if guideSettings != nil && !guideSettings.SuppressGuideOffer {
				status := a.CheckUserGuide()
				if !status.Skipped {
					if !status.Installed {
						runtime.EventsEmit(a.ctx, "guide:offer", map[string]string{"type": "install"})
					} else if status.NeedsUpdate {
						runtime.EventsEmit(a.ctx, "guide:offer", map[string]string{"type": "update"})
					}
				}
			}
		}
	}()

	a.mu.Lock()
	a.connectedDeviceType = deviceType
	a.connectedDeviceArch = deviceArch
	a.connectedFirmware = firmware
	a.mu.Unlock()

	return ConnectionResult{
		Success: true,
		Message: "Connected successfully",
		Device:  string(deviceType),
	}
}

func (a *App) detectDevice() (rmdevice.Type, rmdevice.Architecture, string, error) {
	client := a.getClient()

	if client == nil {
		return rmdevice.Unknown, "", "", fmt.Errorf("not connected")
	}

	fs, err := rmfilesystem.NewSSH(client)
	if err != nil {
		return rmdevice.Unknown, "", "", fmt.Errorf("failed to create filesystem: %w", err)
	}
	defer fs.Close()

	deviceType, err := rmdevice.Detect(fs)
	if err != nil {
		return rmdevice.Unknown, "", "", fmt.Errorf("failed to detect device: %w", err)
	}

	exec := rmexecutor.NewSSH(client)
	arch, err := rmdevice.DetectArchitecture(exec)
	if err != nil {
		return deviceType, "", "", fmt.Errorf("failed to detect architecture: %w", err)
	}

	firmware, _ := rmdevice.DetectVersion(fs)

	return deviceType, arch, firmware, nil
}

func (a *App) Disconnect() {
	if a.logger != nil {
		a.logger.LogConnection("disconnect", "user initiated")
	}

	a.stopConnectionMonitor()

	a.reconnectMu.Lock()
	a.reconnecting = false
	a.reconnectMu.Unlock()

	a.StopPreventSleep()
	a.StopShell()

	a.mu.Lock()
	defer a.mu.Unlock()

	a.connectedDeviceID = ""
	a.connectedDeviceType = ""
	a.connectedDeviceArch = ""
	a.connectedFirmware = ""
	a.writeableRootBusy = false

	if a.client != nil {
		a.client.Close()
		a.client = nil
	}
	if a.agentConn != nil {
		a.agentConn.Close()
		a.agentConn = nil
	}
	a.vellumClient = nil
}

func (a *App) IsConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client != nil
}

func (a *App) acquireWriteableRoot(deviceType rmdevice.Type) error {
	if !deviceType.IsPaperPro() {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.writeableRootBusy {
		return fmt.Errorf("another operation is modifying the filesystem")
	}
	a.writeableRootBusy = true
	runtime.EventsEmit(a.ctx, "writeable-root:busy", true)
	return nil
}

func (a *App) releaseWriteableRoot(deviceType rmdevice.Type) {
	if !deviceType.IsPaperPro() {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.writeableRootBusy = false
	runtime.EventsEmit(a.ctx, "writeable-root:busy", false)
}

func (a *App) IsWriteableRootBusy() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.writeableRootBusy
}

const (
	keepaliveInterval    = 15 * time.Second
	keepaliveTimeout     = 5 * time.Second
	maxReconnectAttempts = 5
)

var reconnectBackoff = []time.Duration{
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
	16 * time.Second,
	30 * time.Second,
}

func (a *App) startConnectionMonitor() {
	a.mu.Lock()
	if a.keepaliveStop != nil {
		a.mu.Unlock()
		return
	}
	a.keepaliveStop = make(chan struct{})
	a.mu.Unlock()

	go a.connectionMonitorLoop()
}

func (a *App) stopConnectionMonitor() {
	a.mu.Lock()
	if a.keepaliveStop != nil {
		close(a.keepaliveStop)
		a.keepaliveStop = nil
	}
	a.mu.Unlock()
}

func (a *App) connectionMonitorLoop() {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	a.mu.Lock()
	stopCh := a.keepaliveStop
	a.mu.Unlock()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			if err := a.checkConnection(); err != nil {
				a.handleConnectionLost(err)
				return
			}
		}
	}
}

func (a *App) checkConnection() error {
	client := a.getClient()

	if client == nil {
		return fmt.Errorf("client is nil")
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(keepaliveTimeout):
		return fmt.Errorf("keepalive timeout")
	}
}

func (a *App) handleConnectionLost(err error) {
	if a.logger != nil {
		a.logger.LogConnection("connection-lost", err.Error())
	}
	a.mu.Lock()
	hadCommandSession := a.commandSession != nil
	if a.commandSession != nil {
		a.commandSession.Close()
		a.commandSession = nil
		a.commandStdin = nil
	}
	if a.client != nil {
		a.client.Close()
		a.client = nil
	}
	if a.agentConn != nil {
		a.agentConn.Close()
		a.agentConn = nil
	}
	a.vellumClient = nil
	a.writeableRootBusy = false
	deviceID := a.connectedDeviceID
	a.keepaliveStop = nil
	a.mu.Unlock()

	if hadCommandSession {
		runtime.EventsEmit(a.ctx, "command:output", "\nConnection lost.\n")
	}

	ue := apperrors.Classify(err)
	runtime.EventsEmit(a.ctx, "connection:lost", map[string]interface{}{
		"reason":   ue.Message,
		"code":     ue.Code,
		"deviceId": deviceID,
	})

	if deviceID != "" {
		go a.attemptReconnect(deviceID)
	} else {
		runtime.EventsEmit(a.ctx, "connection:failed", map[string]interface{}{
			"reason":   "Connection lost. Manual reconnection required.",
			"code":     apperrors.ErrHostDown,
			"deviceId": "",
		})
	}
}

func (a *App) attemptReconnect(deviceID string) {
	debug.Printf("[%s] attemptReconnect started for device %s\n", time.Now().Format("15:04:05.000"), deviceID)

	a.reconnectMu.Lock()
	if a.reconnecting {
		a.reconnectMu.Unlock()
		debug.Printf("[%s] Already reconnecting, skipping\n", time.Now().Format("15:04:05.000"))
		return
	}
	a.reconnecting = true
	a.fastDialMode = true
	a.reconnectMu.Unlock()

	defer func() {
		a.reconnectMu.Lock()
		a.reconnecting = false
		a.fastDialMode = false
		a.reconnectMu.Unlock()
		debug.Printf("[%s] attemptReconnect finished\n", time.Now().Format("15:04:05.000"))
	}()

	for attempt := 0; attempt < maxReconnectAttempts; attempt++ {
		a.reconnectMu.Lock()
		stillReconnecting := a.reconnecting
		a.reconnectMu.Unlock()

		if !stillReconnecting {
			debug.Printf("[%s] Reconnect cancelled\n", time.Now().Format("15:04:05.000"))
			return
		}

		debug.Printf("[%s] Reconnect attempt %d/%d starting\n", time.Now().Format("15:04:05.000"), attempt+1, maxReconnectAttempts)

		runtime.EventsEmit(a.ctx, "connection:reconnecting", map[string]interface{}{
			"attempt":     attempt + 1,
			"maxAttempts": maxReconnectAttempts,
			"deviceId":    deviceID,
		})

		result := a.ConnectToSavedDevice(deviceID)
		debug.Printf("[%s] Reconnect attempt %d/%d result: success=%v, message=%s\n", time.Now().Format("15:04:05.000"), attempt+1, maxReconnectAttempts, result.Success, result.Message)

		if result.Success {
			if a.logger != nil {
				a.logger.LogConnection("reconnected", deviceID)
			}
			runtime.EventsEmit(a.ctx, "connection:restored", map[string]interface{}{
				"deviceId": deviceID,
				"device":   result.Device,
			})
			return
		}

		if !isRetryableError(errors.New(result.Message)) {
			runtime.EventsEmit(a.ctx, "connection:failed", map[string]interface{}{
				"reason":   result.Message,
				"code":     result.Code,
				"deviceId": deviceID,
			})
			return
		}

		if attempt < maxReconnectAttempts-1 {
			backoffIdx := attempt
			if backoffIdx >= len(reconnectBackoff) {
				backoffIdx = len(reconnectBackoff) - 1
			}
			debug.Printf("[%s] Waiting %v before next attempt\n", time.Now().Format("15:04:05.000"), reconnectBackoff[backoffIdx])
			time.Sleep(reconnectBackoff[backoffIdx])
			debug.Printf("[%s] Backoff complete\n", time.Now().Format("15:04:05.000"))
		}
	}

	debug.Printf("[%s] All reconnect attempts exhausted\n", time.Now().Format("15:04:05.000"))
	runtime.EventsEmit(a.ctx, "connection:failed", map[string]interface{}{
		"reason":   "Could not reconnect after multiple attempts. Please check your connection and try again.",
		"code":     apperrors.ErrTimeout,
		"deviceId": deviceID,
	})
}
