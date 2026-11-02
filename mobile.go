package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"reManager/internal/storage"
)

// Android's sandbox forbids the XDG-default home dir and makes os.TempDir (/data/local/tmp) writable only by the shell user, so storage and TMPDIR are redirected to the Wails-exposed private files dir.
func configureMobileStorage() {
	if runtime.GOOS != "android" {
		return
	}
	path := application.Mobile.StoragePath()
	if path == "" {
		return
	}
	storage.SetConfigDir(path)
	tempDir := filepath.Join(path, "tmp")
	if err := os.MkdirAll(tempDir, 0700); err == nil {
		os.Setenv("TMPDIR", tempDir)
	}
}

func isMobile() bool {
	return runtime.GOOS == "ios" || runtime.GOOS == "android"
}

// Mobile has no save panel: the target is a path in app storage handed to the share sheet. iOS uses the sandbox Documents dir (the only location Files surfaces, via UIFileSharingEnabled); Android reports HOME as "/", so it uses its private files dir instead.
func sandboxSavePath(filename string) (string, error) {
	base := ""
	if runtime.GOOS == "android" {
		base = application.Mobile.StoragePath()
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = home
	}
	dir := filepath.Join(base, "Documents")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create Documents directory: %w", err)
	}
	return filepath.Join(dir, sanitizeFilename(filename)), nil
}

// The --background values from index.css; the native backdrop shows wherever the page has not painted yet, most visibly at the screen edges mid-rotation.
var (
	lightBackdrop = application.NewRGB(255, 255, 255)
	darkBackdrop  = application.NewRGB(10, 10, 10)
)

// The webview runs full bleed, so the status bar draws over the page: its glyphs and the native backdrop follow the frontend-resolved theme, not the system appearance iOS would otherwise pick.
func (s *SystemService) ApplyNativeTheme(appTheme string) {
	if !isMobile() {
		return
	}
	statusBarStyle, backdrop := "dark", lightBackdrop
	if appTheme == "dark" {
		statusBarStyle, backdrop = "light", darkBackdrop
	}
	setStatusBarContentStyle(statusBarStyle)
	for _, window := range s.wails.Window.GetAll() {
		window.SetBackgroundColour(backdrop)
	}
}

// iOS shares the file by URL; Android's share intent carries only text, so a file URL arrives as its path and the file goes through the save dialog instead.
func shareFile(path string) {
	if !isMobile() || path == "" {
		return
	}
	share := map[string]string{"url": "file://" + path}
	if runtime.GOOS == "android" {
		share = map[string]string{"file": path}
	}
	payload, err := json.Marshal(share)
	if err != nil {
		return
	}
	application.Mobile.Share(string(payload))
}

// Android's folder picker returns a document tree rather than a path, so folder transfers stage a copy in private storage: a download copies into the picked folder when done, an upload copies out of it before it starts.
func stagesFolderTransfers() bool {
	return runtime.GOOS == "android"
}

func shareFolder(path string) {
	if !stagesFolderTransfers() || path == "" {
		return
	}
	payload, err := json.Marshal(map[string]string{"folder": path})
	if err != nil {
		return
	}
	application.Mobile.Share(string(payload))
}

func removeStagedFolderDownload(job *transferJob) {
	if stagesFolderTransfers() && job.kind == TransferDownload && job.isDir {
		os.RemoveAll(job.localPath)
	}
}

// The activity reports the staged copy as an event, not a return value, because the picker and copy both finish long after the request.
func (s *FilesService) pickFolderToUpload(onPicked func(localDir string)) {
	var stopListening func()
	stopListening = s.wails.Event.On("android:folderPicked", func(event *application.CustomEvent) {
		stopListening()
		data, _ := event.Data.(map[string]any)
		if localDir, _ := data["path"].(string); localDir != "" {
			onPicked(localDir)
		}
	})
	payload, err := json.Marshal(map[string]bool{"pickFolderToUpload": true})
	if err != nil {
		stopListening()
		return
	}
	application.Mobile.Share(string(payload))
}

func removeStagedFolderUpload(job *transferJob) {
	if stagesFolderTransfers() && job.kind == TransferUpload && job.isDir &&
		strings.HasPrefix(job.localPath, os.TempDir()+string(filepath.Separator)) {
		os.RemoveAll(filepath.Dir(job.localPath))
	}
}
