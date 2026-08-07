package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"github.com/skratchdot/open-golang/open"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reManager/internal/backup"
	"reManager/internal/logger"
	"reManager/internal/platform"

	"github.com/rymdport/portal/openuri"
)

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime int64  `json:"modTime"`
	Mode    string `json:"mode"`
}

type cmdLogWriter struct {
	cmdLog *logger.CommandLog
}

func (w *cmdLogWriter) Write(p []byte) (n int, err error) {
	w.cmdLog.Write(string(p))
	return len(p), nil
}

func (a *App) ListDirectory(dirPath string) ([]FileInfo, error) {
	if dirPath == "" {
		dirPath = "/home/root"
	}

	sftpClient, err := a.getSFTPClient()
	if err != nil {
		return nil, err
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

func (a *App) countRemoteFolder(sftpClient *sftp.Client, remotePath string) (int, int64, error) {
	var filesTotal int
	var bytesTotal int64

	walker := sftpClient.Walk(remotePath)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			continue
		}
		info := walker.Stat()
		if info.Mode()&os.ModeSymlink != 0 {
			filesTotal++
		} else if !info.IsDir() {
			filesTotal++
			bytesTotal += info.Size()
		}
	}

	return filesTotal, bytesTotal, nil
}

func (a *App) SelectFilesForUpload() []string {
	var lastDir string
	if a.settingsStore != nil {
		if settings, err := a.settingsStore.Load(); err == nil {
			lastDir = settings.LastUploadDir
		}
	}

	files, err := openMultipleFilesDialog(a.ctx, "Select files to upload", lastDir)
	if err != nil || len(files) == 0 {
		return []string{}
	}

	if a.settingsStore != nil {
		if settings, err := a.settingsStore.Load(); err == nil {
			settings.LastUploadDir = filepath.Dir(files[0])
			_ = a.settingsStore.Save(settings)
		}
	}

	return files
}

func (a *App) countLocalFolder(localPath string) (int, int64, error) {
	var filesTotal int
	var bytesTotal int64

	err := filepath.WalkDir(localPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			filesTotal++
		} else if !d.IsDir() {
			filesTotal++
			bytesTotal += info.Size()
		}
		return nil
	})

	return filesTotal, bytesTotal, err
}

const uploadTempSuffix = ".remanager-tmp"

const maxRemoteFilenameLength = 255

type remoteFileWriter struct {
	sftpClient   *sftp.Client
	file         *sftp.File
	tempPath     string
	destPath     string
	replacedPerm *fs.FileMode
}

func remoteTempPath(destPath string) string {
	dir, name := path.Split(destPath)
	if len(name)+len(uploadTempSuffix) > maxRemoteFilenameLength {
		name = name[:maxRemoteFilenameLength-len(uploadTempSuffix)]
	}
	return dir + name + uploadTempSuffix
}

func resolveSymlinkedPath(sftpClient *sftp.Client, remotePath string) (string, error) {
	info, err := sftpClient.Lstat(remotePath)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return remotePath, nil
	}
	return sftpClient.RealPath(remotePath)
}

func openRemoteFileForReplace(sftpClient *sftp.Client, destPath string) (*remoteFileWriter, error) {
	destPath, err := resolveSymlinkedPath(sftpClient, destPath)
	if err != nil {
		return nil, err
	}

	writer := &remoteFileWriter{sftpClient: sftpClient, destPath: destPath}

	if info, err := sftpClient.Stat(destPath); err == nil && info.Mode().IsRegular() {
		perm := info.Mode().Perm()
		writer.replacedPerm = &perm
	}

	tempPath := remoteTempPath(destPath)
	_ = sftpClient.Remove(tempPath)

	file, err := sftpClient.Create(tempPath)
	if err != nil {
		return nil, err
	}

	writer.file = file
	writer.tempPath = tempPath
	return writer, nil
}

func (w *remoteFileWriter) Write(p []byte) (int, error) {
	return w.file.Write(p)
}

func (w *remoteFileWriter) WriteAt(p []byte, off int64) (int, error) {
	return w.file.WriteAt(p, off)
}

func (w *remoteFileWriter) TempPath() string {
	return w.tempPath
}

// Commit verifies the uploaded size before renaming, so a short write is not published under the destination name.
func (w *remoteFileWriter) Commit(expectedSize int64) error {
	if err := w.file.Close(); err != nil {
		_ = w.sftpClient.Remove(w.tempPath)
		return err
	}

	if expectedSize >= 0 {
		info, err := w.sftpClient.Stat(w.tempPath)
		if err != nil {
			_ = w.sftpClient.Remove(w.tempPath)
			return fmt.Errorf("failed to verify uploaded file: %w", err)
		}
		if info.Size() != expectedSize {
			_ = w.sftpClient.Remove(w.tempPath)
			return fmt.Errorf("upload incomplete: wrote %d of %d bytes", info.Size(), expectedSize)
		}
	}

	if w.replacedPerm != nil {
		if err := w.sftpClient.Chmod(w.tempPath, *w.replacedPerm); err != nil {
			_ = w.sftpClient.Remove(w.tempPath)
			return err
		}
	}

	if err := w.sftpClient.PosixRename(w.tempPath, w.destPath); err != nil {
		_ = w.sftpClient.Remove(w.tempPath)
		return err
	}
	return nil
}

func (w *remoteFileWriter) Abort() {
	_ = w.file.Close()
	_ = w.sftpClient.Remove(w.tempPath)
}

func (a *App) DeletePath(path string) error {
	client := a.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

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
	client := a.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

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
	client := a.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

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

func (a *App) SelectBackupFile() string {
	a.mu.Lock()
	deviceID := a.connectedDeviceID
	a.mu.Unlock()

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
	destPath, err := saveFileDialog(a.ctx, "Save Backup", defaultName, "")
	if err != nil || destPath == "" {
		return ""
	}
	return destPath
}

func (a *App) CreateDeviceBackup(destPath string) {
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
	archivePath, err := openFileDialog(a.ctx, "Select Backup to Restore", "")
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
		client := a.getClient()

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
