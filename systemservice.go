package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"reManager/internal/debug"
	"reManager/internal/httputil"
	"reManager/internal/platform"
	"reManager/internal/storage"
	versionpkg "reManager/internal/version"
)

const eventSystemThemeChanged = "system-theme-changed"

func init() {
	application.RegisterEvent[ColorScheme](eventSystemThemeChanged)
}

type ColorScheme string

const (
	ColorSchemeLight   ColorScheme = "light"
	ColorSchemeDark    ColorScheme = "dark"
	ColorSchemeUnknown ColorScheme = "unknown"
)

type SystemService struct {
	*core
	os *OSService

	pendingDeepLink string
	deepLinkMu      sync.Mutex
}

func (s *SystemService) ServiceStartup(context.Context, application.ServiceOptions) error {
	go watchSystemColorScheme(func(scheme ColorScheme) {
		s.wails.Event.Emit(eventSystemThemeChanged, scheme)
	})
	return nil
}

func (s *SystemService) GetAppVersion() string {
	return version
}

func (s *SystemService) IsOutsideApplicationsFolder() bool {
	return version != "dev" && platform.IsOutsideApplicationsFolder()
}

func (s *SystemService) MoveToApplications() error {
	if err := platform.MoveToApplications(); err != nil {
		return err
	}
	s.wails.Quit()
	return nil
}

type UpdateCheckResult struct {
	UpdateAvailable bool   `json:"updateAvailable"`
	LatestVersion   string `json:"latestVersion"`
	CurrentVersion  string `json:"currentVersion"`
	ReleaseURL      string `json:"releaseURL"`
	Error           string `json:"error,omitempty"`
}

func (s *SystemService) CheckForAppUpdate() UpdateCheckResult {
	result := UpdateCheckResult{
		CurrentVersion: version,
	}

	if version == "dev" || !platform.HasUpdateCheck() {
		return result
	}

	if platform.IsRunningInFlatpak() {
		return s.checkFlatpakUpdate(result)
	}
	if platform.DistributionChannel() == "sideload" {
		return s.checkSideloadUpdate(result)
	}
	return s.checkGitHubUpdate(result)
}

func (s *SystemService) checkFlatpakUpdate(result UpdateCheckResult) UpdateCheckResult {
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

func (s *SystemService) checkGitHubUpdate(result UpdateCheckResult) UpdateCheckResult {
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

type gitHubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

// Android-only releases are published without the "Latest" mark, which stays on desktop releases, so /releases/latest can point at a release with no APK.
func (s *SystemService) checkSideloadUpdate(result UpdateCheckResult) UpdateCheckResult {
	client := httputil.NewClient(10 * time.Second)
	resp, err := client.Get("https://api.github.com/repos/rmitchellscott/remanager/releases?per_page=30")
	if err != nil {
		debug.Printf("[DEBUG] checkSideloadUpdate: request failed: %v\n", err)
		result.Error = "network_error"
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		debug.Printf("[DEBUG] checkSideloadUpdate: HTTP %d\n", resp.StatusCode)
		result.Error = "api_error"
		return result
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = "read_error"
		return result
	}

	var releases []gitHubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		result.Error = "parse_error"
		return result
	}

	latest, found := newestAPKRelease(releases)
	if !found {
		result.Error = "no_releases"
		return result
	}

	result.LatestVersion = latest.TagName
	result.ReleaseURL = latest.HTMLURL
	result.UpdateAvailable = isNewerVersion(version, latest.TagName)

	debug.Printf("[DEBUG] checkSideloadUpdate: current=%s, latest=%s, updateAvailable=%v\n",
		version, latest.TagName, result.UpdateAvailable)

	return result
}

func newestAPKRelease(releases []gitHubRelease) (gitHubRelease, bool) {
	var newest gitHubRelease
	found := false
	for _, release := range releases {
		if release.Draft || release.Prerelease || !hasAPKAsset(release) {
			continue
		}
		if !found || versionpkg.Compare(release.TagName, newest.TagName) > 0 {
			newest = release
			found = true
		}
	}
	return newest, found
}

func hasAPKAsset(release gitHubRelease) bool {
	for _, asset := range release.Assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), ".apk") {
			return true
		}
	}
	return false
}

func isNewerVersion(current, latest string) bool {
	return versionpkg.Compare(latest, current) > 0
}

type BehaviorSettings struct {
	ProxyMode                  bool `json:"proxyMode"`
	SuppressSystemFileWarnings bool `json:"suppressSystemFileWarnings"`
	PreventSleep               bool `json:"preventSleep"`
	CheckForUpdates            bool `json:"checkForUpdates"`
}

type SettingsInfo struct {
	BehaviorSettings
	Theme              string `json:"theme"`
	TerminalTheme      string `json:"terminalTheme"`
	EditorTheme        string `json:"editorTheme"`
	SSHAgentSocketPath string `json:"sshAgentSocketPath"`
}

func (s *SystemService) GetSettings() SettingsInfo {
	if s.settingsStore == nil {
		debug.Println("[DEBUG] GetSettings: settingsStore is nil")
		return SettingsInfo{
			BehaviorSettings: BehaviorSettings{
				ProxyMode:                  true,
				SuppressSystemFileWarnings: false,

				PreventSleep:    true,
				CheckForUpdates: true,
			},
			Theme:         "system",
			TerminalTheme: "match",
			EditorTheme:   "match",
		}
	}
	settings, err := s.settingsStore.Load()
	if err != nil {
		debug.Printf("[DEBUG] GetSettings: failed to load: %v\n", err)
		return SettingsInfo{
			BehaviorSettings: BehaviorSettings{
				ProxyMode:                  true,
				SuppressSystemFileWarnings: false,

				PreventSleep:    true,
				CheckForUpdates: true,
			},
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
		},
		Theme:              settings.Theme,
		TerminalTheme:      settings.TerminalTheme,
		EditorTheme:        settings.EditorTheme,
		SSHAgentSocketPath: settings.SSHAgentSocketPath,
	}
}

// A save writes only the fields it carries, so a value sync merged in while the Settings dialog was open survives it.
type SettingsPatch struct {
	ProxyMode                  *bool   `json:"proxyMode,omitempty"`
	SuppressSystemFileWarnings *bool   `json:"suppressSystemFileWarnings,omitempty"`
	PreventSleep               *bool   `json:"preventSleep,omitempty"`
	CheckForUpdates            *bool   `json:"checkForUpdates,omitempty"`
	Theme                      *string `json:"theme,omitempty"`
	TerminalTheme              *string `json:"terminalTheme,omitempty"`
	EditorTheme                *string `json:"editorTheme,omitempty"`
	SSHAgentSocketPath         *string `json:"sshAgentSocketPath,omitempty"`
}

func (s *SystemService) UpdateSettings(patch SettingsPatch) error {
	if s.settingsStore == nil {
		return fmt.Errorf("settings store not initialized")
	}
	if patch.PreventSleep != nil {
		debug.Printf("[DEBUG] UpdateSettings: preventSleep=%v, isConnected=%v\n", *patch.PreventSleep, s.isConnected())
		if *patch.PreventSleep && s.isConnected() {
			if err := s.os.startPreventSleep(); err != nil {
				debug.Printf("[DEBUG] UpdateSettings: startPreventSleep failed: %v\n", err)
			}
		} else {
			s.os.stopPreventSleep()
		}
	}

	return s.settingsStore.Update(func(settings *storage.Settings) {
		setIfPresent(&settings.ProxyMode, patch.ProxyMode)
		setIfPresent(&settings.SuppressSystemFileWarnings, patch.SuppressSystemFileWarnings)
		setIfPresent(&settings.PreventSleep, patch.PreventSleep)
		setIfPresent(&settings.CheckForUpdates, patch.CheckForUpdates)
		setIfPresent(&settings.Theme, patch.Theme)
		setIfPresent(&settings.TerminalTheme, patch.TerminalTheme)
		setIfPresent(&settings.EditorTheme, patch.EditorTheme)
		setIfPresent(&settings.SSHAgentSocketPath, patch.SSHAgentSocketPath)
	})
}

func setIfPresent[T any](field *T, value *T) {
	if value != nil {
		*field = *value
	}
}

func (s *SystemService) GetSystemColorScheme() ColorScheme {
	scheme, _ := systemColorScheme()
	return scheme
}
