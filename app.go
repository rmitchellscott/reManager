package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"github.com/rymdport/portal/filechooser"
	"github.com/rymdport/portal/settings"
	"github.com/rymdport/portal/settings/appearance"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"reManager/internal/component"
	"reManager/internal/debug"
	"reManager/internal/httputil"
	"reManager/internal/logger"
	"reManager/internal/platform"
	"reManager/internal/storage"
	"reManager/internal/vellum"
	versionpkg "reManager/internal/version"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"
)

func resolveDialogDir(defaultDir string) string {
	if defaultDir != "" {
		if info, err := os.Stat(defaultDir); err == nil && info.IsDir() {
			return defaultDir
		}
	}
	home, _ := os.UserHomeDir()
	return home
}

func openFileDialog(ctx context.Context, title, defaultDir string) (string, error) {
	dir := resolveDialogDir(defaultDir)
	if platform.IsRunningInFlatpak() {
		files, err := filechooser.OpenFile("", title, &filechooser.OpenFileOptions{
			CurrentFolder: dir,
		})
		debug.Println("[DEBUG] openFileDialog: files:", files, "err:", err)
		if err != nil {
			return "", err
		}
		if len(files) == 0 {
			return "", nil
		}
		return strings.TrimPrefix(files[0], "file://"), nil
	}
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: dir,
	})
}

func openMultipleFilesDialog(ctx context.Context, title, defaultDir string) ([]string, error) {
	dir := resolveDialogDir(defaultDir)
	if platform.IsRunningInFlatpak() {
		files, err := filechooser.OpenFile("", title, &filechooser.OpenFileOptions{
			CurrentFolder: dir,
			Multiple:      true,
		})
		debug.Println("[DEBUG] openMultipleFilesDialog: files:", files, "err:", err)
		if err != nil {
			return nil, err
		}
		for i, f := range files {
			files[i] = strings.TrimPrefix(f, "file://")
		}
		return files, nil
	}
	return runtime.OpenMultipleFilesDialog(ctx, runtime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: dir,
	})
}

func saveFileDialog(ctx context.Context, title, defaultFilename, defaultDir string) (string, error) {
	dir := resolveDialogDir(defaultDir)
	if platform.IsRunningInFlatpak() {
		files, err := filechooser.SaveFile("", title, &filechooser.SaveFileOptions{
			CurrentFolder: dir,
			CurrentName:   defaultFilename,
		})
		debug.Println("[DEBUG] saveFileDialog: files:", files, "err:", err)
		if err != nil {
			return "", err
		}
		if len(files) == 0 {
			return "", nil
		}
		return strings.TrimPrefix(files[0], "file://"), nil
	}
	return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title:            title,
		DefaultFilename:  defaultFilename,
		DefaultDirectory: dir,
	})
}

func openDirectoryDialog(ctx context.Context, title, defaultDir string) (string, error) {
	dir := resolveDialogDir(defaultDir)
	if platform.IsRunningInFlatpak() {
		files, err := filechooser.OpenFile("", title, &filechooser.OpenFileOptions{
			CurrentFolder: dir,
			Directory:     true,
		})
		debug.Println("[DEBUG] openDirectoryDialog: files:", files, "err:", err)
		if err != nil {
			return "", err
		}
		if len(files) == 0 {
			return "", nil
		}
		return strings.TrimPrefix(files[0], "file://"), nil
	}
	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: dir,
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
	dialogResponse chan string
	deviceStore    *storage.DeviceStore
	settingsStore  *storage.SettingsStore
	bundleStore      *storage.BundleStore
	deviceInfoCache  *storage.DeviceInfoCacheStore
	vellumClient   *vellum.Client
	metadata       *vellum.MetadataStore

	logger          *logger.Logger
	operationLog    *logger.CommandLog
	supportBundleID string

	keepaliveStop          chan struct{}
	connectedDeviceID      string
	connectedDeviceType    rmdevice.Type
	connectedDeviceArch    rmdevice.Architecture
	connectedFirmware      string
	writeableRootBusy      bool
	reconnecting           bool
	reconnectMu            sync.Mutex
	fastDialMode           bool
	installCancelCh        chan struct{}
	osInstallCancelCh      chan struct{}
	backupCancelCh         chan struct{}
	backupMu               sync.Mutex
	folderTransferCancelCh chan struct{}
	folderTransferMu       sync.Mutex

	agentConn net.Conn

	shellSession *ssh.Session
	shellStdin   io.WriteCloser
	shellMu      sync.Mutex
	shellActive  bool

	preventSleepStop chan struct{}
	penInputDevice   string
}

func NewApp() *App {
	return &App{}
}

func (a *App) getClient() *ssh.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client
}

func (a *App) getSFTPClient() (*sftp.Client, error) {
	client := a.getClient()
	if client == nil {
		return nil, fmt.Errorf("not connected")
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create SFTP client: %w", err)
	}
	return sftpClient, nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	configDir, err := storage.GetConfigDir()
	if err == nil {
		l, lerr := logger.New(configDir)
		if lerr == nil {
			a.logger = l
			debug.SetFileLogger(l)
			a.logger.LogEvent("APP", "reManager starting, version="+version)
			go a.logger.CleanupOldCommandLogs(30 * 24 * time.Hour)
		}
	}

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

	bundleStore, err := storage.NewBundleStore()
	if err != nil {
		fmt.Printf("Warning: could not initialize bundle store: %v\n", err)
	}
	a.bundleStore = bundleStore

	deviceInfoCache, err := storage.NewDeviceInfoCacheStore()
	if err != nil {
		fmt.Printf("Warning: could not initialize device info cache: %v\n", err)
	}
	a.deviceInfoCache = deviceInfoCache

	a.metadata = vellum.NewMetadataStore()
	go func() {
		if err := a.metadata.Load(); err != nil {
			debug.Printf("[DEBUG] Failed to load metadata: %v\n", err)
			runtime.EventsEmit(a.ctx, "metadata:error", err.Error())
			return
		}
		runtime.EventsEmit(a.ctx, "metadata:loaded")
	}()

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
	if a.logger != nil {
		a.logger.LogEvent("APP", "reManager shutting down")
	}
	a.Disconnect()
	if a.logger != nil {
		a.logger.Close()
	}
}

type TransferProgress struct {
	Filename   string  `json:"filename"`
	BytesSent  int64   `json:"bytesSent"`
	TotalBytes int64   `json:"totalBytes"`
	Percentage float64 `json:"percentage"`
	Status     string  `json:"status"`
}

type FolderTransferProgress struct {
	CurrentFile    string  `json:"currentFile"`
	FilesDone      int     `json:"filesDone"`
	FilesTotal     int     `json:"filesTotal"`
	BytesDone      int64   `json:"bytesDone"`
	BytesTotal     int64   `json:"bytesTotal"`
	Percentage     float64 `json:"percentage"`
	Status         string  `json:"status"`
	ContainsFolder bool    `json:"containsFolder"`
}

type DialogActionRequest struct {
	Id    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type DialogRequest struct {
	Title             string                `json:"title"`
	Message           string                `json:"message"`
	Note              string                `json:"note"`
	Steps             []string              `json:"steps"`
	ConfirmText       string                `json:"confirmText"`
	CancelText        string                `json:"cancelText"`
	InProgressMessage string                `json:"inProgressMessage"`
	InfoOnly          bool                  `json:"infoOnly"`
	InstallFlow       bool                  `json:"installFlow"`
	Actions           []DialogActionRequest `json:"actions"`
}

func dialogRequestFromConfig(cfg *component.DialogConfig) DialogRequest {
	var actions []DialogActionRequest
	for _, a := range cfg.Actions {
		actions = append(actions, DialogActionRequest{
			Id:    a.Id,
			Label: a.Label,
			Type:  a.Type,
			Value: a.Value,
		})
	}
	return DialogRequest{
		Title:             cfg.Title,
		Message:           cfg.Message,
		Note:              cfg.Note,
		Steps:             cfg.Steps,
		ConfirmText:       cfg.ConfirmText,
		CancelText:        cfg.CancelText,
		InProgressMessage: cfg.InProgressMessage,
		InfoOnly:          cfg.InfoOnly,
		InstallFlow:       cfg.InstallFlow,
		Actions:           actions,
	}
}

func (a *App) RespondToDialog(response string) {
	if a.dialogResponse != nil {
		a.dialogResponse <- response
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

type wailsExecutor struct {
	app *App
}

func (e *wailsExecutor) Execute(cmds []component.CommandResult) error {
	var operationErr error
	if e.app.logger != nil && len(cmds) > 0 {
		name := cmds[0].Description
		if name == "" {
			name = "execute"
		}
		e.app.operationLog = e.app.logger.StartCommandLog(e.app.connectedDeviceID, name)
		defer func() {
			e.app.operationLog.WriteExitCode(operationErr)
			e.app.operationLog.Close()
			e.app.operationLog = nil
		}()
	}

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
			operationErr = fmt.Errorf("command failed: %s", cmd.Description)
			return operationErr
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
	var cmdLog *logger.CommandLog
	if e.app.logger != nil {
		cmdLog = e.app.logger.StartCommandLog(e.app.connectedDeviceID, cmd)
	}
	defer cmdLog.Close()

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
			line := scanner.Text()
			cmdLog.Write(line + "\n")
			if onOutput != nil {
				onOutput(line)
			}
		}
	}

	go readLines(stdout)
	go readLines(stderr)

	err = session.Wait()
	wg.Wait()
	cmdLog.WriteExitCode(err)
	return err
}

func (a *App) GetAppVersion() string {
	return version
}

type UpdateCheckResult struct {
	UpdateAvailable bool   `json:"updateAvailable"`
	LatestVersion   string `json:"latestVersion"`
	CurrentVersion  string `json:"currentVersion"`
	ReleaseURL      string `json:"releaseURL"`
	Error           string `json:"error,omitempty"`
}

func (a *App) CheckForAppUpdate() UpdateCheckResult {
	result := UpdateCheckResult{
		CurrentVersion: version,
	}

	if version == "dev" {
		return result
	}

	if platform.IsRunningInFlatpak() {
		return a.checkFlatpakUpdate(result)
	}
	return a.checkGitHubUpdate(result)
}

func (a *App) checkFlatpakUpdate(result UpdateCheckResult) UpdateCheckResult {
	client := httputil.NewClient(10 * time.Second)
	resp, err := client.Get("https://flathub.org/api/v2/appstream/io.scottlabs.reManager")
	if err != nil {
		debug.Printf("[DEBUG] checkFlatpakUpdate: request failed: %v\n", err)
		result.Error = "network_error"
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		debug.Printf("[DEBUG] checkFlatpakUpdate: HTTP %d\n", resp.StatusCode)
		result.Error = "api_error"
		return result
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = "read_error"
		return result
	}

	var appstream struct {
		Releases []struct {
			Version   string `json:"version"`
			Timestamp string `json:"timestamp"`
		} `json:"releases"`
	}

	if err := json.Unmarshal(body, &appstream); err != nil {
		result.Error = "parse_error"
		return result
	}

	if len(appstream.Releases) == 0 {
		result.Error = "no_releases"
		return result
	}

	latest := appstream.Releases[0].Version
	result.LatestVersion = latest
	result.ReleaseURL = "https://flathub.org/apps/io.scottlabs.reManager"
	result.UpdateAvailable = isNewerVersion(version, latest)

	debug.Printf("[DEBUG] checkFlatpakUpdate: current=%s, latest=%s, updateAvailable=%v\n",
		version, latest, result.UpdateAvailable)

	return result
}

func (a *App) checkGitHubUpdate(result UpdateCheckResult) UpdateCheckResult {
	client := httputil.NewClient(10 * time.Second)
	resp, err := client.Get("https://api.github.com/repos/rmitchellscott/remanager/releases/latest")
	if err != nil {
		debug.Printf("[DEBUG] checkGitHubUpdate: request failed: %v\n", err)
		result.Error = "network_error"
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		debug.Printf("[DEBUG] checkGitHubUpdate: HTTP %d\n", resp.StatusCode)
		result.Error = "api_error"
		return result
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = "read_error"
		return result
	}

	if err := json.Unmarshal(body, &release); err != nil {
		result.Error = "parse_error"
		return result
	}

	result.LatestVersion = release.TagName
	result.ReleaseURL = "https://remanager.io"
	result.UpdateAvailable = isNewerVersion(version, release.TagName)

	debug.Printf("[DEBUG] checkGitHubUpdate: current=%s, latest=%s, updateAvailable=%v\n",
		version, release.TagName, result.UpdateAvailable)

	return result
}

func isNewerVersion(current, latest string) bool {
	return versionpkg.Compare(latest, current) > 0
}

type BehaviorSettings struct {
	ProxyMode                  bool `json:"proxyMode"`
	SuppressSystemFileWarnings bool `json:"suppressSystemFileWarnings"`
	PreventSleep               bool `json:"preventSleep"`
	CheckForUpdates            bool `json:"checkForUpdates"`
	SuppressGuideOffer         bool `json:"suppressGuideOffer"`
}

type SettingsInfo struct {
	BehaviorSettings
	TabVisibility      map[string]bool `json:"tabVisibility"`
	Theme              string          `json:"theme"`
	TerminalTheme      string          `json:"terminalTheme"`
	EditorTheme        string          `json:"editorTheme"`
	SSHAgentSocketPath string          `json:"sshAgentSocketPath"`
}

func (a *App) GetSettings() SettingsInfo {
	if a.settingsStore == nil {
		debug.Println("[DEBUG] GetSettings: settingsStore is nil")
		return SettingsInfo{
			BehaviorSettings: BehaviorSettings{
				ProxyMode:                  true,
				SuppressSystemFileWarnings: false,

				PreventSleep:               true,
				CheckForUpdates:            true,
			},
			TabVisibility: map[string]bool{"mods": true, "maintenance": true, "utilities": true},
			Theme:         "system",
			TerminalTheme: "match",
			EditorTheme:   "match",
		}
	}
	settings, err := a.settingsStore.Load()
	if err != nil {
		debug.Printf("[DEBUG] GetSettings: failed to load: %v\n", err)
		return SettingsInfo{
			BehaviorSettings: BehaviorSettings{
				ProxyMode:                  true,
				SuppressSystemFileWarnings: false,

				PreventSleep:               true,
				CheckForUpdates:            true,
			},
			TabVisibility: map[string]bool{"mods": true, "maintenance": true, "utilities": true},
			Theme:         "system",
			TerminalTheme: "match",
			EditorTheme:   "match",
		}
	}
	debug.Printf("[DEBUG] GetSettings: loaded PreventSleep=%v\n", settings.PreventSleep)
	return SettingsInfo{
		BehaviorSettings: BehaviorSettings{
			ProxyMode:                  settings.ProxyMode,
			SuppressSystemFileWarnings: settings.SuppressSystemFileWarnings,
			PreventSleep:               settings.PreventSleep,
			CheckForUpdates:            settings.CheckForUpdates,
			SuppressGuideOffer:         settings.SuppressGuideOffer,
		},
		TabVisibility:      settings.TabVisibility,
		Theme:              settings.Theme,
		TerminalTheme:      settings.TerminalTheme,
		EditorTheme:        settings.EditorTheme,
		SSHAgentSocketPath: settings.SSHAgentSocketPath,
	}
}

func (a *App) SaveSettings(tabVisibility map[string]bool, proxyMode bool, suppressSystemFileWarnings bool, preventSleep bool, theme string, terminalTheme string, editorTheme string, checkForUpdates bool, sshAgentSocketPath string) error {
	debug.Printf("[DEBUG] SaveSettings: preventSleep=%v, isConnected=%v\n", preventSleep, a.IsConnected())
	if a.settingsStore == nil {
		return fmt.Errorf("settings store not initialized")
	}
	existing, _ := a.settingsStore.Load()
	var suppressGuideOffer bool
	if existing != nil {
		suppressGuideOffer = existing.SuppressGuideOffer
	}

	settings := &storage.Settings{
		TabVisibility:              storage.TabVisibility(tabVisibility),
		ProxyMode:                  proxyMode,
		SuppressSystemFileWarnings: suppressSystemFileWarnings,
		PreventSleep:               preventSleep,
		Theme:                      theme,
		TerminalTheme:              terminalTheme,
		EditorTheme:                editorTheme,
		CheckForUpdates:            checkForUpdates,
		SuppressGuideOffer:         suppressGuideOffer,
		SSHAgentSocketPath:         sshAgentSocketPath,
	}

	if preventSleep && a.IsConnected() {
		debug.Println("[DEBUG] SaveSettings: starting prevent sleep")
		if err := a.StartPreventSleep(); err != nil {
			debug.Printf("[DEBUG] SaveSettings: StartPreventSleep failed: %v\n", err)
		}
	} else {
		debug.Println("[DEBUG] SaveSettings: stopping prevent sleep")
		a.StopPreventSleep()
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
		client := a.getClient()

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
