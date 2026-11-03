package main

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/crypto/ssh"

	"reManager/internal/commands"
	"reManager/internal/debug"
	"reManager/internal/device"
	apperrors "reManager/internal/errors"
	"reManager/internal/httputil"
	"reManager/internal/logger"
	"reManager/internal/swupdate"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"
	rmexecutor "github.com/rmitchellscott/remarkable-go/executor"
	rmfilesystem "github.com/rmitchellscott/remarkable-go/filesystem"
	"github.com/rmitchellscott/remarkable-go/partition"
	rmversion "github.com/rmitchellscott/remarkable-go/version"
)

const minSwupdateVersion = "3.11.2.5"

type OSService struct {
	*core
	preventSleepStop chan struct{}
	penInputDevice   string
}

const eventFilesystemRestoreError = "filesystem:restore-error"

func init() {
	application.RegisterEvent[FilesystemRestoreError](eventFilesystemRestoreError)
}

// The root filesystem was left writable after a system file was changed.
type FilesystemRestoreError struct {
	Message string `json:"message"`
}

func isSystemPath(remotePath string) bool {
	cleanPath := path.Clean(remotePath)
	return cleanPath != "/home/root" && !strings.HasPrefix(cleanPath, "/home/root/")
}

func (c *core) getConnectedDevice() (rmdevice.Type, rmdevice.Architecture) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectedDeviceType, c.connectedDeviceArch
}

func (c *core) getConnectedDeviceType() rmdevice.Type {
	deviceType, _ := c.getConnectedDevice()
	return deviceType
}

// Counts holders so one finishing transfer does not remount root read-only under another; the count resets per connection, since a holder that outlives a disconnect cannot release it.
func (c *core) makeFilesystemWritable(client *ssh.Client) error {
	if !c.getConnectedDeviceType().IsPaperPro() {
		return nil
	}

	c.writableRootMu.Lock()
	defer c.writableRootMu.Unlock()

	if c.writableRootClient != client {
		c.writableRootClient = client
		c.writableRootCount = 0
	}
	if c.writableRootCount > 0 {
		c.writableRootCount++
		return nil
	}

	if err := c.remountRootWritable(client); err != nil {
		return err
	}

	c.writableRootCount++
	return nil
}

func (c *core) releaseWritableRoot(client *ssh.Client) error {
	c.writableRootMu.Lock()
	defer c.writableRootMu.Unlock()

	if c.writableRootClient != client || c.writableRootCount == 0 {
		return nil
	}

	c.writableRootCount--
	if c.writableRootCount > 0 {
		return nil
	}

	return c.restoreFilesystemWithRetry(client)
}

func (c *core) remountRootWritable(client *ssh.Client) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	cmd := `mount -o remount,rw / 2>/dev/null; if grep -q '^overlay.*/etc' /proc/mounts; then umount -l /etc 2>/dev/null; fi`
	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("failed to make filesystem writable: %w", err)
	}

	return nil
}

func (c *core) restoreFilesystem(client *ssh.Client) error {
	if !c.getConnectedDeviceType().IsPaperPro() {
		return nil
	}

	session1, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	session1.Run(`if [ -d /var/volatile/.etc-work ]; then rm -rf /var/volatile/.etc-work/* 2>/dev/null; mount -t overlay overlay -o rw,relatime,lowerdir=/etc,upperdir=/var/volatile/etc,workdir=/var/volatile/.etc-work /etc 2>/dev/null; fi`)
	session1.Close()

	session2, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	session2.Run("sync && mount -o remount,ro /")
	session2.Close()

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

func (c *core) restoreFilesystemWithRetry(client *ssh.Client) error {
	var lastErr error

	for attempt := 1; attempt <= restoreMaxRetries; attempt++ {
		lastErr = c.restoreFilesystem(client)
		if lastErr == nil {
			return nil
		}

		if attempt < restoreMaxRetries {
			time.Sleep(restoreRetryDelay)
		}
	}

	return lastErr
}

func (c *core) restoreFilesystemDeferred(client *ssh.Client) {
	if err := c.releaseWritableRoot(client); err != nil {
		c.wails.Event.Emit(eventFilesystemRestoreError, FilesystemRestoreError{Message: err.Error()})
	}
}

func (c *core) withWritableRoot(client *ssh.Client, path string, operation func() error) error {
	if !isSystemPath(path) {
		return operation()
	}

	if err := c.makeFilesystemWritable(client); err != nil {
		return err
	}

	opErr := operation()

	restoreErr := c.releaseWritableRoot(client)
	if restoreErr != nil {
		c.wails.Event.Emit(eventFilesystemRestoreError, FilesystemRestoreError{Message: restoreErr.Error()})
	}

	return opErr
}

func (s *OSService) RetryRestoreFilesystem() error {
	client := s.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	return s.restoreFilesystem(client)
}

func (s *OSService) RebootDevice(opID string) {
	s.runOperation(opID, "Rebooting your reMarkable", func(op *Operation) (any, error) {
		if s.inDemoMode() {
			return nil, errDemoUnavailable
		}
		if s.isInstallActive() || s.updateWritingTargetSlot() {
			return nil, apperrors.New(apperrors.ErrUpdateInProgress,
				"An OS update is still being written. Wait for it to finish, because rebooting now can corrupt the partition.", nil, false)
		}
		if s.isBackupActive() {
			return nil, stderrors.New("A backup or restore is running. Wait for it to finish, because rebooting now leaves it incomplete.")
		}
		if s.getClient() == nil {
			return nil, errNotConnected
		}

		if err := s.runOperationCommand(op, "nohup sh -c 'sleep 1 && systemctl reboot' >/dev/null 2>&1 &", false); err != nil {
			return nil, fmt.Errorf("Could not reboot your reMarkable: %w", err)
		}
		return nil, nil
	})
}

// The manager holds an SFTP session, a process on the reMarkable until closed, so every caller must call the returned release.
func (s *OSService) getPartitionManager() (partition.Manager, func(), error) {
	s.mu.Lock()
	client := s.client
	deviceType := s.connectedDeviceType
	s.mu.Unlock()

	if client == nil {
		return nil, nil, fmt.Errorf("not connected")
	}

	exec := rmexecutor.NewSSH(client, rmexecutor.WithLogger(debug.SlogLogger()))
	fs, err := rmfilesystem.NewSSH(client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create filesystem client: %w", err)
	}

	release := func() { fs.Close() }
	return partition.NewManager(exec, fs, rmdevice.Type(deviceType)), release, nil
}

func (s *OSService) deviceExecutor() (rmexecutor.Executor, error) {
	client := s.getClient()
	if client == nil {
		return nil, fmt.Errorf("not connected")
	}
	return rmexecutor.NewSSH(client, rmexecutor.WithLogger(debug.SlogLogger())), nil
}

func (s *OSService) GetPartitionInfo() (*partition.SystemInfo, error) {
	if s.inDemoMode() {
		return demoPartitionInfo(), nil
	}
	mgr, release, err := s.getPartitionManager()
	if err != nil {
		return nil, err
	}
	defer release()
	return mgr.GetSystemInfo(context.Background())
}

const lockDirMissingMessage = "Your reMarkable is missing a system lock it needs to change partitions. Rebooting may resolve this: restart it, then reconnect and try again."

func (s *OSService) lockDirHealthy() bool {
	if s.getConnectedDeviceType().IsPaperPro() {
		return true
	}
	exec, err := s.deviceExecutor()
	if err != nil {
		return true
	}
	result, err := exec.Run(context.Background(), "sh", "-c", `d=$(readlink -f /var/lock 2>/dev/null); [ -d "$d" ] && [ -w "$d" ] && echo ok`)
	if err != nil {
		return true
	}
	return strings.TrimSpace(result.Stdout) == "ok"
}

func (s *OSService) ensureLockDir() bool {
	if s.lockDirHealthy() {
		return true
	}

	exec, err := s.deviceExecutor()
	if err != nil {
		return true
	}

	debug.Printf("[DEBUG] lock dir missing, recreating\n")
	if _, err := exec.Run(context.Background(), "sh", "-c", `d=$(readlink -f /var/lock 2>/dev/null); [ -n "$d" ] || d=/run/lock; mkdir -p "$d" && chmod 1777 "$d"`); err != nil {
		return false
	}

	return s.lockDirHealthy()
}

type OSManagerReadiness struct {
	Ready   bool                `json:"ready"`
	Code    apperrors.ErrorCode `json:"code,omitempty"`
	Message string              `json:"message,omitempty"`
}

func (s *OSService) CheckOSManagerReady() OSManagerReadiness {
	if !s.ensureLockDir() {
		return OSManagerReadiness{Ready: false, Code: apperrors.ErrLockDirMissing, Message: lockDirMissingMessage}
	}
	return OSManagerReadiness{Ready: true}
}

func (s *OSService) updateWritingTargetSlot() bool {
	mgr, release, err := s.getPartitionManager()
	if err != nil {
		return false
	}
	defer release()
	inProgress, err := mgr.IsUpdateInProgress(context.Background())
	if err != nil {
		return false
	}
	return inProgress
}

func (s *OSService) SwitchBootPartition(opID string, partitionNumber int) {
	s.runOperation(opID, "Switching partitions", func(op *Operation) (any, error) {
		if s.inDemoMode() {
			return nil, errDemoUnavailable
		}
		if s.isInstallActive() || s.updateWritingTargetSlot() {
			return nil, apperrors.New(apperrors.ErrUpdateInProgress,
				"An OS update is still being written. Wait for it to finish before switching partitions.", nil, false)
		}

		if !s.ensureLockDir() {
			return nil, apperrors.New(apperrors.ErrLockDirMissing, lockDirMissingMessage, nil, false)
		}

		mgr, release, err := s.getPartitionManager()
		if err != nil {
			return nil, err
		}
		defer release()

		// Not tied to the operation: a switch stopped halfway could leave the boot slots half changed.
		if _, err := mgr.SwitchBoot(context.Background(), partitionNumber); err != nil {
			switch {
			case stderrors.Is(err, partition.ErrSlotUnhealthy):
				return nil, apperrors.New(apperrors.ErrTargetSlotUnhealthy,
					"The target partition failed an integrity check, so the switch was blocked. That slot likely holds a failed or incomplete OS install. Reinstall the OS to it before switching.", err, false)
			case stderrors.Is(err, partition.ErrSlotCheckUnavailable):
				return nil, apperrors.New(apperrors.ErrSlotCheckUnavailable,
					"Could not verify the target partition's health, so the switch was blocked as a precaution.", err, false)
			}
			return nil, err
		}
		return nil, nil
	})
}

func (s *OSService) activeOSVersion() string {
	if info, err := s.GetPartitionInfo(); err == nil && info != nil {
		if v := info.Active.Version; v != "" && v != "unknown" {
			return v
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connectedFirmware
}

func (s *OSService) taskNeedsWritableRoot(task device.SystemTask) bool {
	if !task.NeedsWritableRoot || task.WritableRootBelowOS == "" {
		return task.NeedsWritableRoot
	}
	v := s.activeOSVersion()
	return v == "" || rmversion.Compare(v, task.WritableRootBelowOS) < 0
}

func (s *OSService) isLegacyOS() bool {
	v := s.activeOSVersion()
	return v != "" && rmversion.Compare(v, minSwupdateVersion) < 0
}

func (s *OSService) GetOSInstallMode() string {
	if s.isLegacyOS() {
		return "legacy"
	}
	return "swupdate"
}

func (s *OSService) GetAvailableOSVersions() ([]swupdate.OSVersionInfo, error) {
	s.mu.Lock()
	deviceType := s.connectedDeviceType
	s.mu.Unlock()

	if deviceType == "" {
		return nil, fmt.Errorf("not connected")
	}

	if s.isLegacyOS() {
		versions, err := swupdate.ListLegacyVersions(string(deviceType))
		if err != nil {
			return nil, err
		}
		filtered := make([]swupdate.OSVersionInfo, 0, len(versions))
		for _, v := range versions {
			if rmversion.Compare(v.Version, minSwupdateVersion) <= 0 {
				filtered = append(filtered, v)
			}
		}
		return filtered, nil
	}

	return swupdate.ListVersions(string(deviceType), nil)
}

// Versions only: whether the tablet is ahead of or behind the public release is decided in the frontend, as the other connect facts are.
type OSChannelStatus struct {
	ActiveVersion string `json:"activeVersion"`
	LatestPublic  string `json:"latestPublic"`
}

func (s *OSService) osChannelStatus() OSChannelStatus {
	s.mu.Lock()
	deviceType := s.connectedDeviceType
	s.mu.Unlock()

	active := s.activeOSVersion()
	status := OSChannelStatus{ActiveVersion: active}

	if deviceType == "" || active == "" || s.isLegacyOS() {
		return status
	}

	versions, err := swupdate.ListVersions(string(deviceType), nil)
	if err != nil || len(versions) == 0 {
		return status
	}

	status.LatestPublic = versions[0].Version
	return status
}

// Answered from the desktop's own metadata rather than `vellum check-os`, so it holds with the tablet offline and costs no round trip.
func (s *OSService) incompatibleWithOS(targetOS string, installed []string) []string {
	if targetOS == "" || s.metadata == nil {
		return nil
	}

	s.mu.Lock()
	deviceType := string(s.connectedDeviceType)
	arch := commands.GetDownloadArch(s.connectedDeviceArch)
	s.mu.Unlock()

	blocked := []string{}
	for _, name := range installed {
		if isHiddenPackage(name) {
			continue
		}
		// A package the catalog has never heard of cannot be judged, and reporting it as blocked would be a guess.
		if s.metadata.GetPackage(name) == nil {
			continue
		}
		if s.metadata.GetPackageForTargetOS(name, targetOS, deviceType, arch) == nil {
			blocked = append(blocked, name)
		}
	}
	sort.Strings(blocked)
	return blocked
}

type OSInstallStage string

const (
	OSInstallDownloading OSInstallStage = "downloading"
	OSInstallUploading   OSInstallStage = "uploading"
	OSInstallInstalling  OSInstallStage = "installing"
	OSInstallFinalizing  OSInstallStage = "finalizing"
	OSInstallComplete    OSInstallStage = "complete"
)

type OSInstallProgress struct {
	Stage   OSInstallStage `json:"stage"`
	Percent float64        `json:"percent"`
	Message string         `json:"message"`
}

func (s *OSService) InstallOSVersion(opID, version string) {
	s.runOperation(opID, "Installing OS "+version, func(op *Operation) (any, error) {
		if s.inDemoMode() {
			return nil, errDemoUnavailable
		}
		if !s.beginInstall() {
			return nil, stderrors.New("An OS update is already in progress.")
		}
		defer s.endInstall()
		return nil, s.installOSVersion(op, version)
	})
}

func (s *OSService) installOSVersion(op *Operation, version string) error {
	s.mu.Lock()
	client := s.client
	deviceType := s.connectedDeviceType
	deviceID := s.connectedDeviceID
	s.mu.Unlock()

	if client == nil {
		return errNotConnected
	}

	if s.updateWritingTargetSlot() {
		return stderrors.New("An OS update is already in progress on your reMarkable. Wait for it to finish.")
	}

	if !s.ensureLockDir() {
		return stderrors.New(lockDirMissingMessage)
	}

	if info, infoErr := s.GetPartitionInfo(); infoErr == nil && info != nil {
		active := info.Active.Version
		if active != "" && active != "unknown" && rmversion.Compare(active, minSwupdateVersion) < 0 {
			return s.installLegacyOSVersion(op, version)
		}
	}

	versions, err := swupdate.ListVersions(string(deviceType), nil)
	if err != nil {
		return err
	}

	var filename string
	for _, v := range versions {
		if v.Version == version {
			filename = v.Filename
			break
		}
	}
	if filename == "" {
		return stderrors.New("Version not found.")
	}

	url := swupdate.ImageURL(filename)

	op.OSInstallProgress(OSInstallDownloading, 0.0, "Downloading…")

	tmpFile, err := os.CreateTemp("", "remanager-swu-*.swu")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	resp, err := httputil.GetStreaming(op.ctx, url)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		tmpFile.Close()
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	totalSize := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		if op.Canceled() {
			tmpFile.Close()
			return errOperationCanceled
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := tmpFile.Write(buf[:n]); writeErr != nil {
				tmpFile.Close()
				return fmt.Errorf("write error: %w", writeErr)
			}
			downloaded += int64(n)
			if totalSize > 0 {
				pct := float64(downloaded) / float64(totalSize) * 50.0
				op.OSInstallProgress(OSInstallDownloading, pct, "Downloading…")
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tmpFile.Close()
			return fmt.Errorf("download error: %w", readErr)
		}
	}
	tmpFile.Close()

	op.OSInstallProgress(OSInstallUploading, 50.0, "Uploading to your reMarkable…")

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("SFTP error: %w", err)
	}
	defer sftpClient.Close()

	remotePath := "/tmp/" + filename
	localFile, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to open temp file: %w", err)
	}
	defer localFile.Close()

	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %w", err)
	}
	// The tablet's /tmp is RAM, so an image left behind by a failed or canceled install holds hundreds of MB until the next reboot.
	defer s.removeRemoteFile(remotePath)

	stat, _ := localFile.Stat()
	uploadSize := stat.Size()
	var uploaded int64
	for {
		if op.Canceled() {
			remoteFile.Close()
			return errOperationCanceled
		}
		n, readErr := localFile.Read(buf)
		if n > 0 {
			if _, writeErr := remoteFile.Write(buf[:n]); writeErr != nil {
				remoteFile.Close()
				return fmt.Errorf("upload error: %w", writeErr)
			}
			uploaded += int64(n)
			if uploadSize > 0 {
				pct := 50.0 + float64(uploaded)/float64(uploadSize)*50.0
				op.OSInstallProgress(OSInstallUploading, pct, "Uploading to your reMarkable…")
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			remoteFile.Close()
			return fmt.Errorf("upload read error: %w", readErr)
		}
	}
	remoteFile.Close()

	op.OSInstallProgress(OSInstallInstalling, 100.0, "Installing…")

	var outputBuf bytes.Buffer
	var outputWriter io.Writer = &outputBuf
	var cmdLog *logger.CommandLog
	if s.logger != nil {
		cmdLog = s.logger.StartCommandLog(deviceID, "swupdate-install")
		defer func() {
			cmdLog.Close()
		}()
		outputWriter = io.MultiWriter(&outputBuf, &cmdLogWriter{cmdLog: cmdLog})
	}

	if cmdLog != nil {
		cmdLog.Write(fmt.Sprintf("[install] mode=swupdate target_version=%s image=%s\n", version, filename))
		if info, e := s.GetPartitionInfo(); e == nil && info != nil {
			cmdLog.Write(fmt.Sprintf("[install] active_slot=%d target_slot=%d active_version=%s\n", info.Active.Number, info.Fallback.Number, info.Active.Version))
		}
	}
	if s.logger != nil {
		s.logger.LogInstall("swupdate", version, "start")
	}

	if err := s.runSwupdateDetached(s.ctx, remotePath, outputWriter, op.Canceled); err != nil {
		if op.Canceled() {
			return errOperationCanceled
		}
		if s.logger != nil {
			s.logger.LogInstall("swupdate", version, "failed: "+err.Error())
		}
		op.Output(outputBuf.String())
		return fmt.Errorf("install failed: %w", err)
	}
	if s.logger != nil {
		s.logger.LogInstall("swupdate", version, "success: standby slot written")
	}

	op.OSInstallProgress(OSInstallFinalizing, 100.0, "Finalizing…")
	op.OSInstallProgress(OSInstallComplete, 100.0, "Complete")
	return nil
}

// Uses the current client rather than the one the install started on, which a reconnect during the install may have replaced.
func (s *OSService) removeRemoteFile(remotePath string) {
	client := s.getClient()
	if client == nil {
		return
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		debug.Printf("[DEBUG] removeRemoteFile %s: %v\n", remotePath, err)
		return
	}
	defer sftpClient.Close()
	if err := sftpClient.Remove(remotePath); err != nil && !os.IsNotExist(err) {
		debug.Printf("[DEBUG] removeRemoteFile %s: %v\n", remotePath, err)
	}
}

func (s *OSService) findPenInputDevice() (string, error) {
	debug.Println("[DEBUG] findPenInputDevice: searching for pen input device via BTN_STYLUS capability")
	cmd := `for ev in /sys/class/input/event*; do
		caps=$(cat "$ev/device/capabilities/key" 2>/dev/null)
		[ -z "$caps" ] && continue
		count=$(echo "$caps" | wc -w)
		[ "$count" -lt 6 ] && continue
		first=$(echo "$caps" | awk '{print $1}')
		if [ $((0x$first & 0x800)) -ne 0 ]; then
			echo "/dev/input/$(basename $ev)"
			exit 0
		fi
	done`
	output, err := s.runCommand(cmd)
	if err != nil {
		debug.Printf("[DEBUG] findPenInputDevice: runCommand failed: %v\n", err)
		return "", err
	}
	device := strings.TrimSpace(output)
	debug.Printf("[DEBUG] findPenInputDevice: raw output=%q, device=%q\n", output, device)
	if device == "" {
		return "", fmt.Errorf("pen input device not found (no device with BTN_STYLUS capability)")
	}
	debug.Printf("[DEBUG] findPenInputDevice: found device: %s\n", device)
	return device, nil
}

const penPokeTimeout = 5 * time.Second

func (s *OSService) pokePenDevice(devicePath string, enter bool) error {
	debug.Printf("[DEBUG] pokePenDevice: device=%s enter=%v\n", devicePath, enter)

	var val, dist string
	if enter {
		val = `\001`
		dist = `\062`
	} else {
		val = `\000`
		dist = `\000`
	}

	cmd := fmt.Sprintf(`arch=$(uname -m)
if [ "$arch" = "aarch64" ]; then
  printf '\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\001\000\100\001%s\000\000\000' > %s
  printf '\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\003\000\031\000%s\000\000\000' > %s
  printf '\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000' > %s
else
  printf '\000\000\000\000\000\000\000\000\003\000\000\000\062\000\000\000' > %s
  printf '\000\000\000\000\000\000\000\000\003\000\001\000\062\000\000\000' > %s
  printf '\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000' > %s
  printf '\000\000\000\000\000\000\000\000\003\000\000\000\144\000\000\000' > %s
  printf '\000\000\000\000\000\000\000\000\003\000\001\000\144\000\000\000' > %s
  printf '\000\000\000\000\000\000\000\000\001\000\100\001%s\000\000\000' > %s
  printf '\000\000\000\000\000\000\000\000\003\000\031\000%s\000\000\000' > %s
  printf '\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000\000' > %s
fi`,
		val, devicePath,
		dist, devicePath,
		devicePath,
		devicePath,
		devicePath,
		devicePath,
		devicePath,
		devicePath,
		val, devicePath,
		dist, devicePath,
		devicePath)

	_, err := s.runCommandWithTimeout(cmd, penPokeTimeout)
	if err != nil {
		debug.Printf("[DEBUG] pokePenDevice: command failed: %v\n", err)
		return err
	}
	debug.Println("[DEBUG] pokePenDevice: successfully wrote events")
	return nil
}

func (s *OSService) startPreventSleep() error {
	debug.Println("[DEBUG] startPreventSleep: called")
	// The slot is taken before the device lookup's round trip, so a second start that arrives meanwhile sees it and does not pulse alongside this one.
	s.mu.Lock()
	if s.preventSleepStop != nil {
		s.mu.Unlock()
		debug.Println("[DEBUG] startPreventSleep: already running, skipping")
		return nil
	}
	stopCh := make(chan struct{})
	s.preventSleepStop = stopCh
	s.mu.Unlock()

	penDevice, err := s.findPenInputDevice()
	if err != nil {
		debug.Printf("[DEBUG] startPreventSleep: findPenInputDevice failed: %v\n", err)
		s.mu.Lock()
		if s.preventSleepStop == stopCh {
			s.preventSleepStop = nil
		}
		s.mu.Unlock()
		return fmt.Errorf("failed to find pen input device: %w", err)
	}

	s.mu.Lock()
	if s.preventSleepStop != stopCh {
		s.mu.Unlock()
		debug.Println("[DEBUG] startPreventSleep: stopped during the device lookup")
		return nil
	}
	s.penInputDevice = penDevice
	s.mu.Unlock()

	debug.Printf("[DEBUG] startPreventSleep: starting goroutine for device %s\n", penDevice)
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				debug.Println("[DEBUG] startPreventSleep goroutine: pulsing pen proximity")
				if err := s.pokePenDevice(penDevice, true); err != nil {
					debug.Printf("[DEBUG] startPreventSleep goroutine: pen-enter failed: %v\n", err)
				}
				time.Sleep(50 * time.Millisecond)
				s.pokePenDevice(penDevice, false)
			case <-stopCh:
				debug.Println("[DEBUG] startPreventSleep goroutine: stopped")
				return
			}
		}
	}()

	debug.Println("[DEBUG] startPreventSleep: completed successfully")
	return nil
}

func (s *OSService) stopPreventSleep() error {
	debug.Println("[DEBUG] stopPreventSleep: called")
	s.mu.Lock()
	penDevice := s.penInputDevice
	if s.preventSleepStop != nil {
		debug.Println("[DEBUG] stopPreventSleep: closing stop channel")
		close(s.preventSleepStop)
		s.preventSleepStop = nil
	} else {
		debug.Println("[DEBUG] stopPreventSleep: not running, nothing to stop")
	}
	s.penInputDevice = ""
	s.mu.Unlock()
	if penDevice != "" {
		s.pokePenDevice(penDevice, false)
	}
	return nil
}
