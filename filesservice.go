package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"

	"reManager/internal/backup"
	apperrors "reManager/internal/errors"
	"reManager/internal/executor"
	"reManager/internal/logger"
	"reManager/internal/storage"
	"reManager/internal/vellum"
)

type FilesService struct {
	*core
	config *ConfigService
	device *DeviceService

	transfers    *transferManager
	transferOnce sync.Once
	roundTrip    time.Duration
	roundTripGen uint64
	roundTripMu  sync.Mutex

	guideTargetRevision atomic.Int64

	demoHasGuide  bool
	demoGuideSeen bool
}

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

func (s *FilesService) ListDirectory(dirPath string) ([]FileInfo, error) {
	if dirPath == "" {
		dirPath = "/home/root"
	}
	if listing, ok := s.screenshotListing(dirPath); ok {
		return listing, nil
	}

	sftpClient, err := s.getSFTPClient()
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

func (s *FilesService) countRemoteFolder(sftpClient *sftp.Client, remotePath string) (int, int64, error) {
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

func (s *FilesService) SelectFilesForUpload() []string {
	var lastDir string
	if s.settingsStore != nil {
		if settings, err := s.settingsStore.Load(); err == nil {
			lastDir = settings.LastUploadDir
		}
	}

	files, err := openMultipleFilesDialog("Select files to upload", lastDir)
	if err != nil || len(files) == 0 {
		return []string{}
	}

	if s.settingsStore != nil {
		_ = s.settingsStore.Update(func(settings *storage.Settings) {
			settings.LastUploadDir = filepath.Dir(files[0])
		})
	}

	return files
}

func (s *FilesService) countLocalFolder(localPath string) (int, int64, error) {
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

func isProtectedFromDelete(remotePath string) bool {
	cleanPath := path.Clean("/" + remotePath)
	return path.Dir(cleanPath) == "/" || cleanPath == "/home/root"
}

func (s *FilesService) DeletePath(path string) error {
	if s.inDemoMode() {
		return errDemoUnavailable
	}
	client := s.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	if isProtectedFromDelete(path) {
		return fmt.Errorf("%s cannot be deleted", path)
	}

	if isSystemPath(path) {
		if err := s.makeFilesystemWritable(client); err != nil {
			return err
		}
		defer s.restoreFilesystemDeferred(client)
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
			_, err = session.CombinedOutput("rm -rf -- " + executor.QuoteArg(path))
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

func (s *FilesService) RenamePath(oldPath, newPath string) error {
	if s.inDemoMode() {
		return errDemoUnavailable
	}
	client := s.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	if isSystemPath(oldPath) || isSystemPath(newPath) {
		if err := s.makeFilesystemWritable(client); err != nil {
			return err
		}
		defer s.restoreFilesystemDeferred(client)
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

func (s *FilesService) CreateDirectory(path string) error {
	if s.inDemoMode() {
		return errDemoUnavailable
	}
	client := s.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	if isSystemPath(path) {
		if err := s.makeFilesystemWritable(client); err != nil {
			return err
		}
		defer s.restoreFilesystemDeferred(client)
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

func (s *FilesService) SelectBackupFile() string {
	s.mu.Lock()
	deviceID := s.connectedDeviceID
	s.mu.Unlock()

	rawDeviceName := ""
	if savedDevice, err := s.deviceStore.Get(deviceID); err == nil && savedDevice.Name != "" {
		rawDeviceName = savedDevice.Name
	}
	deviceName := "device"
	if rawDeviceName != "" {
		deviceName = sanitizeFilename(rawDeviceName)
	}

	timestamp := time.Now().Format("2006-01-02-150405")
	defaultName := fmt.Sprintf("remarkable-backup-%s-%s.tar.zst", deviceName, timestamp)
	if s.inScreenshotDemo() {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Downloads", defaultName)
	}
	destPath, err := saveFileDialog("Save backup", defaultName, "")
	if err != nil || destPath == "" {
		return ""
	}
	return destPath
}

var errBackupBusy = errors.New("A backup or restore is already running. Wait for it to finish, then try again.")

// Backup and restore share one slot: both stream through the same device directories.
func (c *core) claimBackupSlot() bool {
	c.backupMu.Lock()
	defer c.backupMu.Unlock()
	if c.backupActive {
		return false
	}
	c.backupActive = true
	return true
}

func (c *core) isBackupActive() bool {
	c.backupMu.Lock()
	defer c.backupMu.Unlock()
	return c.backupActive
}

func (c *core) releaseBackupSlot() {
	c.backupMu.Lock()
	defer c.backupMu.Unlock()
	c.backupActive = false
}

func (s *FilesService) logBackupEvent(category, message string) {
	if s.logger != nil {
		s.logger.LogEvent(category, message)
	}
}

func (s *FilesService) CreateDeviceBackup(opID, destPath string) {
	s.runOperation(opID, "Backing up your reMarkable", func(op *Operation) (any, error) {
		if s.inScreenshotDemo() {
			return s.screenshotBackup(op, destPath)
		}
		if s.inDemoMode() {
			return nil, errDemoUnavailable
		}
		if !s.claimBackupSlot() {
			s.logBackupEvent("BACKUP", "refused: another backup or restore is running")
			return nil, errBackupBusy
		}
		defer s.releaseBackupSlot()

		s.mu.Lock()
		client := s.client
		deviceID := s.connectedDeviceID
		machineID := s.connectedMachineID
		deviceName := s.connectedDeviceType.DisplayName()
		s.mu.Unlock()

		if client == nil {
			return nil, errNotConnected
		}

		// A reMarkable reached by address rather than from the saved list has no saved ID or name, so the machine ID and the model identify it instead.
		if deviceID != "" {
			if savedDevice, err := s.deviceStore.Get(deviceID); err == nil && savedDevice.Name != "" {
				deviceName = savedDevice.Name
			}
		}

		var lastProgress backup.Progress
		manager := backup.Manager{
			SSHClient: client,
			CancelCh:  op.ctx.Done(),
			ProgressFn: func(p backup.Progress) {
				lastProgress = p
				op.BackupProgress(p)
			},
			DeviceName: deviceName,
			DeviceID:   deviceID,
			MachineID:  machineID,
		}

		sources := []string{
			"/home/root/.local/share/remarkable/xochitl",
			"/home/root/.config/remarkable",
		}

		s.logBackupEvent("BACKUP", "started: "+filepath.Base(destPath))
		started := time.Now()
		err := manager.CreateBackup(destPath, sources)
		summary := fmt.Sprintf("%d/%d files, %d bytes, %s", lastProgress.FilesDone, lastProgress.FilesTotal, lastProgress.BytesDone, time.Since(started).Round(time.Second))

		if err != nil {
			s.logBackupEvent("BACKUP", fmt.Sprintf("failed after %s: %v", summary, err))
			return nil, err
		}
		s.logBackupEvent("BACKUP", "complete: "+summary)
		shareFile(destPath)
		return destPath, nil
	})
}

// The backup's identity is compared with the connected reMarkable's, so the restore can warn before it overwrites one tablet with another's documents.
type RestoreFile struct {
	Path              string `json:"path"`
	FromAnotherDevice bool   `json:"fromAnotherDevice"`
	BackupDevice      string `json:"backupDevice"`
	CurrentDevice     string `json:"currentDevice"`
}

func (s *FilesService) SelectRestoreFile() RestoreFile {
	archivePath, err := openFileDialog("Select backup to restore", "")
	if err != nil || archivePath == "" {
		return RestoreFile{}
	}
	selection := RestoreFile{Path: archivePath}

	metadata, err := backup.ReadBackupMetadata(archivePath)
	if err == nil && metadata != nil {
		s.mu.Lock()
		currentDeviceID := s.connectedDeviceID
		currentMachineID := s.connectedMachineID
		currentDeviceName := s.connectedDeviceType.DisplayName()
		s.mu.Unlock()

		if metadata.MachineID != "" && currentMachineID != "" {
			selection.FromAnotherDevice = metadata.MachineID != currentMachineID
		} else {
			selection.FromAnotherDevice = metadata.DeviceID != "" && currentDeviceID != "" && metadata.DeviceID != currentDeviceID
		}

		if selection.FromAnotherDevice {
			if savedDevice, err := s.deviceStore.Get(currentDeviceID); err == nil && savedDevice.Name != "" {
				currentDeviceName = savedDevice.Name
			}
			selection.BackupDevice = metadata.DeviceName
			selection.CurrentDevice = currentDeviceName
		}
	}

	return selection
}

func restoreRefusal(op *Operation, message, detail string) error {
	op.Output(detail + "\n")
	return apperrors.New(apperrors.ErrUnknown, message, nil, false)
}

func (s *FilesService) RestoreDeviceBackup(opID, archivePath string) {
	s.runOperation(opID, "Restoring your reMarkable", func(op *Operation) (any, error) {
		if s.inDemoMode() {
			return nil, errDemoUnavailable
		}
		if !s.claimBackupSlot() {
			s.logBackupEvent("RESTORE", "refused: another backup or restore is running")
			return nil, errBackupBusy
		}
		defer s.releaseBackupSlot()

		client := s.getClient()
		if client == nil {
			return nil, errNotConnected
		}

		var lastProgress backup.Progress
		manager := backup.Manager{
			SSHClient: client,
			CancelCh:  op.ctx.Done(),
			ProgressFn: func(p backup.Progress) {
				lastProgress = p
				op.BackupProgress(p)
			},
		}

		uiStatus, err := s.device.GetXochitlStatus()
		if err != nil {
			s.logBackupEvent("RESTORE", "refused: could not read UI state: "+err.Error())
			return nil, restoreRefusal(op, "Could not check whether the reMarkable UI is running, so nothing was restored.", err.Error())
		}
		ui := restoreUIControlFor(uiStatus)
		s.logBackupEvent("RESTORE", fmt.Sprintf("started: %s (%s, running=%t)", filepath.Base(archivePath), ui.name, ui.running))
		if ui.running {
			op.BackupProgress(backup.Progress{
				Phase:   backup.PhaseStoppingUI,
				Message: fmt.Sprintf("Stopping %s…", ui.name),
			})
			if output, stopErr := s.runCommand(ui.stopCommand); stopErr != nil {
				s.runCommand(ui.startCommand)
				s.logBackupEvent("RESTORE", fmt.Sprintf("refused: could not stop %s: %v", ui.name, stopErr))
				return nil, restoreRefusal(op, fmt.Sprintf("Could not stop %s, so nothing was restored.", ui.name), strings.TrimSpace(output+"\n"+stopErr.Error()))
			}
		}

		started := time.Now()
		err = manager.RestoreBackup(archivePath)
		summary := fmt.Sprintf("%d/%d files, %d bytes, %s", lastProgress.FilesDone, lastProgress.FilesTotal, lastProgress.BytesDone, time.Since(started).Round(time.Second))
		if err != nil {
			s.logBackupEvent("RESTORE", fmt.Sprintf("failed after %s: %v", summary, err))
		} else {
			s.logBackupEvent("RESTORE", "extracted: "+summary)
		}

		startingProgress := lastProgress
		startingProgress.Phase = backup.PhaseStartingUI
		startingProgress.Message = fmt.Sprintf("Starting %s…", ui.name)
		op.BackupProgress(startingProgress)
		output, startErr := s.runCommand(ui.startCommand)
		if startErr != nil {
			s.logBackupEvent("RESTORE", fmt.Sprintf("could not start %s: %v", ui.name, startErr))
		} else {
			s.logBackupEvent("RESTORE", "started "+ui.name)
		}
		if startErr != nil && err == nil {
			return nil, restoreRefusal(op, fmt.Sprintf("The backup was restored, but %s did not start. Reboot your reMarkable to start it.", ui.name), strings.TrimSpace(output+"\n"+startErr.Error()))
		}
		return nil, err
	})
}

// A running xochitl writes its in-memory state back over restored files on exit; Oxide runs xochitl as its own child rather than xochitl.service, so a launcher is stopped through launcherctl, and the UI is always restarted so a restore never leaves the tablet without one.
type restoreUIControl struct {
	name         string
	running      bool
	stopCommand  string
	startCommand string
}

func restoreUIControlFor(status XochitlStatus) restoreUIControl {
	if vellum.LauncherSelected(status.ActiveLauncher) {
		return restoreUIControl{
			name:         status.ActiveLauncher,
			running:      true,
			stopCommand:  vellum.LauncherctlBin + " stop-launcher",
			startCommand: executor.WithXochitlStartReset(vellum.LauncherctlBin + " start-launcher"),
		}
	}
	start := "systemctl start xochitl"
	if status.XoviActive {
		start = "/home/root/xovi/start"
	}
	return restoreUIControl{
		name:         "the reMarkable UI",
		running:      status.Running,
		stopCommand:  "systemctl stop xochitl",
		startCommand: executor.WithXochitlStartReset(start),
	}
}

func (s *FilesService) RevealInFileManager(path string) {
	revealInFileManager(path)
}
