package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"reManager/internal/commands"
	"reManager/internal/component"
	"reManager/internal/device"
	"reManager/internal/executor"
	"reManager/internal/installer"
	"reManager/internal/storage"
	"reManager/internal/vellum"
)

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
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select SSH Private Key",
	})
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
		if a.vellumClient == nil {
			runtime.EventsEmit(a.ctx, "vellum:bootstrap-error", "Not connected")
			return
		}

		a.mu.Lock()
		sshClient := a.client
		a.mu.Unlock()

		if sshClient == nil {
			runtime.EventsEmit(a.ctx, "vellum:bootstrap-error", "SSH client not available")
			return
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

		fmt.Printf("[%s] dialWithContextWithRetry attempt %d/%d to %s\n", time.Now().Format("15:04:05.000"), attempt+1, maxRetries+1, addr)
		client, err := a.dialWithContext(ctx, addr, config)
		if err == nil {
			fmt.Printf("[%s] dialWithContextWithRetry attempt %d/%d succeeded\n", time.Now().Format("15:04:05.000"), attempt+1, maxRetries+1)
			return client, nil
		}

		fmt.Printf("[%s] dialWithContextWithRetry attempt %d/%d failed: %v\n", time.Now().Format("15:04:05.000"), attempt+1, maxRetries+1, err)
		lastErr = err

		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		if !isRetryableError(err) {
			fmt.Printf("[%s] Error not retryable, giving up\n", time.Now().Format("15:04:05.000"))
			return nil, err
		}

		if attempt == maxRetries {
			break
		}

		backoffDuration := backoffDurations[attempt]
		fmt.Printf("[%s] dialWithContextWithRetry waiting %v before retry\n", time.Now().Format("15:04:05.000"), backoffDuration)

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
	fmt.Println("[DEBUG] SSH connected, creating vellum client")
	a.vellumClient = vellum.NewClient(&wailsExecutor{app: a})
	a.mu.Unlock()

	fmt.Println("[DEBUG] Detecting device...")
	deviceType, err := a.detectDevice()
	fmt.Printf("[DEBUG] Device detected: %s, err: %v\n", deviceType, err)
	if err != nil {
		return ConnectionResult{
			Success: true,
			Message: "Connected (could not detect device type)",
			Device:  "unknown",
		}
	}

	go func() {
		if err := a.metadata.Refresh(); err != nil {
			fmt.Printf("[DEBUG] Metadata refresh failed: %v\n", err)
		}

		fmt.Println("[DEBUG] Checking if vellum is installed...")
		installed, err := a.vellumClient.IsInstalled()
		fmt.Printf("[DEBUG] Vellum installed: %v, err: %v\n", installed, err)
		if err == nil && !installed {
			runtime.EventsEmit(a.ctx, "vellum:bootstrap-prompt", nil)
		} else if err == nil && installed {
			runtime.EventsEmit(a.ctx, "vellum:ready", nil)

			osState, err := a.vellumClient.GetOSVersionState()
			if err == nil && osState.Mismatch {
				fmt.Printf("[DEBUG] OS mismatch detected: stored=%s, current=%s\n",
					osState.StoredVersion, osState.CurrentVersion)
				runtime.EventsEmit(a.ctx, "os:mismatch", map[string]string{
					"prevVersion": osState.StoredVersion,
					"newVersion":  osState.CurrentVersion,
				})
			}

			status := a.CheckHashtabVersion()
			fmt.Printf("[DEBUG] Hashtab check: installed=%v, hashtabVersion=%s, firmwareVersion=%s, needsRebuild=%v\n",
				status.Installed, status.HashtabVersion, status.FirmwareVersion, status.NeedsRebuild)
			if status.NeedsRebuild {
				runtime.EventsEmit(a.ctx, "hashtab:version-mismatch", status)
			}

			updateStatus := a.GetUpdateServiceStatus()
			fmt.Printf("[DEBUG] Auto-update check: enabled=%v, running=%v\n", updateStatus.Enabled, updateStatus.Running)
			if updateStatus.Enabled || updateStatus.Running {
				runtime.EventsEmit(a.ctx, "autoupdate:enabled", updateStatus)
			}

			timezoneStatus := a.GetTimezoneStatus()
			fmt.Printf("[DEBUG] Timezone check: device=%s, saved=%s, needsUpdate=%v\n",
				timezoneStatus.DeviceTimezone, timezoneStatus.SavedTimezone, timezoneStatus.NeedsUpdate)

			// Auto-save device timezone if no preference saved yet
			if timezoneStatus.DeviceTimezone != "" && timezoneStatus.SavedTimezone == "" {
				a.mu.Lock()
				deviceID := a.connectedDeviceID
				a.mu.Unlock()
				if deviceID != "" && a.deviceStore != nil {
					if err := a.deviceStore.UpdateTimezone(deviceID, timezoneStatus.DeviceTimezone); err == nil {
						fmt.Printf("[DEBUG] Auto-saved device timezone: %s\n", timezoneStatus.DeviceTimezone)
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
	fmt.Printf("[%s] attemptReconnect started for device %s\n", time.Now().Format("15:04:05.000"), deviceID)

	a.reconnectMu.Lock()
	if a.reconnecting {
		a.reconnectMu.Unlock()
		fmt.Printf("[%s] Already reconnecting, skipping\n", time.Now().Format("15:04:05.000"))
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
		fmt.Printf("[%s] attemptReconnect finished\n", time.Now().Format("15:04:05.000"))
	}()

	for attempt := 0; attempt < maxReconnectAttempts; attempt++ {
		a.reconnectMu.Lock()
		stillReconnecting := a.reconnecting
		a.reconnectMu.Unlock()

		if !stillReconnecting {
			fmt.Printf("[%s] Reconnect cancelled\n", time.Now().Format("15:04:05.000"))
			return
		}

		fmt.Printf("[%s] Reconnect attempt %d/%d starting\n", time.Now().Format("15:04:05.000"), attempt+1, maxReconnectAttempts)

		runtime.EventsEmit(a.ctx, "connection:reconnecting", map[string]interface{}{
			"attempt":     attempt + 1,
			"maxAttempts": maxReconnectAttempts,
			"deviceId":    deviceID,
		})

		result := a.ConnectToSavedDevice(deviceID)
		fmt.Printf("[%s] Reconnect attempt %d/%d result: success=%v, message=%s\n", time.Now().Format("15:04:05.000"), attempt+1, maxReconnectAttempts, result.Success, result.Message)

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
			fmt.Printf("[%s] Waiting %v before next attempt\n", time.Now().Format("15:04:05.000"), reconnectBackoff[backoffIdx])
			time.Sleep(reconnectBackoff[backoffIdx])
			fmt.Printf("[%s] Backoff complete\n", time.Now().Format("15:04:05.000"))
		}
	}

	fmt.Printf("[%s] All reconnect attempts exhausted\n", time.Now().Format("15:04:05.000"))
	runtime.EventsEmit(a.ctx, "connection:failed", map[string]interface{}{
		"reason":   "Maximum reconnection attempts exceeded",
		"deviceId": deviceID,
	})
}

func (a *App) runCommand(cmd string) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("not connected")
	}

	fmt.Printf("[DEBUG] runCommand creating session: %s\n", cmd[:min(50, len(cmd))])
	session, err := a.client.NewSession()
	if err != nil {
		fmt.Printf("[DEBUG] runCommand session error: %v\n", err)
		return "", err
	}
	defer session.Close()

	fmt.Printf("[DEBUG] runCommand executing: %s\n", cmd[:min(50, len(cmd))])
	output, err := session.CombinedOutput(cmd)
	fmt.Printf("[DEBUG] runCommand done: %s, err: %v\n", cmd[:min(50, len(cmd))], err)
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
	fmt.Println("[DEBUG] RunCommandWithOutput called:", cmd[:min(50, len(cmd))], "requiresPTY:", requiresPTY)
	go func() {
		a.mu.Lock()

		if a.client == nil {
			a.mu.Unlock()
			fmt.Println("[DEBUG] Not connected, emitting error")
			runtime.EventsEmit(a.ctx, "command:output", "Error: not connected\n")
			runtime.EventsEmit(a.ctx, "command:done", false)
			return
		}

		session, err := a.client.NewSession()
		if err != nil {
			a.mu.Unlock()
			fmt.Println("[DEBUG] Session error:", err)
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
				fmt.Println("[DEBUG] PTY request error:", err)
				runtime.EventsEmit(a.ctx, "command:output", fmt.Sprintf("Error requesting PTY: %v\n", err))
				runtime.EventsEmit(a.ctx, "command:done", false)
				return
			}
			fmt.Println("[DEBUG] PTY allocated successfully")
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

		fmt.Println("[DEBUG] Starting command")
		if err := session.Start(cmd); err != nil {
			fmt.Println("[DEBUG] Start error:", err)
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
					fmt.Printf("[DEBUG] stdout: %d bytes\n", n)
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
					fmt.Printf("[DEBUG] stderr: %d bytes\n", n)
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
		fmt.Println("[DEBUG] Command done, success:", err == nil)
		runtime.EventsEmit(a.ctx, "command:done", err == nil)
	}()
}

func (a *App) StopCommand() {
	a.mu.Lock()
	stdin := a.commandStdin
	a.mu.Unlock()

	if stdin != nil {
		fmt.Println("[DEBUG] Sending Ctrl+C (0x03) to stdin")
		_, err := stdin.Write([]byte{0x03})
		if err != nil {
			fmt.Printf("[DEBUG] Error writing Ctrl+C to stdin: %v\n", err)
		} else {
			fmt.Println("[DEBUG] Ctrl+C sent successfully")
		}
	} else {
		fmt.Println("[DEBUG] No stdin available to send Ctrl+C")
	}
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
		fmt.Println("[DEBUG] GetUpdateServiceStatus: client is nil")
		return status
	}

	output, err := a.runCommand("systemctl is-enabled update-engine.service")
	fmt.Printf("[DEBUG] GetUpdateServiceStatus: is-enabled output=%q, err=%v\n", output, err)
	if err == nil {
		status.Enabled = strings.TrimSpace(output) == "enabled"
	}

	output, err = a.runCommand("systemctl is-active update-engine.service")
	fmt.Printf("[DEBUG] GetUpdateServiceStatus: is-active output=%q, err=%v\n", output, err)
	if err == nil {
		status.Running = strings.TrimSpace(output) == "active"
	}

	fmt.Printf("[DEBUG] GetUpdateServiceStatus: returning enabled=%v, running=%v\n", status.Enabled, status.Running)
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

func (a *App) GetPackages(deviceType string) []PackageInfo {
	fmt.Printf("[DEBUG] GetPackages called, metadata=%p, deviceType=%s\n", a.metadata, deviceType)
	packages := a.metadata.GetAllPackages()
	fmt.Printf("[DEBUG] GetPackages got %d packages\n", len(packages))
	var result []PackageInfo

	for _, pkg := range packages {
		if hiddenPackages[pkg.Name] {
			continue
		}
		if len(pkg.Devices) > 0 && !containsString(pkg.Devices, deviceType) {
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

	fmt.Printf("[DEBUG] GetPackages returning %d PackageInfo\n", len(result))
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
	fmt.Println("[DEBUG] GetPackageCompatibilityStatus: called")
	if a.vellumClient == nil {
		fmt.Println("[DEBUG] GetPackageCompatibilityStatus: vellumClient is nil")
		return PackageCompatibilityStatus{FetchFailed: true}
	}

	osState, err := a.vellumClient.GetOSVersionState()
	if err != nil {
		fmt.Printf("[DEBUG] GetPackageCompatibilityStatus: GetOSVersionState error: %v\n", err)
		return PackageCompatibilityStatus{FetchFailed: true}
	}
	fmt.Printf("[DEBUG] GetPackageCompatibilityStatus: osState=%+v\n", osState)

	installed, err := a.vellumClient.List()
	if err != nil {
		fmt.Printf("[DEBUG] GetPackageCompatibilityStatus: List error: %v\n", err)
		return PackageCompatibilityStatus{FetchFailed: true}
	}
	fmt.Printf("[DEBUG] GetPackageCompatibilityStatus: installed=%v\n", installed)

	var filteredInstalled []string
	for _, pkg := range installed {
		if !hiddenPackages[pkg] {
			filteredInstalled = append(filteredInstalled, pkg)
		}
	}
	fmt.Printf("[DEBUG] GetPackageCompatibilityStatus: filteredInstalled=%v\n", filteredInstalled)

	compat, err := a.vellumClient.CheckOSCompatibility(osState.CurrentVersion)
	fmt.Printf("[DEBUG] GetPackageCompatibilityStatus: compat=%+v, err=%v\n", compat, err)

	if err != nil && (compat == nil || compat.FetchFailed) {
		fmt.Println("[DEBUG] GetPackageCompatibilityStatus: returning with FetchFailed due to error")
		return PackageCompatibilityStatus{
			InstalledPackages: filteredInstalled,
			CurrentOsVersion:  osState.CurrentVersion,
			StoredOsVersion:   osState.StoredVersion,
			FetchFailed:       true,
		}
	}

	allEmpty := len(compat.Compatible) == 0 && len(compat.Incompatible) == 0 && len(compat.NoConstraint) == 0
	if compat.FetchFailed || allEmpty {
		fmt.Printf("[DEBUG] GetPackageCompatibilityStatus: fallback (FetchFailed=%v, allEmpty=%v)\n", compat.FetchFailed, allEmpty)
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
	fmt.Printf("[DEBUG] GetPackageCompatibilityStatus: returning result=%+v\n", result)
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
		fmt.Printf("[DEBUG] SimulateAdd failed: %v, using packageNames only\n", err)
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
		fmt.Printf("[DEBUG] SimulateDel failed: %v\n", err)
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
			fmt.Printf("[DEBUG] SimulateDelRecursive failed: %v\n", err)
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
				fmt.Printf("[DEBUG] SimulateDel failed: %v, using packageNames only\n", err)
				allPackages = packageNames
			} else if len(simResult.Blocked) > 0 {
				// Packages are blocked by dependents - prompt user
				fmt.Printf("[DEBUG] Packages blocked: %v\n", simResult.Blocked)
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
					fmt.Printf("[DEBUG] SimulateDelRecursive failed: %v\n", err)
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
	fmt.Printf("[DEBUG] ExecuteWithOutput waiting for lock: %s\n", cmd[:min(50, len(cmd))])
	e.app.mu.Lock()
	fmt.Printf("[DEBUG] ExecuteWithOutput got lock: %s\n", cmd[:min(50, len(cmd))])
	defer func() {
		e.app.mu.Unlock()
		fmt.Printf("[DEBUG] ExecuteWithOutput released lock: %s\n", cmd[:min(50, len(cmd))])
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
	TabVisibility map[string]bool `json:"tabVisibility"`
	ProxyMode     bool            `json:"proxyMode"`
}

func (a *App) GetSettings() SettingsInfo {
	if a.settingsStore == nil {
		return SettingsInfo{
			TabVisibility: map[string]bool{"mods": true, "maintenance": true},
			ProxyMode:     true,
		}
	}
	settings, err := a.settingsStore.Load()
	if err != nil {
		return SettingsInfo{
			TabVisibility: map[string]bool{"mods": true, "maintenance": true},
			ProxyMode:     true,
		}
	}
	return SettingsInfo{
		TabVisibility: map[string]bool{
			"mods":        settings.TabVisibility.Mods,
			"maintenance": settings.TabVisibility.Maintenance,
		},
		ProxyMode: settings.ProxyMode,
	}
}

func (a *App) SaveSettings(tabVisibility map[string]bool, proxyMode bool) error {
	if a.settingsStore == nil {
		return fmt.Errorf("settings store not initialized")
	}
	settings := &storage.Settings{
		TabVisibility: storage.TabVisibility{
			Mods:        tabVisibility["mods"],
			Maintenance: tabVisibility["maintenance"],
		},
		ProxyMode: proxyMode,
	}
	return a.settingsStore.Save(settings)
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
