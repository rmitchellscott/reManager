package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"github.com/rymdport/portal/filechooser"
	"github.com/rymdport/portal/openuri"
	"github.com/rymdport/portal/settings"
	"github.com/rymdport/portal/settings/appearance"
	"github.com/skratchdot/open-golang/open"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"
	"gopkg.in/ini.v1"

	"reManager/internal/backup"
	"reManager/internal/commands"
	"reManager/internal/component"
	"reManager/internal/debug"
	"reManager/internal/device"
	"reManager/internal/executor"
	"reManager/internal/installer"
	"reManager/internal/platform"
	"reManager/internal/storage"
	"reManager/internal/vellum"
)

func openFileDialog(ctx context.Context, title string) (string, error) {
	if platform.IsRunningInFlatpak() {
		home, _ := os.UserHomeDir()
		files, err := filechooser.OpenFile("", title, &filechooser.OpenFileOptions{
			CurrentFolder: home,
		})
		if err != nil {
			return "", err
		}
		if len(files) == 0 {
			return "", nil
		}
		return strings.TrimPrefix(files[0], "file://"), nil
	}
	home, _ := os.UserHomeDir()
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: home,
	})
}

func saveFileDialog(ctx context.Context, title, defaultFilename string) (string, error) {
	if platform.IsRunningInFlatpak() {
		home, _ := os.UserHomeDir()
		files, err := filechooser.SaveFile("", title, &filechooser.SaveFileOptions{
			CurrentFolder: home,
			CurrentName:   defaultFilename,
		})
		if err != nil {
			return "", err
		}
		if len(files) == 0 {
			return "", nil
		}
		return strings.TrimPrefix(files[0], "file://"), nil
	}
	home, _ := os.UserHomeDir()
	return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title:            title,
		DefaultFilename:  defaultFilename,
		DefaultDirectory: home,
	})
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "-", "<", "_", ">", "_", "\"", "_", "|", "_", "?", "_", "*", "_")
	return replacer.Replace(name)
}

type App struct {
	ctx            context.Context
	client         *ssh.Client
	session        *ssh.Session
	mu             sync.Mutex
	connectCancel  context.CancelFunc
	commandCancel  context.CancelFunc
	commandSession *ssh.Session
	commandStdin   io.WriteCloser
	dialogResponse chan bool
	deviceStore    *storage.DeviceStore
	settingsStore  *storage.SettingsStore
	vellumClient   *vellum.Client
	metadata       *vellum.MetadataStore

	keepaliveStop     chan struct{}
	connectedDeviceID string
	reconnecting      bool
	reconnectMu       sync.Mutex
	fastDialMode      bool
	installCancelCh   chan struct{}
	backupCancelCh    chan struct{}
	backupMu          sync.Mutex

	shellSession *ssh.Session
	shellStdin   io.WriteCloser
	shellMu      sync.Mutex
	shellActive  bool
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	store, err := storage.NewDeviceStore()
	if err != nil {
		fmt.Printf("Warning: could not initialize device store: %v\n", err)
	}
	a.deviceStore = store

	settingsStore, err := storage.NewSettingsStore()
	if err != nil {
		fmt.Printf("Warning: could not initialize settings store: %v\n", err)
	}
	a.settingsStore = settingsStore

	a.metadata = vellum.NewMetadataStore()
	if err := a.metadata.Load(); err != nil {
		fmt.Printf("Warning: could not load metadata: %v\n", err)
	}

	go func() {
		settings.OnSignalSettingChanged(func(changed settings.Changed) {
			if changed.Namespace == appearance.Namespace && changed.Key == "color-scheme" {
				scheme, err := appearance.ValueToColorScheme(changed.Value)
				if err == nil {
					var themeName string
					switch scheme {
					case appearance.Dark:
						themeName = "dark"
					case appearance.Light:
						themeName = "light"
					default:
						themeName = "unknown"
					}
					runtime.EventsEmit(a.ctx, "system-theme-changed", themeName)
				}
			}
		})
	}()
}

func (a *App) shutdown(ctx context.Context) {
	a.Disconnect()
}

type ConnectionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Device  string `json:"device,omitempty"`
}

type SSHKey struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func (a *App) GetDefaultSSHKeys() []SSHKey {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	sshDir := filepath.Join(home, ".ssh")
	keyNames := []string{"id_ed25519", "id_rsa", "id_ecdsa", "id_dsa"}
	var keys []SSHKey

	for _, name := range keyNames {
		keyPath := filepath.Join(sshDir, name)
		if _, err := os.Stat(keyPath); err == nil {
			keys = append(keys, SSHKey{
				Path: keyPath,
				Name: name,
			})
		}
	}

	return keys
}

func (a *App) SelectKeyFile() string {
	path, err := openFileDialog(a.ctx, "Select SSH Private Key")
	if err != nil {
		return ""
	}
	return path
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
	return a.deviceStore.Delete(id)
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
			Success: false,
			Message: "Device store not initialized",
		}
	}

	device, err := a.deviceStore.Get(id)
	if err != nil {
		return ConnectionResult{
			Success: false,
			Message: fmt.Sprintf("Device not found: %v", err),
		}
	}

	var result ConnectionResult
	if device.AuthType == "key" {
		passphrase, _ := a.deviceStore.GetKeyPassphrase(id)
		result = a.ConnectWithAuth(device.Host, "key", passphrase, device.KeyPath)
	} else {
		password, err := a.deviceStore.GetPassword(id)
		if err != nil {
			return ConnectionResult{
				Success: false,
				Message: "Could not retrieve password. Please reconnect and save the device again.",
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

		deviceType, err := a.detectDevice()
		if err != nil {
			runtime.EventsEmit(a.ctx, "vellum:bootstrap-error", fmt.Sprintf("Failed to detect device: %v", err))
			return
		}

		arch := device.GetArchitecture(component.DeviceType(deviceType))

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

	ctx, cancel := context.WithCancel(context.Background())
	a.connectCancel = cancel

	a.mu.Unlock()

	var authMethods []ssh.AuthMethod

	if authType == "key" {
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			return ConnectionResult{
				Success: false,
				Message: fmt.Sprintf("Failed to read key file: %v", err),
			}
		}

		var signer ssh.Signer
		if secret != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(secret))
		} else {
			signer, err = ssh.ParsePrivateKey(keyData)
		}
		if err != nil {
			if strings.Contains(err.Error(), "passphrase") {
				return ConnectionResult{
					Success: false,
					Message: "Key requires a passphrase",
				}
			}
			return ConnectionResult{
				Success: false,
				Message: fmt.Sprintf("Failed to parse key: %v", err),
			}
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else {
		authMethods = append(authMethods, ssh.Password(secret))
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

	client, err := a.dialWithContextWithRetry(ctx, addr, config)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return ConnectionResult{
				Success: false,
				Message: "Connection cancelled",
			}
		}
		return ConnectionResult{
			Success: false,
			Message: fmt.Sprintf("Failed to connect: %v", err),
		}
	}

	a.mu.Lock()
	a.client = client
	a.connectCancel = nil
	debug.Println("[DEBUG] SSH connected, creating vellum client")
	a.vellumClient = vellum.NewClient(&wailsExecutor{app: a})
	a.mu.Unlock()

	debug.Println("[DEBUG] Detecting device...")
	deviceType, err := a.detectDevice()
	debug.Printf("[DEBUG] Device detected: %s, err: %v\n", deviceType, err)
	if err != nil {
		return ConnectionResult{
			Success: true,
			Message: "Connected (could not detect device type)",
			Device:  "unknown",
		}
	}

	go func() {
		if err := a.metadata.Refresh(); err != nil {
			debug.Printf("[DEBUG] Metadata refresh failed: %v\n", err)
		}

		debug.Println("[DEBUG] Checking if vellum is installed...")
		installed, err := a.vellumClient.IsInstalled()
		debug.Printf("[DEBUG] Vellum installed: %v, err: %v\n", installed, err)
		if err == nil && !installed {
			runtime.EventsEmit(a.ctx, "vellum:bootstrap-prompt", nil)
		} else if err == nil && installed {
			valid, missing, verr := a.vellumClient.ValidateInstall()
			debug.Printf("[DEBUG] Vellum validation: valid=%v, missing=%v, err=%v\n", valid, missing, verr)
			runtime.EventsEmit(a.ctx, "vellum:ready", nil)
			if verr == nil && !valid {
				runtime.EventsEmit(a.ctx, "vellum:broken-install", missing)
				return
			}

			osState, err := a.vellumClient.GetOSVersionState()
			if err == nil && osState.Mismatch {
				debug.Printf("[DEBUG] OS mismatch detected: stored=%s, current=%s\n",
					osState.StoredVersion, osState.CurrentVersion)
				runtime.EventsEmit(a.ctx, "os:mismatch", map[string]string{
					"prevVersion": osState.StoredVersion,
					"newVersion":  osState.CurrentVersion,
				})
			}

			status := a.CheckHashtabVersion()
			debug.Printf("[DEBUG] Hashtab check: installed=%v, hashtabVersion=%s, firmwareVersion=%s, needsRebuild=%v\n",
				status.Installed, status.HashtabVersion, status.FirmwareVersion, status.NeedsRebuild)
			if status.NeedsRebuild {
				runtime.EventsEmit(a.ctx, "hashtab:version-mismatch", status)
			}

			updateStatus := a.GetUpdateServiceStatus()
			debug.Printf("[DEBUG] Auto-update check: enabled=%v, running=%v\n", updateStatus.Enabled, updateStatus.Running)
			if updateStatus.Enabled || updateStatus.Running {
				runtime.EventsEmit(a.ctx, "autoupdate:enabled", updateStatus)
			}

			timezoneStatus := a.GetTimezoneStatus()
			debug.Printf("[DEBUG] Timezone check: device=%s, saved=%s, needsUpdate=%v\n",
				timezoneStatus.DeviceTimezone, timezoneStatus.SavedTimezone, timezoneStatus.NeedsUpdate)

			// Auto-save device timezone if no preference saved yet
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
				runtime.EventsEmit(a.ctx, "timezone:status", timezoneStatus)
			}
			if timezoneStatus.NeedsUpdate {
				runtime.EventsEmit(a.ctx, "timezone:mismatch", timezoneStatus)
			}
		}
	}()

	return ConnectionResult{
		Success: true,
		Message: "Connected successfully",
		Device:  deviceType,
	}
}

func (a *App) detectDevice() (string, error) {
	output, err := a.runCommand("cat /sys/devices/soc0/machine")
	if err != nil {
		return "", err
	}

	machine := strings.TrimSpace(output)
	switch {
	case strings.Contains(machine, "reMarkable 1"):
		return "rm1", nil
	case strings.Contains(machine, "reMarkable 2"):
		return "rm2", nil
	case strings.Contains(machine, "Ferrari"):
		return "rmpp", nil
	case strings.Contains(machine, "Chiappa"):
		return "rmppm", nil
	default:
		return machine, nil
	}
}

func (a *App) Disconnect() {
	a.stopConnectionMonitor()

	a.reconnectMu.Lock()
	a.reconnecting = false
	a.reconnectMu.Unlock()

	a.StopShell()

	a.mu.Lock()
	defer a.mu.Unlock()

	a.connectedDeviceID = ""

	if a.client != nil {
		a.client.Close()
		a.client = nil
	}
	a.vellumClient = nil
}

func (a *App) IsConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client != nil
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
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

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
	a.mu.Lock()
	if a.commandSession != nil {
		a.commandSession.Close()
		a.commandSession = nil
		a.commandStdin = nil
	}
	if a.client != nil {
		a.client.Close()
		a.client = nil
	}
	a.vellumClient = nil
	deviceID := a.connectedDeviceID
	a.keepaliveStop = nil
	a.mu.Unlock()

	runtime.EventsEmit(a.ctx, "command:output", "\nConnection lost.\n")
	runtime.EventsEmit(a.ctx, "command:done", false)

	runtime.EventsEmit(a.ctx, "connection:lost", map[string]interface{}{
		"reason":   err.Error(),
		"deviceId": deviceID,
	})

	if deviceID != "" {
		go a.attemptReconnect(deviceID)
	} else {
		runtime.EventsEmit(a.ctx, "connection:failed", map[string]interface{}{
			"reason":   "Connection lost. Manual reconnection required.",
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
			runtime.EventsEmit(a.ctx, "connection:restored", map[string]interface{}{
				"deviceId": deviceID,
				"device":   result.Device,
			})
			return
		}

		if !isRetryableError(errors.New(result.Message)) {
			runtime.EventsEmit(a.ctx, "connection:failed", map[string]interface{}{
				"reason":   result.Message,
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
		"reason":   "Maximum reconnection attempts exceeded",
		"deviceId": deviceID,
	})
}

func (a *App) runCommand(cmd string) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("not connected")
	}

	debug.Printf("[DEBUG] runCommand creating session: %s\n", cmd[:min(50, len(cmd))])
	session, err := a.client.NewSession()
	if err != nil {
		debug.Printf("[DEBUG] runCommand session error: %v\n", err)
		return "", err
	}
	defer session.Close()

	debug.Printf("[DEBUG] runCommand executing: %s\n", cmd[:min(50, len(cmd))])
	output, err := session.CombinedOutput(cmd)
	debug.Printf("[DEBUG] runCommand done: %s, err: %v\n", cmd[:min(50, len(cmd))], err)
	return string(output), err
}

func (a *App) RunCommand(cmd string) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	output, err := a.runCommand(cmd)
	if err != nil {
		return fmt.Sprintf("Error: %v\n%s", err, output)
	}
	return output
}

func (a *App) RunCommandWithOutput(cmd string, requiresPTY bool) {
	debug.Println("[DEBUG] RunCommandWithOutput called:", cmd[:min(50, len(cmd))], "requiresPTY:", requiresPTY)
	go func() {
		a.mu.Lock()

		if a.client == nil {
			a.mu.Unlock()
			debug.Println("[DEBUG] Not connected, emitting error")
			runtime.EventsEmit(a.ctx, "command:output", "Error: not connected\n")
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		session, err := a.client.NewSession()
		if err != nil {
			a.mu.Unlock()
			debug.Println("[DEBUG] Session error:", err)
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		if requiresPTY {
			err = session.RequestPty("xterm-256color", 40, 120, ssh.TerminalModes{
				ssh.ECHO:          1,
				ssh.TTY_OP_ISPEED: 14400,
				ssh.TTY_OP_OSPEED: 14400,
			})
			if err != nil {
				a.mu.Unlock()
				debug.Println("[DEBUG] PTY request error:", err)
				runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error requesting PTY: %v\n", err))
				runtime.EventsEmit(a.ctx, "command:done", false)
				return
			}
			debug.Println("[DEBUG] PTY allocated successfully")
		}

		a.commandSession = session
		a.mu.Unlock()

		defer func() {
			session.Close()
			a.mu.Lock()
			a.commandSession = nil
			a.commandStdin = nil
			a.mu.Unlock()
		}()

		stdout, err := session.StdoutPipe()
		if err != nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		stderr, err := session.StderrPipe()
		if err != nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		stdin, err := session.StdinPipe()
		if err != nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		a.mu.Lock()
		a.commandStdin = stdin
		a.mu.Unlock()

		debug.Println("[DEBUG] Starting command")
		if err := session.Start(cmd); err != nil {
			debug.Println("[DEBUG] Start error:", err)
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error: %v\n", err))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		if !requiresPTY {
			stdin.Close()
		}

		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := stdout.Read(buf)
				if n > 0 {
					debug.Printf("[DEBUG] stdout: %d bytes\n", n)
					runtime.EventsEmit(a.ctx, "command:output", string(buf[:n]))
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}
			}
		}()

		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := stderr.Read(buf)
				if n > 0 {
					debug.Printf("[DEBUG] stderr: %d bytes\n", n)
					runtime.EventsEmit(a.ctx, "command:output", string(buf[:n]))
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}
			}
		}()

		err = session.Wait()
		debug.Println("[DEBUG] Command done, success:", err == nil)
		runtime.EventsEmit(a.ctx, "command:done", err == nil)
	}()
}

func (a *App) StopCommand() {
	a.mu.Lock()
	stdin := a.commandStdin
	a.mu.Unlock()

	if stdin != nil {
		debug.Println("[DEBUG] Sending Ctrl+C (0x03) to stdin")
		_, err := stdin.Write([]byte{0x03})
		if err != nil {
			debug.Printf("[DEBUG] Error writing Ctrl+C to stdin: %v\n", err)
		} else {
			debug.Println("[DEBUG] Ctrl+C sent successfully")
		}
	} else {
		debug.Println("[DEBUG] No stdin available to send Ctrl+C")
	}
}

func (a *App) StartShell(rows, cols int) error {
	a.shellMu.Lock()
	defer a.shellMu.Unlock()

	if a.shellActive {
		return fmt.Errorf("shell already running")
	}

	a.mu.Lock()
	if a.client == nil {
		a.mu.Unlock()
		return fmt.Errorf("not connected")
	}

	session, err := a.client.NewSession()
	if err != nil {
		a.mu.Unlock()
		return fmt.Errorf("failed to create session: %w", err)
	}
	a.mu.Unlock()

	if rows <= 0 {
		rows = 24
	}
	if cols <= 0 {
		cols = 80
	}

	err = session.RequestPty("xterm-256color", rows, cols, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	})
	if err != nil {
		session.Close()
		return fmt.Errorf("failed to request PTY: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return fmt.Errorf("failed to get stdout: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return fmt.Errorf("failed to get stdin: %w", err)
	}

	if err := session.Shell(); err != nil {
		session.Close()
		return fmt.Errorf("failed to start shell: %w", err)
	}

	a.shellSession = session
	a.shellStdin = stdin
	a.shellActive = true

	runtime.EventsEmit(a.ctx, "shell:started")

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				runtime.EventsEmit(a.ctx, "shell:output", string(buf[:n]))
			}
			if err != nil {
				break
			}
		}

		a.shellMu.Lock()
		a.shellActive = false
		a.shellSession = nil
		a.shellStdin = nil
		a.shellMu.Unlock()

		runtime.EventsEmit(a.ctx, "shell:stopped")
	}()

	return nil
}

func (a *App) WriteToShell(data string) error {
	a.shellMu.Lock()
	stdin := a.shellStdin
	a.shellMu.Unlock()

	if stdin == nil {
		return fmt.Errorf("shell not active")
	}

	_, err := stdin.Write([]byte(data))
	return err
}

func (a *App) ResizeShell(rows, cols int) error {
	a.shellMu.Lock()
	session := a.shellSession
	a.shellMu.Unlock()

	if session == nil {
		return fmt.Errorf("shell not active")
	}

	return session.WindowChange(rows, cols)
}

func (a *App) StopShell() {
	a.shellMu.Lock()
	session := a.shellSession
	stdin := a.shellStdin
	a.shellMu.Unlock()

	if session != nil {
		session.Signal(ssh.SIGHUP)
	}
	if stdin != nil {
		stdin.Close()
	}
	if session != nil {
		session.Close()
	}
}

func (a *App) IsShellActive() bool {
	a.shellMu.Lock()
	defer a.shellMu.Unlock()
	return a.shellActive
}

func (a *App) GetDeviceInfo() map[string]string {
	a.mu.Lock()

	info := make(map[string]string)

	if a.client == nil {
		a.mu.Unlock()
		return info
	}

	if output, err := a.runCommand("cat /sys/devices/soc0/machine"); err == nil {
		info["machine"] = strings.TrimSpace(output)
	}

	if output, err := a.runCommand("grep REMARKABLE_RELEASE_VERSION /usr/share/remarkable/update.conf"); err == nil {
		if parts := strings.SplitN(output, "=", 2); len(parts) == 2 {
			info["firmware"] = strings.TrimSpace(parts[1])
		}
	} else if output, err := a.runCommand("grep IMG_VERSION /etc/os-release"); err == nil {
		if parts := strings.SplitN(output, "=", 2); len(parts) == 2 {
			info["firmware"] = strings.Trim(strings.TrimSpace(parts[1]), "\"")
		}
	}

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

type UpdateServiceStatus struct {
	Enabled bool `json:"enabled"`
	Running bool `json:"running"`
}

func (a *App) GetUpdateServiceStatus() UpdateServiceStatus {
	a.mu.Lock()
	defer a.mu.Unlock()

	status := UpdateServiceStatus{
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

type HashtabVersionStatus struct {
	Installed       bool   `json:"installed"`
	HashtabVersion  string `json:"hashtabVersion"`
	FirmwareVersion string `json:"firmwareVersion"`
	NeedsRebuild    bool   `json:"needsRebuild"`
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

type TimezoneStatus struct {
	DeviceTimezone string `json:"deviceTimezone"`
	SavedTimezone  string `json:"savedTimezone"`
	NeedsUpdate    bool   `json:"needsUpdate"`
}

func (a *App) GetDeviceTimezone() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client == nil {
		return "", fmt.Errorf("not connected")
	}

	output, err := a.runCommand("timedatectl show --property=Timezone --value")
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

	output, err := a.runCommand("timedatectl show --property=Timezone --value")
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

		cmdResults := []component.CommandResult{
			{
				Script:      fmt.Sprintf("systemctl restart systemd-timedated && timedatectl set-timezone %s", timezone),
				Description: "Set device timezone",
			},
		}

		dev := component.DeviceType(deviceType)
		if dev == component.DeviceRMPP || dev == component.DeviceRMPPM {
			cmdResults = commands.WrapWithWriteableRoot(cmdResults, dev)
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
				runtime.EventsEmit(a.ctx, "timezone:error", "Failed to set timezone")
				return
			}
		}

		runtime.EventsEmit(a.ctx, "timezone:complete", timezone)
	}()
}

type PackageInfo struct {
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	Description    string   `json:"description"`
	UpstreamAuthor string   `json:"upstreamAuthor"`
	Categories     []string `json:"categories"`
	URL            string   `json:"url"`
	License        string   `json:"license"`
	Devices        []string `json:"devices"`
	Depends        []string `json:"depends"`
	Conflicts      []string `json:"conflicts"`
	OSMin          *string  `json:"osMin"`
	OSMax          *string  `json:"osMax"`
}

var hiddenPackages = map[string]bool{
	"vellum":                 true,
	"vellum-bash-completion": true,
	"mount-utils":            true,
	"/bin/sh":                true,
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func (a *App) GetPackages(deviceType, firmware, arch string) []PackageInfo {
	debug.Printf("[DEBUG] GetPackages called, metadata=%p, deviceType=%s, firmware=%s, arch=%s\n", a.metadata, deviceType, firmware, arch)
	packages := a.metadata.GetAllPackagesForDevice(deviceType, firmware, arch)
	debug.Printf("[DEBUG] GetPackages got %d packages\n", len(packages))
	result := []PackageInfo{}

	for _, pkg := range packages {
		if hiddenPackages[pkg.Name] {
			continue
		}
		var visibleDepends []string
		for _, dep := range pkg.Depends {
			if !hiddenPackages[dep] {
				visibleDepends = append(visibleDepends, dep)
			}
		}
		result = append(result, PackageInfo{
			Name:           pkg.Name,
			Version:        pkg.Version,
			Description:    pkg.Description,
			UpstreamAuthor: pkg.UpstreamAuthor,
			Categories:     pkg.Categories,
			URL:            pkg.URL,
			License:        pkg.License,
			Devices:        pkg.Devices,
			Depends:        visibleDepends,
			Conflicts:      pkg.Conflicts,
			OSMin:          pkg.OSMin,
			OSMax:          pkg.OSMax,
		})
	}

	debug.Printf("[DEBUG] GetPackages returning %d PackageInfo\n", len(result))
	return result
}

func (a *App) GetInstalledPackages() []string {
	if a.vellumClient == nil {
		return []string{}
	}

	packages, err := a.vellumClient.List()
	if err != nil {
		fmt.Printf("Error getting installed packages: %v\n", err)
		return []string{}
	}

	var result []string
	for _, pkg := range packages {
		if !hiddenPackages[pkg] {
			result = append(result, pkg)
		}
	}
	return result
}

type InstalledPackagesResult struct {
	Packages    []string `json:"packages"`
	OsUpgraded  bool     `json:"osUpgraded"`
	PrevVersion string   `json:"prevVersion"`
	NewVersion  string   `json:"newVersion"`
}

func (a *App) GetInstalledPackagesWithOsCheck() InstalledPackagesResult {
	if a.vellumClient == nil {
		return InstalledPackagesResult{}
	}

	listResult, err := a.vellumClient.ListWithOsCheck()
	if err != nil {
		fmt.Printf("Error getting installed packages: %v\n", err)
		return InstalledPackagesResult{}
	}

	var packages []string
	for _, pkg := range listResult.Packages {
		if !hiddenPackages[pkg] {
			packages = append(packages, pkg)
		}
	}

	return InstalledPackagesResult{
		Packages:    packages,
		OsUpgraded:  listResult.OsUpgraded,
		PrevVersion: listResult.PrevVersion,
		NewVersion:  listResult.NewVersion,
	}
}

func (a *App) RunReenable() {
	if a.vellumClient == nil {
		return
	}

	runtime.EventsEmit(a.ctx, "terminal:clear")
	runtime.EventsEmit(a.ctx, "terminal:output", "Running vellum reenable...\n")

	err := a.vellumClient.ReenableStreaming(func(line string) {
		runtime.EventsEmit(a.ctx, "terminal:output", line+"\n")
	})

	if err != nil {
		runtime.EventsEmit(a.ctx, "terminal:output", fmt.Sprintf("\nError: %v\n", err))
	} else {
		runtime.EventsEmit(a.ctx, "terminal:output", "\nReenable completed successfully.\n")
	}
}

func (a *App) SimulatePackageUpgrade() (map[string]interface{}, error) {
	if a.vellumClient == nil {
		return nil, fmt.Errorf("not connected")
	}
	result, err := a.vellumClient.SimulateUpgrade()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"packages":    result.Packages,
		"hasUpgrades": result.HasUpgrades,
	}, nil
}

func (a *App) RunPackageUpgrade() {
	if a.vellumClient == nil {
		return
	}

	go func() {
		runtime.EventsEmit(a.ctx, "terminal:clear")
		runtime.EventsEmit(a.ctx, "terminal:output", "Running vellum upgrade...\n")

		err := a.vellumClient.UpgradeStreaming(func(line string) {
			runtime.EventsEmit(a.ctx, "terminal:output", line+"\n")
		})

		success := err == nil
		if success {
			runtime.EventsEmit(a.ctx, "terminal:output", "\nPackage upgrade completed.\n")
		} else {
			runtime.EventsEmit(a.ctx, "terminal:output", fmt.Sprintf("\nUpgrade error: %v\n", err))
		}
		runtime.EventsEmit(a.ctx, "package-upgrade:complete", success)
	}()
}

type OSVersionStateResult struct {
	CurrentVersion string `json:"currentVersion"`
	StoredVersion  string `json:"storedVersion"`
	Mismatch       bool   `json:"mismatch"`
}

func (a *App) GetOSVersionState() OSVersionStateResult {
	if a.vellumClient == nil {
		return OSVersionStateResult{}
	}

	state, err := a.vellumClient.GetOSVersionState()
	if err != nil {
		return OSVersionStateResult{}
	}

	return OSVersionStateResult{
		CurrentVersion: state.CurrentVersion,
		StoredVersion:  state.StoredVersion,
		Mismatch:       state.Mismatch,
	}
}

type CompatibilityResultJSON struct {
	Compatible   []string `json:"compatible"`
	Incompatible []string `json:"incompatible"`
	NoConstraint []string `json:"noConstraint"`
	FetchFailed  bool     `json:"fetchFailed"`
}

func (a *App) CheckOSCompatibility(targetOS string) CompatibilityResultJSON {
	if a.vellumClient == nil {
		return CompatibilityResultJSON{FetchFailed: true}
	}

	result, _ := a.vellumClient.CheckOSCompatibility(targetOS)
	if result == nil {
		return CompatibilityResultJSON{FetchFailed: true}
	}

	return CompatibilityResultJSON{
		Compatible:   result.Compatible,
		Incompatible: result.Incompatible,
		NoConstraint: result.NoConstraint,
		FetchFailed:  result.FetchFailed,
	}
}

type PackageCompatibilityStatus struct {
	InstalledPackages    []string `json:"installedPackages"`
	CompatiblePackages   []string `json:"compatiblePackages"`
	IncompatiblePackages []string `json:"incompatiblePackages"`
	CurrentOsVersion     string   `json:"currentOsVersion"`
	StoredOsVersion      string   `json:"storedOsVersion"`
	FetchFailed          bool     `json:"fetchFailed"`
}

func (a *App) GetPackageCompatibilityStatus() PackageCompatibilityStatus {
	debug.Println("[DEBUG] GetPackageCompatibilityStatus: called")
	if a.vellumClient == nil {
		debug.Println("[DEBUG] GetPackageCompatibilityStatus: vellumClient is nil")
		return PackageCompatibilityStatus{FetchFailed: true}
	}

	osState, err := a.vellumClient.GetOSVersionState()
	if err != nil {
		debug.Printf("[DEBUG] GetPackageCompatibilityStatus: GetOSVersionState error: %v\n", err)
		return PackageCompatibilityStatus{FetchFailed: true}
	}
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: osState=%+v\n", osState)

	installed, err := a.vellumClient.List()
	if err != nil {
		debug.Printf("[DEBUG] GetPackageCompatibilityStatus: List error: %v\n", err)
		return PackageCompatibilityStatus{FetchFailed: true}
	}
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: installed=%v\n", installed)

	var filteredInstalled []string
	for _, pkg := range installed {
		if !hiddenPackages[pkg] {
			filteredInstalled = append(filteredInstalled, pkg)
		}
	}
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: filteredInstalled=%v\n", filteredInstalled)

	compat, err := a.vellumClient.CheckOSCompatibility(osState.CurrentVersion)
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: compat=%+v, err=%v\n", compat, err)

	if err != nil && (compat == nil || compat.FetchFailed) {
		debug.Println("[DEBUG] GetPackageCompatibilityStatus: returning with FetchFailed due to error")
		return PackageCompatibilityStatus{
			InstalledPackages: filteredInstalled,
			CurrentOsVersion:  osState.CurrentVersion,
			StoredOsVersion:   osState.StoredVersion,
			FetchFailed:       true,
		}
	}

	allEmpty := len(compat.Compatible) == 0 && len(compat.Incompatible) == 0 && len(compat.NoConstraint) == 0
	if compat.FetchFailed || allEmpty {
		debug.Printf("[DEBUG] GetPackageCompatibilityStatus: fallback (FetchFailed=%v, allEmpty=%v)\n", compat.FetchFailed, allEmpty)
		return PackageCompatibilityStatus{
			InstalledPackages:    filteredInstalled,
			CompatiblePackages:   filteredInstalled,
			IncompatiblePackages: []string{},
			CurrentOsVersion:     osState.CurrentVersion,
			StoredOsVersion:      osState.StoredVersion,
			FetchFailed:          true,
		}
	}

	result := PackageCompatibilityStatus{
		InstalledPackages:    filteredInstalled,
		CompatiblePackages:   append(compat.Compatible, compat.NoConstraint...),
		IncompatiblePackages: compat.Incompatible,
		CurrentOsVersion:     osState.CurrentVersion,
		StoredOsVersion:      osState.StoredVersion,
		FetchFailed:          false,
	}
	debug.Printf("[DEBUG] GetPackageCompatibilityStatus: returning result=%+v\n", result)
	return result
}

func (a *App) RunUpgrade() {
	if a.vellumClient == nil {
		return
	}

	go func() {
		osState, err := a.vellumClient.GetOSVersionState()
		if err != nil {
			runtime.EventsEmit(a.ctx, "upgrade:error", "Failed to get OS version state")
			return
		}

		if osState.Mismatch {
			runtime.EventsEmit(a.ctx, "terminal:output", fmt.Sprintf("Checking package compatibility with OS %s...\n", osState.CurrentVersion))

			compat, err := a.vellumClient.CheckOSCompatibility(osState.CurrentVersion)
			if err != nil && compat.FetchFailed {
				runtime.EventsEmit(a.ctx, "upgrade:error", "Could not fetch package index to verify compatibility")
				return
			}

			if len(compat.Incompatible) > 0 {
				runtime.EventsEmit(a.ctx, "upgrade:blocked", CompatibilityResultJSON{
					Compatible:   compat.Compatible,
					Incompatible: compat.Incompatible,
					NoConstraint: compat.NoConstraint,
					FetchFailed:  compat.FetchFailed,
				})
				return
			}

			runtime.EventsEmit(a.ctx, "terminal:output", "All packages compatible. Proceeding with upgrade...\n\n")
		}

		runtime.EventsEmit(a.ctx, "terminal:clear")
		runtime.EventsEmit(a.ctx, "terminal:output", "Running vellum upgrade...\n")

		err = a.vellumClient.UpgradeStreaming(func(line string) {
			runtime.EventsEmit(a.ctx, "terminal:output", line+"\n")
		})

		if err != nil {
			runtime.EventsEmit(a.ctx, "terminal:output", fmt.Sprintf("\nUpgrade error: %v\n", err))
			runtime.EventsEmit(a.ctx, "upgrade:complete", false)
			return
		}

		runtime.EventsEmit(a.ctx, "terminal:output", "\nUpgrade completed successfully.\n")
		runtime.EventsEmit(a.ctx, "upgrade:complete", true)
	}()
}

type MaintenanceCommandInfo struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	Description      string `json:"description"`
	RequiresTerminal bool   `json:"requiresTerminal"`
	AllowStop        bool   `json:"allowStop"`
	Hook             string `json:"hook,omitempty"`
}

func (a *App) GetMaintenanceCommands(pkgName string) []MaintenanceCommandInfo {
	commands := a.metadata.GetMaintenanceCommands(pkgName)
	if commands == nil {
		return nil
	}

	result := make([]MaintenanceCommandInfo, len(commands))
	for i, cmd := range commands {
		result[i] = MaintenanceCommandInfo{
			ID:               cmd.ID,
			Label:            cmd.Label,
			Description:      cmd.Description,
			RequiresTerminal: cmd.RequiresTerminal,
			AllowStop:        cmd.AllowStop,
			Hook:             cmd.Hook,
		}
	}

	return result
}

type SystemTaskInfo struct {
	ID                 string `json:"id"`
	Label              string `json:"label"`
	Description        string `json:"description"`
	RequiresTerminal   bool   `json:"requiresTerminal"`
	NeedsWriteableRoot bool   `json:"needsWriteableRoot"`
}

func (a *App) GetSystemTasksInfo() []SystemTaskInfo {
	result := make([]SystemTaskInfo, len(device.SystemTasks))
	for i, task := range device.SystemTasks {
		result[i] = SystemTaskInfo{
			ID:                 task.ID,
			Label:              task.Label,
			Description:        task.Description,
			RequiresTerminal:   task.RequiresTerminal,
			NeedsWriteableRoot: task.NeedsWriteableRoot,
		}
	}
	return result
}

func (a *App) GetDeviceDisplayName(machine string) string {
	return device.GetDisplayName(machine)
}

func (a *App) GetDeviceArchitecture(deviceType string) string {
	return string(device.GetArchitecture(component.DeviceType(deviceType)))
}

type InstallProgress struct {
	Component string `json:"component"`
	Index     int    `json:"index"`
	Total     int    `json:"total"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type InstallResult struct {
	Success  bool     `json:"success"`
	Errors   []string `json:"errors"`
	DNSError bool     `json:"dnsError"`
}

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime int64  `json:"modTime"`
	Mode    string `json:"mode"`
}

type TransferProgress struct {
	Filename   string  `json:"filename"`
	BytesSent  int64   `json:"bytesSent"`
	TotalBytes int64   `json:"totalBytes"`
	Percentage float64 `json:"percentage"`
	Status     string  `json:"status"`
}

type DialogRequest struct {
	Title             string   `json:"title"`
	Message           string   `json:"message"`
	Steps             []string `json:"steps"`
	ConfirmText       string   `json:"confirmText"`
	InProgressMessage string   `json:"inProgressMessage"`
}

type BlockedUninstallInfo struct {
	RequestedPackages []string            `json:"requestedPackages"`
	BlockedBy         map[string][]string `json:"blockedBy"`
}

// InstallSimulationResult contains the result of simulating an install
type InstallSimulationResult struct {
	Packages  []string `json:"packages"`  // All packages that will be installed (including dependencies)
	Requested []string `json:"requested"` // Originally requested packages
}

// UninstallSimulationResult contains the result of simulating an uninstall
type UninstallSimulationResult struct {
	Packages          []string            `json:"packages"`          // Packages that will be removed
	Blocked           map[string][]string `json:"blocked"`           // Packages blocked by dependents
	RecursivePackages []string            `json:"recursivePackages"` // All packages if recursive removal is needed
}

// SimulateInstall returns all packages that will be installed (including dependencies)
func (a *App) SimulateInstall(packageNames []string, deviceType string) (*InstallSimulationResult, error) {
	if a.vellumClient == nil {
		return &InstallSimulationResult{Packages: packageNames, Requested: packageNames}, nil
	}

	// Use SimulateAdd to get all packages that will be installed
	allPackages, err := a.vellumClient.SimulateAdd(packageNames...)
	if err != nil {
		debug.Printf("[DEBUG] SimulateAdd failed: %v, using packageNames only\n", err)
		return &InstallSimulationResult{Packages: packageNames, Requested: packageNames}, nil
	}

	// If nothing to install (all already installed), return empty
	if len(allPackages) == 0 {
		return &InstallSimulationResult{Packages: []string{}, Requested: packageNames}, nil
	}

	return &InstallSimulationResult{Packages: allPackages, Requested: packageNames}, nil
}

// SimulateUninstall returns simulation info for uninstalling packages
func (a *App) SimulateUninstall(packageNames []string) (*UninstallSimulationResult, error) {
	if a.vellumClient == nil {
		return &UninstallSimulationResult{Packages: packageNames}, nil
	}

	simResult, err := a.vellumClient.SimulateDel(packageNames...)
	if err != nil {
		debug.Printf("[DEBUG] SimulateDel failed: %v\n", err)
		return &UninstallSimulationResult{Packages: packageNames}, nil
	}

	result := &UninstallSimulationResult{
		Packages: simResult.Packages,
		Blocked:  simResult.Blocked,
	}

	// If there are blocked packages, also get the recursive list
	if len(simResult.Blocked) > 0 {
		recursiveList, err := a.vellumClient.SimulateDelRecursive(packageNames...)
		if err != nil {
			debug.Printf("[DEBUG] SimulateDelRecursive failed: %v\n", err)
		} else {
			result.RecursivePackages = recursiveList
		}
	}

	return result, nil
}

func (a *App) RespondToDialog(confirmed bool) {
	if a.dialogResponse != nil {
		a.dialogResponse <- confirmed
	}
}

func (a *App) CancelInstallation() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.installCancelCh != nil {
		close(a.installCancelCh)
		a.installCancelCh = nil
	}
}

func (a *App) InstallPackages(packageNames []string, deviceType string) {
	go func() {
		a.mu.Lock()
		a.installCancelCh = make(chan struct{})
		cancelCh := a.installCancelCh
		a.mu.Unlock()

		defer func() {
			a.mu.Lock()
			if a.installCancelCh == cancelCh {
				a.installCancelCh = nil
			}
			a.mu.Unlock()
		}()

		isCancelled := func() bool {
			select {
			case <-cancelCh:
				return true
			default:
				return false
			}
		}

		a.dialogResponse = make(chan bool, 1)
		defer func() {
			close(a.dialogResponse)
			a.dialogResponse = nil
		}()

		arch := device.GetArchitecture(component.DeviceType(deviceType))

		ctx := component.CommandContext{
			Arch:   arch,
			Device: component.DeviceType(deviceType),
		}

		// Check proxy mode setting
		settings, _ := a.settingsStore.Load()
		proxyEnabled := settings == nil || settings.ProxyMode

		// Proxy download packages first
		a.mu.Lock()
		sshClient := a.client
		a.mu.Unlock()

		var allPackages []string
		if sshClient != nil && proxyEnabled {
			proxy := vellum.NewProxy(a.vellumClient, sshClient, string(arch))
			runtime.EventsEmit(a.ctx, "install:progress", InstallProgress{
				Status:  "downloading",
				Message: "Downloading packages via reManager...",
			})

			var err error
			allPackages, err = proxy.ProxyDownloadWithProgress(packageNames, func(progress vellum.ProxyProgress) {
				runtime.EventsEmit(a.ctx, "install:progress", InstallProgress{
					Component: progress.Package,
					Index:     progress.Current - 1,
					Total:     progress.Total,
					Status:    progress.Phase,
					Message:   progress.Message,
				})
			})
			if err != nil {
				runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
					Success: false,
					Errors:  []string{fmt.Sprintf("Proxy download failed: %v", err)},
				})
				return
			}
		} else {
			allPackages = packageNames
		}

		if isCancelled() {
			runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
				Success: false,
				Errors:  []string{"Installation cancelled"},
			})
			return
		}

		exec := &wailsExecutor{app: a}
		inst := installer.NewInstaller(a.vellumClient, a.metadata, exec)

		result := inst.Install(
			packageNames,
			allPackages,
			ctx,
			func(progress executor.ProgressInfo) {
				runtime.EventsEmit(a.ctx, "install:progress", InstallProgress{
					Component: progress.CurrentComponent,
					Index:     progress.CurrentIndex,
					Total:     progress.TotalComponents,
					Status:    string(progress.Status),
					Message:   progress.Message,
				})
			},
			func(hookResult *component.HookExecutionResult) error {
				if hookResult.DialogConfig != nil {
					runtime.EventsEmit(a.ctx, "hook:dialog", DialogRequest{
						Title:             hookResult.DialogConfig.Title,
						Message:           hookResult.DialogConfig.Message,
						Steps:             hookResult.DialogConfig.Steps,
						ConfirmText:       hookResult.DialogConfig.ConfirmText,
						InProgressMessage: hookResult.DialogConfig.InProgressMessage,
					})

					confirmed := <-a.dialogResponse
					if !confirmed {
						return fmt.Errorf("user cancelled")
					}

					runtime.EventsEmit(a.ctx, "hook:started", map[string]string{
						"title": hookResult.DialogConfig.Title,
					})

					if hookResult.Command != nil {
						runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", hookResult.Command.Script))
						if err := exec.Execute([]component.CommandResult{*hookResult.Command}); err != nil {
							return err
						}
					}
				}
				return nil
			},
		)

		runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
			Success:  result.Success,
			Errors:   result.Errors,
			DNSError: result.DNSError,
		})
	}()
}

func (a *App) UninstallPackages(packageNames []string, deviceType string) {
	go func() {
		a.mu.Lock()
		a.installCancelCh = make(chan struct{})
		cancelCh := a.installCancelCh
		a.mu.Unlock()

		defer func() {
			a.mu.Lock()
			if a.installCancelCh == cancelCh {
				a.installCancelCh = nil
			}
			a.mu.Unlock()
		}()

		isCancelled := func() bool {
			select {
			case <-cancelCh:
				return true
			default:
				return false
			}
		}

		a.dialogResponse = make(chan bool, 1)
		defer func() {
			close(a.dialogResponse)
			a.dialogResponse = nil
		}()

		arch := device.GetArchitecture(component.DeviceType(deviceType))

		ctx := component.CommandContext{
			Arch:   arch,
			Device: component.DeviceType(deviceType),
		}

		// Simulate uninstall to check for blockers and get full package list
		var allPackages []string
		useRecursive := false

		if a.vellumClient != nil {
			simResult, err := a.vellumClient.SimulateDel(packageNames...)
			if err != nil {
				debug.Printf("[DEBUG] SimulateDel failed: %v, using packageNames only\n", err)
				allPackages = packageNames
			} else if len(simResult.Blocked) > 0 {
				// Packages are blocked by dependents - prompt user
				debug.Printf("[DEBUG] Packages blocked: %v\n", simResult.Blocked)
				runtime.EventsEmit(a.ctx, "uninstall:blocked", BlockedUninstallInfo{
					RequestedPackages: packageNames,
					BlockedBy:         simResult.Blocked,
				})

				confirmed := <-a.dialogResponse
				if !confirmed {
					runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
						Success: false,
						Errors:  []string{"Uninstall cancelled by user"},
					})
					return
				}

				// User confirmed - use recursive deletion
				useRecursive = true
				allPackages, err = a.vellumClient.SimulateDelRecursive(packageNames...)
				if err != nil {
					debug.Printf("[DEBUG] SimulateDelRecursive failed: %v\n", err)
					allPackages = packageNames
				}
			} else {
				allPackages = simResult.Packages
				if len(allPackages) == 0 {
					allPackages = packageNames
				}
			}
		} else {
			allPackages = packageNames
		}

		if isCancelled() {
			runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
				Success: false,
				Errors:  []string{"Uninstall cancelled"},
			})
			return
		}

		exec := &wailsExecutor{app: a}
		inst := installer.NewInstaller(a.vellumClient, a.metadata, exec)

		result := inst.Uninstall(
			packageNames,
			allPackages,
			useRecursive,
			ctx,
			func(progress executor.ProgressInfo) {
				runtime.EventsEmit(a.ctx, "install:progress", InstallProgress{
					Component: progress.CurrentComponent,
					Index:     progress.CurrentIndex,
					Total:     progress.TotalComponents,
					Status:    string(progress.Status),
					Message:   progress.Message,
				})
			},
			func(hookResult *component.HookExecutionResult) error {
				if hookResult.DialogConfig != nil {
					runtime.EventsEmit(a.ctx, "hook:dialog", DialogRequest{
						Title:             hookResult.DialogConfig.Title,
						Message:           hookResult.DialogConfig.Message,
						Steps:             hookResult.DialogConfig.Steps,
						ConfirmText:       hookResult.DialogConfig.ConfirmText,
						InProgressMessage: hookResult.DialogConfig.InProgressMessage,
					})

					confirmed := <-a.dialogResponse
					if !confirmed {
						return fmt.Errorf("user cancelled")
					}

					if hookResult.Command != nil {
						runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", hookResult.Command.Script))
						if err := exec.Execute([]component.CommandResult{*hookResult.Command}); err != nil {
							return err
						}
					}
				}
				return nil
			},
		)

		runtime.EventsEmit(a.ctx, "install:complete", InstallResult{
			Success:  result.Success,
			Errors:   result.Errors,
			DNSError: result.DNSError,
		})
	}()
}

func (a *App) RunMaintenanceCommand(pkgName, commandID, deviceType string) {
	go func() {
		commands := a.metadata.GetMaintenanceCommands(pkgName)
		if commands == nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("No maintenance commands for package: %s\n", pkgName))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		var cmd *vellum.MaintenanceCommand
		for i := range commands {
			if commands[i].ID == commandID {
				cmd = &commands[i]
				break
			}
		}
		if cmd == nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Command not found: %s\n", commandID))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		if cmd.Hook != "" {
			hookFunc := vellum.GetHook(cmd.Hook)
			if hookFunc != nil {
				arch := device.GetArchitecture(component.DeviceType(deviceType))
				ctx := component.CommandContext{
					Arch:   arch,
					Device: component.DeviceType(deviceType),
				}

				hookResult, err := hookFunc(ctx)
				if err != nil {
					runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Hook error: %v\n", err))
					runtime.EventsEmit(a.ctx, "command:done", false)
					return
				}

				if hookResult != nil && hookResult.DialogConfig != nil {
					a.dialogResponse = make(chan bool, 1)
					defer func() {
						close(a.dialogResponse)
						a.dialogResponse = nil
					}()

					runtime.EventsEmit(a.ctx, "hook:dialog", DialogRequest{
						Title:             hookResult.DialogConfig.Title,
						Message:           hookResult.DialogConfig.Message,
						Steps:             hookResult.DialogConfig.Steps,
						ConfirmText:       hookResult.DialogConfig.ConfirmText,
						InProgressMessage: hookResult.DialogConfig.InProgressMessage,
					})

					confirmed := <-a.dialogResponse
					if !confirmed {
						runtime.EventsEmit(a.ctx, "command:done", false)
						return
					}
				}
			}
		}

		runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", cmd.Command))
		a.RunCommandWithOutput(cmd.Command, cmd.RequiresTerminal)
	}()
}

func (a *App) RunSystemTask(taskID, deviceType string) {
	go func() {
		task := device.GetSystemTask(taskID)
		if task == nil {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Task not found: %s\n", taskID))
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		arch := device.GetArchitecture(component.DeviceType(deviceType))
		ctx := component.CommandContext{
			Arch:   arch,
			Device: component.DeviceType(deviceType),
		}

		cmdResults := task.Command(ctx)

		if task.NeedsWriteableRoot {
			cmdResults = commands.WrapWithWriteableRoot(cmdResults, component.DeviceType(deviceType))
		}

		for _, c := range cmdResults {
			runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("$ %s\n", c.Script))

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

			a.RunCommandWithOutput(c.Script, c.RequiresPTY)
			success := <-done
			unsub()

			if !success {
				runtime.EventsEmit(a.ctx, "command:output", "Command failed, stopping execution\n")
				return
			}
		}
		runtime.EventsEmit(a.ctx, "systemtask:complete", true)
	}()
}

type wailsExecutor struct {
	app *App
}

func (e *wailsExecutor) Execute(cmds []component.CommandResult) error {
	for _, cmd := range cmds {
		runtime.EventsEmit(e.app.ctx, "command:output", fmt.Sprintf("$ %s\n", cmd.Script))

		done := make(chan bool, 1)
		unsub := runtime.EventsOn(e.app.ctx, "command:done", func(optionalData ...interface{}) {
			if len(optionalData) > 0 {
				if success, ok := optionalData[0].(bool); ok {
					done <- success
					return
				}
			}
			done <- false
		})

		e.app.RunCommandWithOutput(cmd.Script, cmd.RequiresPTY)
		success := <-done
		unsub()

		if !success {
			return fmt.Errorf("command failed: %s", cmd.Description)
		}
	}
	return nil
}

func (e *wailsExecutor) ExecuteWithOutput(cmd string) (string, error) {
	debug.Printf("[DEBUG] ExecuteWithOutput waiting for lock: %s\n", cmd[:min(50, len(cmd))])
	e.app.mu.Lock()
	debug.Printf("[DEBUG] ExecuteWithOutput got lock: %s\n", cmd[:min(50, len(cmd))])
	defer func() {
		e.app.mu.Unlock()
		debug.Printf("[DEBUG] ExecuteWithOutput released lock: %s\n", cmd[:min(50, len(cmd))])
	}()
	return e.app.runCommand(cmd)
}

func (e *wailsExecutor) ExecuteStreaming(cmd string, onOutput func(line string)) error {
	e.app.mu.Lock()
	if e.app.client == nil {
		e.app.mu.Unlock()
		return fmt.Errorf("not connected")
	}

	session, err := e.app.client.NewSession()
	if err != nil {
		e.app.mu.Unlock()
		return err
	}
	e.app.mu.Unlock()
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	if err := session.Start(cmd); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)

	readLines := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			if onOutput != nil {
				onOutput(scanner.Text())
			}
		}
	}

	go readLines(stdout)
	go readLines(stderr)

	err = session.Wait()
	wg.Wait()
	return err
}

func (a *App) GetAppVersion() string {
	return version
}

type SettingsInfo struct {
	TabVisibility              map[string]bool `json:"tabVisibility"`
	ProxyMode                  bool            `json:"proxyMode"`
	SuppressSystemFileWarnings bool            `json:"suppressSystemFileWarnings"`
	Theme                      string          `json:"theme"`
	TerminalTheme              string          `json:"terminalTheme"`
	EditorTheme string `json:"editorTheme"`
}

func (a *App) GetSettings() SettingsInfo {
	if a.settingsStore == nil {
		return SettingsInfo{
			TabVisibility:              map[string]bool{"mods": true, "maintenance": true, "utilities": true},
			ProxyMode:                  true,
			SuppressSystemFileWarnings: false,
			Theme:                      "system",
			TerminalTheme:              "match",
			EditorTheme:                "match",
		}
	}
	settings, err := a.settingsStore.Load()
	if err != nil {
		return SettingsInfo{
			TabVisibility:              map[string]bool{"mods": true, "maintenance": true, "utilities": true},
			ProxyMode:                  true,
			SuppressSystemFileWarnings: false,
			Theme:                      "system",
			TerminalTheme:              "match",
			EditorTheme:                "match",
		}
	}
	return SettingsInfo{
		TabVisibility:              settings.TabVisibility,
		ProxyMode:                  settings.ProxyMode,
		SuppressSystemFileWarnings: settings.SuppressSystemFileWarnings,
		Theme:                      settings.Theme,
		TerminalTheme:              settings.TerminalTheme,
		EditorTheme:                settings.EditorTheme,
	}
}

func (a *App) SaveSettings(tabVisibility map[string]bool, proxyMode bool, suppressSystemFileWarnings bool, theme string, terminalTheme string, editorTheme string) error {
	if a.settingsStore == nil {
		return fmt.Errorf("settings store not initialized")
	}
	settings := &storage.Settings{
		TabVisibility:              storage.TabVisibility(tabVisibility),
		ProxyMode:                  proxyMode,
		SuppressSystemFileWarnings: suppressSystemFileWarnings,
		Theme:                      theme,
		TerminalTheme:              terminalTheme,
		EditorTheme:                editorTheme,
	}
	return a.settingsStore.Save(settings)
}

func (a *App) GetSystemColorScheme() string {
	scheme, err := appearance.GetColorScheme()
	if err != nil {
		return "unknown"
	}
	switch scheme {
	case appearance.Dark:
		return "dark"
	case appearance.Light:
		return "light"
	default:
		return "unknown"
	}
}

func (a *App) UninstallVellum(removeAllPackages bool) {
	go func() {
		if a.vellumClient == nil {
			runtime.EventsEmit(a.ctx, "vellum:uninstall-error", "Not connected")
			return
		}

		runtime.EventsEmit(a.ctx, "vellum:uninstall-start")

		err := a.vellumClient.UninstallVellum(removeAllPackages, func(line string) {
			runtime.EventsEmit(a.ctx, "vellum:uninstall-output", line)
		})

		if err != nil {
			runtime.EventsEmit(a.ctx, "vellum:uninstall-error", err.Error())
			return
		}

		a.vellumClient = nil
		runtime.EventsEmit(a.ctx, "vellum:uninstall-complete")
	}()
}

func (a *App) CleanupBrokenVellum() {
	go func() {
		a.mu.Lock()
		client := a.client
		a.mu.Unlock()

		if client == nil {
			runtime.EventsEmit(a.ctx, "vellum:cleanup-error", "Not connected")
			return
		}

		runtime.EventsEmit(a.ctx, "vellum:cleanup-start")

		session, err := client.NewSession()
		if err != nil {
			runtime.EventsEmit(a.ctx, "vellum:cleanup-error", err.Error())
			return
		}

		err = session.Run("rm -rf " + vellum.VellumRoot)
		session.Close()

		if err != nil {
			runtime.EventsEmit(a.ctx, "vellum:cleanup-error", err.Error())
			return
		}

		a.vellumClient = nil
		runtime.EventsEmit(a.ctx, "vellum:cleanup-complete")
		runtime.EventsEmit(a.ctx, "vellum:bootstrap-prompt", nil)
	}()
}

// isSystemPath returns true if the path is outside /home/root (a system path)
func isSystemPath(path string) bool {
	cleanPath := filepath.Clean(path)
	return !strings.HasPrefix(cleanPath, "/home/root")
}

// makeFilesystemWritable makes the root filesystem writable on RMPP/RMPP-M devices
func (a *App) makeFilesystemWritable(client *ssh.Client) error {
	deviceType, err := a.detectDevice()
	if err != nil {
		return nil // If we can't detect, assume it's not RMPP
	}

	if deviceType != "rmpp" && deviceType != "rmppm" {
		return nil // Only RMPP devices have read-only root
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Remount root as rw and unmount /etc overlay
	cmd := `mount -o remount,rw / 2>/dev/null; if grep -q '^overlay.*/etc' /proc/mounts; then umount -l /etc 2>/dev/null; fi`
	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("failed to make filesystem writable: %w", err)
	}

	return nil
}

// restoreFilesystem restores the /etc overlay and remounts root as read-only on RMPP/RMPP-M
func (a *App) restoreFilesystem(client *ssh.Client) error {
	deviceType, err := a.detectDevice()
	if err != nil {
		return nil
	}

	if deviceType != "rmpp" && deviceType != "rmppm" {
		return nil
	}

	// Step 1: Restore /etc overlay (if workdir exists)
	session1, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	session1.Run(`if [ -d /var/volatile/.etc-work ]; then rm -rf /var/volatile/.etc-work/* 2>/dev/null; mount -t overlay overlay -o rw,relatime,lowerdir=/etc,upperdir=/var/volatile/etc,workdir=/var/volatile/.etc-work /etc 2>/dev/null; fi`)
	session1.Close()

	// Step 2: Sync and remount root as ro
	session2, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	session2.Run("sync && mount -o remount,ro /")
	session2.Close()

	// Step 3: Verify root is actually read-only
	session3, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	output, _ := session3.CombinedOutput(`grep ' / ' /proc/mounts | grep -q '\bro\b' && echo "ro" || echo "rw"`)
	session3.Close()

	if strings.TrimSpace(string(output)) != "ro" {
		return fmt.Errorf("failed to re-enable read-only root filesystem")
	}

	return nil
}

const (
	restoreMaxRetries = 3
	restoreRetryDelay = 1 * time.Second
)

// restoreFilesystemWithRetry attempts to restore the filesystem with automatic retries
func (a *App) restoreFilesystemWithRetry(client *ssh.Client) error {
	var lastErr error

	for attempt := 1; attempt <= restoreMaxRetries; attempt++ {
		lastErr = a.restoreFilesystem(client)
		if lastErr == nil {
			return nil
		}

		if attempt < restoreMaxRetries {
			time.Sleep(restoreRetryDelay)
		}
	}

	return lastErr
}

// restoreFilesystemDeferred wraps restore with retry logic and event emission for use in defer
func (a *App) restoreFilesystemDeferred(client *ssh.Client) {
	if err := a.restoreFilesystemWithRetry(client); err != nil {
		runtime.EventsEmit(a.ctx, "filesystem:restore-error", map[string]interface{}{
			"message": err.Error(),
		})
	}
}

// withWritableRoot wraps an operation that requires writable root, handling restore and error events
func (a *App) withWritableRoot(client *ssh.Client, path string, operation func() error) error {
	if !isSystemPath(path) {
		return operation()
	}

	if err := a.makeFilesystemWritable(client); err != nil {
		return err
	}

	opErr := operation()

	restoreErr := a.restoreFilesystemWithRetry(client)
	if restoreErr != nil {
		runtime.EventsEmit(a.ctx, "filesystem:restore-error", map[string]interface{}{
			"message":            restoreErr.Error(),
			"operationSucceeded": opErr == nil,
		})
	}

	return opErr
}

// RetryRestoreFilesystem attempts to restore the read-only filesystem state (called from frontend)
func (a *App) RetryRestoreFilesystem() error {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	return a.restoreFilesystem(client)
}

// RebootDevice triggers a device reboot
func (a *App) RebootDevice() error {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	session.Run("nohup sh -c 'sleep 1 && reboot' &>/dev/null &")
	return nil
}

func (a *App) ListDirectory(dirPath string) ([]FileInfo, error) {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}

	if dirPath == "" {
		dirPath = "/home/root"
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	entries, err := sftpClient.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		fullPath := path.Join(dirPath, entry.Name())
		if dirPath == "/" {
			fullPath = "/" + entry.Name()
		}
		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    fullPath,
			Size:    entry.Size(),
			IsDir:   entry.IsDir(),
			ModTime: entry.ModTime().Unix(),
			Mode:    entry.Mode().String(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return files, nil
}

func (a *App) DownloadFile(remotePath string) {
	go func() {
		a.mu.Lock()
		client := a.client
		a.mu.Unlock()

		if client == nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": "Not connected",
			})
			return
		}

		filename := path.Base(remotePath)
		localPath, err := saveFileDialog(a.ctx, "Save File", filename)
		if err != nil || localPath == "" {
			return
		}

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to create SFTP client: %v", err),
			})
			return
		}
		defer sftpClient.Close()

		remoteFile, err := sftpClient.Open(remotePath)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to open remote file: %v", err),
			})
			return
		}
		defer remoteFile.Close()

		stat, err := remoteFile.Stat()
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to stat remote file: %v", err),
			})
			return
		}
		totalBytes := stat.Size()

		localFile, err := os.Create(localPath)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to create local file: %v", err),
			})
			return
		}
		defer localFile.Close()

		buffer := make([]byte, 32*1024)
		var transferred int64

		for {
			n, err := remoteFile.Read(buffer)
			if n > 0 {
				_, writeErr := localFile.Write(buffer[:n])
				if writeErr != nil {
					runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
						"message": fmt.Sprintf("Failed to write to local file: %v", writeErr),
					})
					return
				}
				transferred += int64(n)

				var percentage float64
				if totalBytes > 0 {
					percentage = float64(transferred) / float64(totalBytes) * 100
				}
				runtime.EventsEmit(a.ctx, "filebrowser:progress", TransferProgress{
					Filename:   filename,
					BytesSent:  transferred,
					TotalBytes: totalBytes,
					Percentage: percentage,
					Status:     "downloading",
				})
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
					"message": fmt.Sprintf("Failed to read remote file: %v", err),
				})
				return
			}
		}

		runtime.EventsEmit(a.ctx, "filebrowser:download-complete", map[string]string{
			"path": remotePath,
		})
	}()
}

func (a *App) UploadFile(remotePath string) {
	go func() {
		a.mu.Lock()
		client := a.client
		a.mu.Unlock()

		if client == nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": "Not connected",
			})
			return
		}

		localPath, err := openFileDialog(a.ctx, "Select File to Upload")
		if err != nil || localPath == "" {
			return
		}

		localFile, err := os.Open(localPath)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to open local file: %v", err),
			})
			return
		}
		defer localFile.Close()

		stat, err := localFile.Stat()
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to stat local file: %v", err),
			})
			return
		}
		totalBytes := stat.Size()
		filename := stat.Name()

		destPath := remotePath
		if strings.HasSuffix(remotePath, "/") || remotePath == "" {
			destPath = path.Join(remotePath, filename)
		}

		// Make filesystem writable for system paths on RMPP devices
		if isSystemPath(destPath) {
			if err := a.makeFilesystemWritable(client); err != nil {
				runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
					"message": fmt.Sprintf("Failed to prepare filesystem: %v", err),
				})
				return
			}
			defer a.restoreFilesystemDeferred(client)
		}

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to create SFTP client: %v", err),
			})
			return
		}
		defer sftpClient.Close()

		remoteFile, err := sftpClient.Create(destPath)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to create remote file: %v", err),
			})
			return
		}
		defer remoteFile.Close()

		buffer := make([]byte, 32*1024)
		var transferred int64

		for {
			n, err := localFile.Read(buffer)
			if n > 0 {
				_, writeErr := remoteFile.Write(buffer[:n])
				if writeErr != nil {
					runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
						"message": fmt.Sprintf("Failed to write to remote file: %v", writeErr),
					})
					return
				}
				transferred += int64(n)

				var percentage float64
				if totalBytes > 0 {
					percentage = float64(transferred) / float64(totalBytes) * 100
				}
				runtime.EventsEmit(a.ctx, "filebrowser:progress", TransferProgress{
					Filename:   filename,
					BytesSent:  transferred,
					TotalBytes: totalBytes,
					Percentage: percentage,
					Status:     "uploading",
				})
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
					"message": fmt.Sprintf("Failed to read local file: %v", err),
				})
				return
			}
		}

		runtime.EventsEmit(a.ctx, "filebrowser:upload-complete", map[string]string{
			"path": destPath,
		})
	}()
}

func (a *App) UploadFilesFromPaths(localPaths []string, remotePath string) {
	go func() {
		a.mu.Lock()
		client := a.client
		a.mu.Unlock()

		if client == nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": "Not connected",
			})
			return
		}

		if len(localPaths) == 0 {
			return
		}

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
				"message": fmt.Sprintf("Failed to create SFTP client: %v", err),
			})
			return
		}
		defer sftpClient.Close()

		for _, localPath := range localPaths {
			if err := a.uploadSingleFile(client, sftpClient, localPath, remotePath); err != nil {
				runtime.EventsEmit(a.ctx, "filebrowser:error", map[string]string{
					"message": err.Error(),
				})
			}
		}

		runtime.EventsEmit(a.ctx, "filebrowser:upload-complete", map[string]string{
			"path": remotePath,
		})
	}()
}

func (a *App) uploadSingleFile(client *ssh.Client, sftpClient *sftp.Client, localPath string, remotePath string) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file %s: %v", localPath, err)
	}
	defer localFile.Close()

	stat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %v", err)
	}

	if stat.IsDir() {
		return nil
	}

	totalBytes := stat.Size()
	filename := stat.Name()

	destPath := remotePath
	if strings.HasSuffix(remotePath, "/") || remotePath == "" {
		destPath = path.Join(remotePath, filename)
	}

	if isSystemPath(destPath) {
		if err := a.makeFilesystemWritable(client); err != nil {
			return fmt.Errorf("failed to prepare filesystem: %v", err)
		}
		defer a.restoreFilesystemDeferred(client)
	}

	remoteFile, err := sftpClient.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %v", err)
	}
	defer remoteFile.Close()

	buffer := make([]byte, 32*1024)
	var transferred int64

	for {
		n, err := localFile.Read(buffer)
		if n > 0 {
			_, writeErr := remoteFile.Write(buffer[:n])
			if writeErr != nil {
				return fmt.Errorf("failed to write to remote file: %v", writeErr)
			}
			transferred += int64(n)

			var percentage float64
			if totalBytes > 0 {
				percentage = float64(transferred) / float64(totalBytes) * 100
			}
			runtime.EventsEmit(a.ctx, "filebrowser:progress", TransferProgress{
				Filename:   filename,
				BytesSent:  transferred,
				TotalBytes: totalBytes,
				Percentage: percentage,
				Status:     "uploading",
			})
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read local file: %v", err)
		}
	}

	return nil
}

func (a *App) DeletePath(path string) error {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	// Make filesystem writable for system paths on RMPP devices
	if isSystemPath(path) {
		if err := a.makeFilesystemWritable(client); err != nil {
			return err
		}
		defer a.restoreFilesystemDeferred(client)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	stat, err := sftpClient.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	if stat.IsDir() {
		entries, err := sftpClient.ReadDir(path)
		if err != nil {
			return fmt.Errorf("failed to read directory: %w", err)
		}
		if len(entries) > 0 {
			session, err := client.NewSession()
			if err != nil {
				return fmt.Errorf("failed to create SSH session: %w", err)
			}
			defer session.Close()
			_, err = session.CombinedOutput(fmt.Sprintf("rm -rf %q", path))
			if err != nil {
				return fmt.Errorf("failed to delete directory: %w", err)
			}
		} else {
			err = sftpClient.RemoveDirectory(path)
			if err != nil {
				return fmt.Errorf("failed to remove directory: %w", err)
			}
		}
	} else {
		err = sftpClient.Remove(path)
		if err != nil {
			return fmt.Errorf("failed to delete file: %w", err)
		}
	}

	return nil
}

func (a *App) RenamePath(oldPath, newPath string) error {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	// Make filesystem writable for system paths on RMPP devices
	if isSystemPath(oldPath) || isSystemPath(newPath) {
		if err := a.makeFilesystemWritable(client); err != nil {
			return err
		}
		defer a.restoreFilesystemDeferred(client)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	err = sftpClient.Rename(oldPath, newPath)
	if err != nil {
		return fmt.Errorf("failed to rename: %w", err)
	}

	return nil
}

func (a *App) CreateDirectory(path string) error {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	// Make filesystem writable for system paths on RMPP devices
	if isSystemPath(path) {
		if err := a.makeFilesystemWritable(client); err != nil {
			return err
		}
		defer a.restoreFilesystemDeferred(client)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	err = sftpClient.Mkdir(path)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return nil
}

type BackupInfo struct {
	Name      string `json:"name"`
	Timestamp int64  `json:"timestamp"`
	Size      int64  `json:"size"`
}

func (a *App) ReadConfigFile() (string, error) {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return "", fmt.Errorf("not connected")
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	file, err := sftpClient.Open("/home/root/.config/remarkable/xochitl.conf")
	if err != nil {
		return "", fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	return string(content), nil
}

func (a *App) WriteConfigFile(content string) error {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	_, err := ini.Load([]byte(content))
	if err != nil {
		return fmt.Errorf("invalid INI syntax: %w", err)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	tmpPath := "/home/root/.config/remarkable/xochitl.conf.tmp"
	finalPath := "/home/root/.config/remarkable/xochitl.conf"

	tmpFile, err := sftpClient.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	_, err = tmpFile.Write([]byte(content))
	if err != nil {
		tmpFile.Close()
		sftpClient.Remove(tmpPath)
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	tmpFile.Close()

	// Delete the target file first - Rename only works when destination doesn't exist
	_ = sftpClient.Remove(finalPath)

	err = sftpClient.Rename(tmpPath, finalPath)
	if err != nil {
		sftpClient.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (a *App) BackupConfigFile() (string, error) {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return "", fmt.Errorf("not connected")
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	configPath := "/home/root/.config/remarkable/xochitl.conf"
	file, err := sftpClient.Open(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	backupName := fmt.Sprintf("xochitl.conf.backup-%s", time.Now().Format("2006-01-02-150405"))
	backupPath := path.Join("/home/root/.config/remarkable", backupName)

	backupFile, err := sftpClient.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer backupFile.Close()

	_, err = backupFile.Write(content)
	if err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	return backupName, nil
}

func (a *App) ListConfigBackups() ([]BackupInfo, error) {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	entries, err := sftpClient.ReadDir("/home/root/.config/remarkable")
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "xochitl.conf.backup-") {
			backups = append(backups, BackupInfo{
				Name:      entry.Name(),
				Timestamp: entry.ModTime().Unix(),
				Size:      entry.Size(),
			})
		}
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp > backups[j].Timestamp
	})

	return backups, nil
}

func (a *App) RestoreConfigBackup(backupName string) error {
	debug.Println("RestoreConfigBackup called with backupName:", backupName)
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		debug.Println("RestoreConfigBackup: not connected")
		return fmt.Errorf("not connected")
	}

	if !strings.HasPrefix(backupName, "xochitl.conf.backup-") {
		debug.Println("RestoreConfigBackup: invalid backup name")
		return fmt.Errorf("invalid backup name")
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		debug.Println("RestoreConfigBackup: failed to create SFTP client:", err)
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	backupPath := path.Join("/home/root/.config/remarkable", backupName)
	debug.Println("RestoreConfigBackup: opening backup file:", backupPath)
	file, err := sftpClient.Open(backupPath)
	if err != nil {
		debug.Println("RestoreConfigBackup: failed to open backup file:", err)
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		debug.Println("RestoreConfigBackup: failed to read backup file:", err)
		return fmt.Errorf("failed to read backup file: %w", err)
	}
	debug.Println("RestoreConfigBackup: read", len(content), "bytes from backup")

	_, err = ini.Load(content)
	if err != nil {
		debug.Println("RestoreConfigBackup: backup file has invalid INI syntax:", err)
		return fmt.Errorf("backup file has invalid INI syntax: %w", err)
	}

	debug.Println("RestoreConfigBackup: calling WriteConfigFile")
	err = a.WriteConfigFile(string(content))
	if err != nil {
		debug.Println("RestoreConfigBackup: WriteConfigFile failed:", err)
	} else {
		debug.Println("RestoreConfigBackup: success")
	}
	return err
}

func (a *App) CreateDeviceBackup() {
	go func() {
		a.mu.Lock()
		client := a.client
		deviceID := a.connectedDeviceID
		a.mu.Unlock()

		if client == nil {
			runtime.EventsEmit(a.ctx, "backup:error", map[string]string{
				"message": "Not connected to device",
			})
			return
		}

		rawDeviceName := ""
		if savedDevice, err := a.deviceStore.Get(deviceID); err == nil && savedDevice.Name != "" {
			rawDeviceName = savedDevice.Name
		}
		deviceName := "device"
		if rawDeviceName != "" {
			deviceName = sanitizeFilename(rawDeviceName)
		}

		timestamp := time.Now().Format("2006-01-02-150405")
		defaultName := fmt.Sprintf("remarkable-backup-%s-%s.tar.zst", deviceName, timestamp)
		destPath, err := saveFileDialog(a.ctx, "Save Backup", defaultName)
		if err != nil || destPath == "" {
			return
		}

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			runtime.EventsEmit(a.ctx, "backup:error", map[string]string{
				"message": fmt.Sprintf("Failed to create SFTP client: %v", err),
			})
			return
		}
		defer sftpClient.Close()

		a.backupMu.Lock()
		a.backupCancelCh = make(chan struct{})
		cancelCh := a.backupCancelCh
		a.backupMu.Unlock()

		manager := backup.Manager{
			Ctx:        a.ctx,
			SftpClient: sftpClient,
			SSHClient:  client,
			CancelCh:   cancelCh,
			ProgressFn: func(p backup.Progress) {
				runtime.EventsEmit(a.ctx, "backup:progress", p)
			},
			DeviceName: rawDeviceName,
			DeviceID:   deviceID,
		}

		sources := []string{
			"/home/root/.local/share/remarkable/xochitl",
			"/home/root/.config/remarkable",
		}

		err = manager.CreateBackup(destPath, sources)

		a.backupMu.Lock()
		a.backupCancelCh = nil
		a.backupMu.Unlock()

		if err != nil {
			runtime.EventsEmit(a.ctx, "backup:error", map[string]string{
				"message": err.Error(),
			})
		} else {
			runtime.EventsEmit(a.ctx, "backup:complete", map[string]string{
				"message": "Backup completed successfully",
				"path":    destPath,
			})
		}
	}()
}

func (a *App) SelectRestoreFile() string {
	archivePath, err := openFileDialog(a.ctx, "Select Backup to Restore")
	if err != nil || archivePath == "" {
		return ""
	}

	metadata, err := backup.ReadBackupMetadata(archivePath)
	if err == nil && metadata != nil && metadata.DeviceID != "" {
		a.mu.Lock()
		currentDeviceID := a.connectedDeviceID
		a.mu.Unlock()

		if currentDeviceID != "" && metadata.DeviceID != currentDeviceID {
			currentDeviceName := ""
			if savedDevice, err := a.deviceStore.Get(currentDeviceID); err == nil {
				currentDeviceName = savedDevice.Name
			}
			runtime.EventsEmit(a.ctx, "restore:device-mismatch", map[string]string{
				"backupDevice":  metadata.DeviceName,
				"currentDevice": currentDeviceName,
			})
		}
	}

	return archivePath
}

func (a *App) RestoreDeviceBackup(archivePath string) {
	go func() {
		a.mu.Lock()
		client := a.client
		a.mu.Unlock()

		if client == nil {
			runtime.EventsEmit(a.ctx, "restore:error", map[string]string{
				"message": "Not connected to device",
			})
			return
		}

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			runtime.EventsEmit(a.ctx, "restore:error", map[string]string{
				"message": fmt.Sprintf("Failed to create SFTP client: %v", err),
			})
			return
		}
		defer sftpClient.Close()

		a.backupMu.Lock()
		a.backupCancelCh = make(chan struct{})
		cancelCh := a.backupCancelCh
		a.backupMu.Unlock()

		manager := backup.Manager{
			Ctx:        a.ctx,
			SftpClient: sftpClient,
			SSHClient:  client,
			CancelCh:   cancelCh,
			ProgressFn: func(p backup.Progress) {
				runtime.EventsEmit(a.ctx, "restore:progress", p)
			},
		}

		err = manager.RestoreBackup(archivePath)

		a.backupMu.Lock()
		a.backupCancelCh = nil
		a.backupMu.Unlock()

		if err != nil {
			runtime.EventsEmit(a.ctx, "restore:error", map[string]string{
				"message": err.Error(),
			})
		} else {
			runtime.EventsEmit(a.ctx, "restore:complete", map[string]string{
				"message": "Restore completed successfully! Please reboot your device for changes to take effect.",
			})
		}
	}()
}

func (a *App) CancelBackup() {
	a.backupMu.Lock()
	defer a.backupMu.Unlock()

	if a.backupCancelCh != nil {
		close(a.backupCancelCh)
		a.backupCancelCh = nil
	}
}

func (a *App) RevealInFileManager(path string) {
	dir := filepath.Dir(path)

	if platform.IsRunningInFlatpak() {
		file, err := os.Open(dir)
		if err != nil {
			open.Start(dir)
			return
		}
		defer file.Close()

		if err := openuri.OpenDirectory("", file.Fd(), nil); err != nil {
			open.Start(dir)
		}
		return
	}

	open.Start(dir)
}

func (a *App) IsSleepScreenSupported() bool {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return false
	}

	output, err := a.runCommand("grep REMARKABLE_RELEASE_VERSION /usr/share/remarkable/update.conf")
	if err != nil {
		output, err = a.runCommand("grep IMG_VERSION /etc/os-release")
		if err != nil {
			return false
		}
	}

	parts := strings.SplitN(output, "=", 2)
	if len(parts) != 2 {
		return false
	}
	version := strings.Trim(strings.TrimSpace(parts[1]), "\"")

	versionParts := strings.Split(version, ".")
	if len(versionParts) < 2 {
		return false
	}

	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(versionParts[1])
	if err != nil {
		return false
	}

	// Supported: 3.2-3.13 or 3.20+
	if major == 3 {
		return (minor >= 2 && minor <= 13) || minor >= 20
	}
	return major > 3
}

func (a *App) SetSleepScreen(imagePath string) error {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	configPath := "/home/root/.config/remarkable/xochitl.conf"

	file, err := sftpClient.Open(configPath)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	content, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	cfg, err := ini.Load(content)
	if err != nil {
		return fmt.Errorf("invalid config file syntax: %w", err)
	}

	section := cfg.Section("General")
	section.Key("SleepScreenPath").SetValue(imagePath)

	var buf bytes.Buffer
	_, err = cfg.WriteTo(&buf)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	tmpPath := configPath + ".tmp"
	tmpFile, err := sftpClient.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	_, err = tmpFile.Write(buf.Bytes())
	if err != nil {
		tmpFile.Close()
		sftpClient.Remove(tmpPath)
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	tmpFile.Close()

	_ = sftpClient.Remove(configPath)
	err = sftpClient.Rename(tmpPath, configPath)
	if err != nil {
		sftpClient.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (a *App) RestartXochitl() error {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	_, err = session.CombinedOutput("systemctl restart xochitl")
	if err != nil {
		return fmt.Errorf("failed to restart xochitl: %w", err)
	}

	return nil
}
