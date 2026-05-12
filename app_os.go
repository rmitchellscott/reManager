package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"reManager/internal/debug"
	"reManager/internal/httputil"
	"reManager/internal/logger"
	"reManager/internal/swupdate"

	rmexecutor "github.com/rmitchellscott/remarkable-go/executor"
	rmfilesystem "github.com/rmitchellscott/remarkable-go/filesystem"
	rmdevice "github.com/rmitchellscott/remarkable-go/device"
	"github.com/rmitchellscott/remarkable-go/partition"
	rmupdate "github.com/rmitchellscott/remarkable-go/update"
)

func isSystemPath(path string) bool {
	cleanPath := filepath.Clean(path)
	return !strings.HasPrefix(cleanPath, "/home/root")
}

func (a *App) makeFilesystemWritable(client *ssh.Client) error {
	if !a.connectedDeviceType.IsPaperPro() {
		return nil
	}

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

func (a *App) restoreFilesystem(client *ssh.Client) error {
	if !a.connectedDeviceType.IsPaperPro() {
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

func (a *App) restoreFilesystemDeferred(client *ssh.Client) {
	if err := a.restoreFilesystemWithRetry(client); err != nil {
		runtime.EventsEmit(a.ctx, "filesystem:restore-error", map[string]interface{}{
			"message": err.Error(),
		})
	}
}

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

func (a *App) RetryRestoreFilesystem() error {
	client := a.getClient()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	return a.restoreFilesystem(client)
}

func (a *App) RebootDevice() error {
	client := a.getClient()

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

func (a *App) getPartitionManager() (partition.Manager, error) {
	a.mu.Lock()
	client := a.client
	deviceType := a.connectedDeviceType
	a.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}

	exec := rmexecutor.NewSSH(client)
	fs, err := rmfilesystem.NewSSH(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create filesystem client: %w", err)
	}

	return partition.NewManager(exec, fs, rmdevice.Type(deviceType)), nil
}

func (a *App) GetPartitionInfo() (*partition.SystemInfo, error) {
	mgr, err := a.getPartitionManager()
	if err != nil {
		return nil, err
	}
	return mgr.GetSystemInfo(context.Background())
}

func (a *App) SwitchBootPartition(partitionNumber int) (*partition.SwitchResult, error) {
	mgr, err := a.getPartitionManager()
	if err != nil {
		return nil, err
	}
	return mgr.SwitchBoot(context.Background(), partitionNumber)
}

func (a *App) GetAvailableOSVersions() ([]swupdate.OSVersionInfo, error) {
	a.mu.Lock()
	deviceType := a.connectedDeviceType
	a.mu.Unlock()

	if deviceType == "" {
		return nil, fmt.Errorf("not connected")
	}

	return swupdate.ListVersions(string(deviceType), nil)
}

func (a *App) CancelOSInstall() {
	a.mu.Lock()
	ch := a.osInstallCancelCh
	a.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

func (a *App) InstallOSVersion(version string) {
	go func() {
		a.mu.Lock()
		client := a.client
		deviceType := a.connectedDeviceType
		deviceID := a.connectedDeviceID
		cancelCh := make(chan struct{})
		a.osInstallCancelCh = cancelCh
		a.mu.Unlock()

		if client == nil {
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "not connected"})
			return
		}

		versions, err := swupdate.ListVersions(string(deviceType), nil)
		if err != nil {
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": err.Error()})
			return
		}

		var filename string
		for _, v := range versions {
			if v.Version == version {
				filename = v.Filename
				break
			}
		}
		if filename == "" {
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "version not found"})
			return
		}

		url := "https://remarkable-software.s3.us-east-2.amazonaws.com/" + filename

		runtime.EventsEmit(a.ctx, "software:install-progress", map[string]interface{}{"phase": "downloading", "percent": 0.0, "message": "Downloading..."})

		tmpFile, err := os.CreateTemp("", "remanager-swu-*.swu")
		if err != nil {
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "failed to create temp file: " + err.Error()})
			return
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		resp, err := httputil.NewClient(10 * time.Minute).Get(url)
		if err != nil {
			tmpFile.Close()
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "download failed: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			tmpFile.Close()
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": fmt.Sprintf("download failed: HTTP %d", resp.StatusCode)})
			return
		}

		cancelled := func() bool {
			select {
			case <-cancelCh:
				return true
			default:
				return false
			}
		}

		totalSize := resp.ContentLength
		var downloaded int64
		buf := make([]byte, 32*1024)
		for {
			if cancelled() {
				tmpFile.Close()
				runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "cancelled"})
				return
			}
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				if _, writeErr := tmpFile.Write(buf[:n]); writeErr != nil {
					tmpFile.Close()
					runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "write error: " + writeErr.Error()})
					return
				}
				downloaded += int64(n)
				if totalSize > 0 {
					pct := float64(downloaded) / float64(totalSize) * 50.0
					runtime.EventsEmit(a.ctx, "software:install-progress", map[string]interface{}{"phase": "downloading", "percent": pct, "message": "Downloading..."})
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				tmpFile.Close()
				runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "download error: " + readErr.Error()})
				return
			}
		}
		tmpFile.Close()

		runtime.EventsEmit(a.ctx, "software:install-progress", map[string]interface{}{"phase": "uploading", "percent": 50.0, "message": "Uploading to device..."})

		sftpClient, err := sftp.NewClient(client)
		if err != nil {
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "SFTP error: " + err.Error()})
			return
		}
		defer sftpClient.Close()

		remotePath := "/tmp/" + filename
		localFile, err := os.Open(tmpPath)
		if err != nil {
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "failed to open temp file: " + err.Error()})
			return
		}
		defer localFile.Close()

		remoteFile, err := sftpClient.Create(remotePath)
		if err != nil {
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "failed to create remote file: " + err.Error()})
			return
		}

		stat, _ := localFile.Stat()
		uploadSize := stat.Size()
		var uploaded int64
		for {
			if cancelled() {
				remoteFile.Close()
				runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "cancelled"})
				return
			}
			n, readErr := localFile.Read(buf)
			if n > 0 {
				if _, writeErr := remoteFile.Write(buf[:n]); writeErr != nil {
					remoteFile.Close()
					runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "upload error: " + writeErr.Error()})
					return
				}
				uploaded += int64(n)
				if uploadSize > 0 {
					pct := 50.0 + float64(uploaded)/float64(uploadSize)*50.0
					runtime.EventsEmit(a.ctx, "software:install-progress", map[string]interface{}{"phase": "uploading", "percent": pct, "message": "Uploading to device..."})
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				remoteFile.Close()
				runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": "upload read error: " + readErr.Error()})
				return
			}
		}
		remoteFile.Close()

		runtime.EventsEmit(a.ctx, "software:install-progress", map[string]interface{}{"phase": "installing", "percent": 100.0, "message": "Installing..."})

		exec := rmexecutor.NewSSH(client)
		updater := rmupdate.NewManager(exec)

		var outputBuf bytes.Buffer
		var outputWriter io.Writer = &outputBuf
		var cmdLog *logger.CommandLog
		if a.logger != nil {
			cmdLog = a.logger.StartCommandLog(deviceID, "swupdate-install")
			defer func() {
				cmdLog.Close()
			}()
			outputWriter = io.MultiWriter(&outputBuf, &cmdLogWriter{cmdLog: cmdLog})
		}

		result, err := updater.Install(context.Background(), remotePath, outputWriter)
		if err != nil {
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]interface{}{"error": "install failed: " + err.Error(), "output": outputBuf.String()})
			return
		}
		if !result.Success {
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]interface{}{"error": "install failed: " + result.Message, "output": outputBuf.String()})
			return
		}

		runtime.EventsEmit(a.ctx, "software:install-progress", map[string]interface{}{"phase": "finalizing", "percent": 100.0, "message": "Finalizing..."})

		session, err := client.NewSession()
		if err == nil {
			session.Run("rm -f " + remotePath)
			session.Close()
		}

		runtime.EventsEmit(a.ctx, "software:install-progress", map[string]interface{}{"phase": "complete", "percent": 100.0, "message": "Complete"})
		runtime.EventsEmit(a.ctx, "software:install-complete", map[string]interface{}{"version": version, "partition": "standby"})
	}()
}

var (
	btnTouchDown = []byte{
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0x01, 0x00,
		0x4a, 0x01,
		0x01, 0x00, 0x00, 0x00,
	}
	btnTouchUp = []byte{
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0x01, 0x00,
		0x4a, 0x01,
		0x00, 0x00, 0x00, 0x00,
	}
	synReport = []byte{
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
)

func (a *App) findPenInputDevice() (string, error) {
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
	output, err := a.runCommand(cmd)
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

func (a *App) pokePenDevice(devicePath string, enter bool) error {
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

	_, err := a.runCommand(cmd)
	if err != nil {
		debug.Printf("[DEBUG] pokePenDevice: command failed: %v\n", err)
		return err
	}
	debug.Println("[DEBUG] pokePenDevice: successfully wrote events")
	return nil
}

func (a *App) StartPreventSleep() error {
	debug.Println("[DEBUG] StartPreventSleep: called")
	a.mu.Lock()
	if a.preventSleepStop != nil {
		a.mu.Unlock()
		debug.Println("[DEBUG] StartPreventSleep: already running, skipping")
		return nil
	}
	a.mu.Unlock()

	penDevice, err := a.findPenInputDevice()
	if err != nil {
		debug.Printf("[DEBUG] StartPreventSleep: findPenInputDevice failed: %v\n", err)
		return fmt.Errorf("failed to find pen input device: %w", err)
	}

	a.mu.Lock()
	a.penInputDevice = penDevice
	a.preventSleepStop = make(chan struct{})
	stopCh := a.preventSleepStop
	a.mu.Unlock()

	debug.Printf("[DEBUG] StartPreventSleep: starting goroutine for device %s\n", penDevice)
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				debug.Println("[DEBUG] StartPreventSleep goroutine: pulsing pen proximity")
				if err := a.pokePenDevice(penDevice, true); err != nil {
					debug.Printf("[DEBUG] StartPreventSleep goroutine: pen-enter failed: %v\n", err)
				}
				time.Sleep(50 * time.Millisecond)
				a.pokePenDevice(penDevice, false)
			case <-stopCh:
				debug.Println("[DEBUG] StartPreventSleep goroutine: stopped")
				return
			}
		}
	}()

	runtime.EventsEmit(a.ctx, "prevent-sleep-changed", true)
	debug.Println("[DEBUG] StartPreventSleep: completed successfully")
	return nil
}

func (a *App) StopPreventSleep() error {
	debug.Println("[DEBUG] StopPreventSleep: called")
	a.mu.Lock()
	penDevice := a.penInputDevice
	if a.preventSleepStop != nil {
		debug.Println("[DEBUG] StopPreventSleep: closing stop channel")
		close(a.preventSleepStop)
		a.preventSleepStop = nil
	} else {
		debug.Println("[DEBUG] StopPreventSleep: not running, nothing to stop")
	}
	a.penInputDevice = ""
	a.mu.Unlock()
	if penDevice != "" {
		a.pokePenDevice(penDevice, false)
	}

	runtime.EventsEmit(a.ctx, "prevent-sleep-changed", false)
	return nil
}

func (a *App) IsPreventingSleep() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	running := a.preventSleepStop != nil
	debug.Printf("[DEBUG] IsPreventingSleep: %v\n", running)
	return running
}
