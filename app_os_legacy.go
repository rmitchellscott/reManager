package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"

	"reManager/internal/debug"
	"reManager/internal/httputil"
	"reManager/internal/logger"
	"reManager/internal/swupdate"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"
	rmexecutor "github.com/rmitchellscott/remarkable-go/executor"
	rmfilesystem "github.com/rmitchellscott/remarkable-go/filesystem"
	"github.com/rmitchellscott/remarkable-go/omaha"
	"github.com/rmitchellscott/remarkable-go/updateconf"
	rmversion "github.com/rmitchellscott/remarkable-go/version"
)

const (
	updateConfPath       = "/usr/share/remarkable/update.conf"
	updateConfBackupPath = updateConfPath + ".remanagerbak"
)

type legacyUpdateServer struct {
	version   string
	imageName string
	imagePath string
	size      int64
	sha1      string
	sha256    string
	tunnelURL string
	onEvent   func(omaha.Event)
}

func (s *legacyUpdateServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/"+s.imageName, s.serveImage)
	mux.HandleFunc("/", s.handleCheckin)
	return mux
}

func (s *legacyUpdateServer) handleCheckin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, "reManager legacy update server\n")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	req, err := omaha.ParseRequest(body)
	if err != nil {
		http.Error(w, "bad omaha request", http.StatusBadRequest)
		return
	}

	if req.IsUpdateCheck() {
		writeXML(w, omaha.BuildUpdateResponse(omaha.Update{
			Version:  s.version,
			Name:     s.imageName,
			Size:     s.size,
			SHA1:     s.sha1,
			SHA256:   s.sha256,
			Codebase: s.tunnelURL,
		}))
		return
	}

	if ev := req.App.Event; ev != nil && s.onEvent != nil {
		s.onEvent(*ev)
	}
	writeXML(w, omaha.NoUpdateResponse())
}

func (s *legacyUpdateServer) serveImage(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open(s.imagePath)
	if err != nil {
		http.Error(w, "image unavailable", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, "image unavailable", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, s.imageName, fi.ModTime(), f)
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	io.WriteString(w, body)
}

func (a *App) installLegacyOSVersion(version string, cancelCh chan struct{}) {
	a.mu.Lock()
	client := a.client
	deviceType := a.connectedDeviceType
	deviceID := a.connectedDeviceID
	a.mu.Unlock()

	emitErr := func(msg string) {
		runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{"error": msg})
	}
	emitProg := func(phase string, pct float64, msg string) {
		runtime.EventsEmit(a.ctx, "software:install-progress", map[string]interface{}{"phase": phase, "percent": pct, "message": msg})
	}
	cancelled := func() bool {
		select {
		case <-cancelCh:
			return true
		default:
			return false
		}
	}

	if client == nil {
		emitErr("not connected")
		return
	}

	sess := &installSession{
		deviceID: deviceID,
		version:  version,
		mode:     "legacy",
		resume:   make(chan *ssh.Client, 1),
	}
	a.setInstallSession(sess)

	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	go func() {
		select {
		case <-cancelCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	entry, err := swupdate.LegacyEntry(string(deviceType), version)
	if err != nil {
		emitErr(err.Error())
		return
	}

	emitProg("downloading", 0, "Downloading image...")
	tmpFile, err := os.CreateTemp("", "remanager-signed-*.signed")
	if err != nil {
		emitErr("failed to create temp file: " + err.Error())
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := downloadLegacyImage(ctx, swupdate.LegacyImageURL(entry.Key), tmpFile, entry.Size, cancelled, func(pct float64) {
		emitProg("downloading", pct*0.4, "Downloading image...")
	}); err != nil {
		tmpFile.Close()
		emitErr(err.Error())
		return
	}
	tmpFile.Close()

	if cancelled() {
		emitErr("cancelled")
		return
	}

	if err := verifyLegacyImage(tmpPath, entry.Size, entry.SHA256); err != nil {
		emitErr(err.Error())
		return
	}

	emitProg("downloading", 40, "Downloading to device...")
	lis, err := client.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		emitErr("device refused the update tunnel (sshd AllowTcpForwarding may be disabled): " + err.Error())
		return
	}
	tunnelURL := "http://" + lis.Addr().String() + "/"

	var cmdLog *logger.CommandLog
	if a.logger != nil {
		cmdLog = a.logger.StartCommandLog(deviceID, "legacy-update")
		defer cmdLog.Close()
	}

	a.mu.Lock()
	firmware := a.connectedFirmware
	a.mu.Unlock()
	activeSlot, targetSlot, activeVer := 0, 0, ""
	if info, e := a.GetPartitionInfo(); e == nil && info != nil {
		activeSlot, targetSlot, activeVer = info.Active.Number, info.Fallback.Number, info.Active.Version
		sess.targetPartition = targetSlot
	}
	logResult := func(status, detail string) {
		if cmdLog != nil {
			cmdLog.Write(fmt.Sprintf("[result] %s %s\n", status, detail))
		}
		if a.logger != nil {
			a.logger.LogInstall("legacy", version, status+": "+detail)
		}
	}
	if cmdLog != nil {
		cmdLog.Write(fmt.Sprintf("[install] mode=legacy target_version=%s image=%s size=%d sha256=%s\n", version, filepath.Base(entry.Key), entry.Size, entry.SHA256))
		cmdLog.Write(fmt.Sprintf("[install] device=%s firmware=%s active_slot=%d target_slot=%d active_version=%s\n", deviceType, firmware, activeSlot, targetSlot, activeVer))
	}
	if a.logger != nil {
		a.logger.LogInstall("legacy", version, fmt.Sprintf("start: device=%s firmware=%s active_slot=%d target_slot=%d", deviceType, firmware, activeSlot, targetSlot))
	}

	logEvent := func(ev omaha.Event) {
		if cmdLog != nil {
			cmdLog.Write(fmt.Sprintf("[omaha event] type=%d result=%d errorcode=%s\n", ev.Type, ev.Result, ev.ErrorCode))
		}
	}

	srv := &http.Server{Handler: (&legacyUpdateServer{
		version:   entry.Version,
		imageName: filepath.Base(entry.Key),
		imagePath: tmpPath,
		size:      entry.Size,
		sha1:      entry.SHA1,
		sha256:    entry.SHA256,
		tunnelURL: tunnelURL,
		onEvent:   logEvent,
	}).handler()}
	go srv.Serve(lis)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
		lis.Close()
	}()

	exec := rmexecutor.NewSSH(client, rmexecutor.WithLogger(debug.SlogLogger()))
	fs, err := rmfilesystem.NewSSH(client)
	if err != nil {
		emitErr("failed to open device filesystem: " + err.Error())
		return
	}
	defer fs.Close()

	orig, err := fs.ReadFile(updateConfPath)
	if err != nil {
		emitErr("failed to read update.conf (is this a legacy reMarkable?): " + err.Error())
		return
	}
	if exists, _ := fs.Exists(updateConfBackupPath); !exists {
		if err := fs.WriteFile(updateConfBackupPath, orig, 0644); err != nil {
			emitErr("failed to back up update.conf: " + err.Error())
			return
		}
	}

	restore := func() {
		a.withWritableRoot(client, updateConfPath, func() error {
			return fs.WriteFile(updateConfPath, orig, 0644)
		})
		fs.Remove(updateConfBackupPath)
	}

	newConf := updateconf.SetServer(orig, tunnelURL)
	if err := a.withWritableRoot(client, updateConfPath, func() error {
		return fs.WriteFile(updateConfPath, newConf, 0644)
	}); err != nil {
		restore()
		emitErr("failed to update update.conf: " + err.Error())
		return
	}

	failAfterRewrite := func(msg string) {
		restore()
		logResult("failed", msg)
		emitErr(msg)
	}

	if cancelled() {
		failAfterRewrite("cancelled")
		return
	}

	if _, err := exec.Run(ctx, "systemctl", "restart", "update-engine"); err != nil {
		failAfterRewrite("failed to restart update-engine: " + err.Error())
		return
	}
	time.Sleep(2 * time.Second)

	statusDone := make(chan struct{})
	go a.pollLegacyStatus(ctx, exec, statusDone, emitProg)

	emitProg("downloading", 40, "Downloading to device...")
	var out io.Writer = io.Discard
	if cmdLog != nil {
		out = &cmdLogWriter{cmdLog: cmdLog}
	}
	updateErr := exec.RunStreaming(ctx, "update_engine_client", out, out, "-update")
	close(statusDone)

	if updateErr != nil {
		if cancelled() {
			failAfterRewrite("cancelled")
			return
		}
		sess.mu.Lock()
		lost := sess.connLost
		sess.mu.Unlock()
		if lost {
			a.reconcileLegacyInstall(sess, restore, version, emitProg, logResult)
			return
		}
		failAfterRewrite("update failed: " + updateErr.Error())
		return
	}

	// Restore before reboot: the edit is on the active partition, which
	// update_engine has flipped away from for next boot.
	emitProg("installing", 100, "Installing...")
	restore()

	logResult("success", "standby slot written, awaiting reboot")
	runtime.EventsEmit(a.ctx, "software:install-progress", map[string]interface{}{"phase": "complete", "percent": 100.0, "message": "Complete"})
	runtime.EventsEmit(a.ctx, "software:install-complete", map[string]interface{}{"version": version, "partition": "standby"})
}

func (a *App) reconcileLegacyInstall(sess *installSession, restore func(), version string, emitProg func(string, float64, string), logResult func(string, string)) {
	emitProg("installing", 95, "Connection lost during install — reconnecting and verifying on device...")

	var client *ssh.Client
	select {
	case client = <-sess.resume:
	case <-a.ctx.Done():
		return
	case <-time.After(3 * time.Minute):
		logResult("unknown", "lost contact during install; never reconnected")
		runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{
			"error": "Lost contact with the device during the install. Reconnect and check the device before switching partitions — the standby slot may be incomplete.",
		})
		return
	}

	exec := rmexecutor.NewSSH(client, rmexecutor.WithLogger(debug.SlogLogger()))
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	overall := time.After(10 * time.Minute)
	idleSeen := 0

	failIncomplete := func() {
		restore()
		logResult("failed", "update did not complete after reconnect (standby slot incomplete)")
		runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{
			"error": "The update did not complete. The standby slot may be incomplete — reinstall before switching to it.",
		})
	}

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-overall:
			logResult("unknown", "timed out verifying update after reconnect")
			runtime.EventsEmit(a.ctx, "software:install-error", map[string]string{
				"error": "Timed out verifying the update after reconnect. Check the device before switching partitions.",
			})
			return
		case <-ticker.C:
		}

		res, err := exec.Run(a.ctx, "update_engine_client", "-status")
		if err != nil || res == nil {
			// Dropped again — pick up the current client and keep trying.
			if c := a.getClient(); c != nil {
				exec = rmexecutor.NewSSH(c, rmexecutor.WithLogger(debug.SlogLogger()))
			}
			continue
		}

		op, prog := parseUpdateStatus(res.Stdout)
		phase, pct, msg := legacyPhase(op, prog)
		emitProg(phase, pct, msg)

		switch op {
		case "UPDATE_STATUS_UPDATED_NEED_REBOOT":
			restore()
			logResult("success", "update completed after reconnect, awaiting reboot")
			runtime.EventsEmit(a.ctx, "software:install-progress", map[string]interface{}{"phase": "complete", "percent": 100.0, "message": "Complete"})
			runtime.EventsEmit(a.ctx, "software:install-complete", map[string]interface{}{"version": version, "partition": "standby"})
			return
		case "UPDATE_STATUS_REPORTING_ERROR_EVENT":
			failIncomplete()
			return
		case "UPDATE_STATUS_IDLE":
			// IDLE here means aborted; require two reads to avoid a reconnect race.
			idleSeen++
			if idleSeen >= 2 {
				failIncomplete()
				return
			}
		default:
			idleSeen = 0
		}
	}
}

func downloadLegacyImage(ctx context.Context, url string, w io.Writer, totalSize int64, cancelled func() bool, onProgress func(float64)) error {
	resp, err := httputil.NewClient(10 * time.Minute).Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	if totalSize <= 0 {
		totalSize = resp.ContentLength
	}

	buf := make([]byte, 32*1024)
	var downloaded int64
	for {
		if cancelled() || ctx.Err() != nil {
			return fmt.Errorf("cancelled")
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("write error: %w", writeErr)
			}
			downloaded += int64(n)
			if totalSize > 0 && onProgress != nil {
				onProgress(float64(downloaded) / float64(totalSize))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("download error: %w", readErr)
		}
	}
	return nil
}

func verifyLegacyImage(path string, wantSize int64, wantSHA256 string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open downloaded image: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat downloaded image: %w", err)
	}
	if wantSize > 0 && fi.Size() != wantSize {
		return fmt.Errorf("downloaded image size %d does not match manifest %d", fi.Size(), wantSize)
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to hash downloaded image: %w", err)
	}
	if got := base64.StdEncoding.EncodeToString(h.Sum(nil)); wantSHA256 != "" && got != wantSHA256 {
		return fmt.Errorf("downloaded image failed integrity check (sha256 mismatch)")
	}
	return nil
}

func (a *App) pollLegacyStatus(ctx context.Context, exec *rmexecutor.SSH, done <-chan struct{}, emitProg func(string, float64, string)) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			res, err := exec.Run(ctx, "update_engine_client", "-status")
			if err != nil || res == nil {
				continue
			}
			op, prog := parseUpdateStatus(res.Stdout)
			phase, pct, msg := legacyPhase(op, prog)
			emitProg(phase, pct, msg)
		}
	}
}

func parseUpdateStatus(status string) (op string, progress float64) {
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "CURRENT_OP="); ok {
			op = v
		} else if v, ok := strings.CutPrefix(line, "PROGRESS="); ok {
			fmt.Sscanf(v, "%f", &progress)
		}
	}
	return op, progress
}

func legacyPhase(op string, progress float64) (phase string, pct float64, msg string) {
	switch op {
	case "UPDATE_STATUS_CHECKING_FOR_UPDATE", "UPDATE_STATUS_UPDATE_AVAILABLE":
		return "downloading", 40, "Downloading to device..."
	case "UPDATE_STATUS_DOWNLOADING":
		return "downloading", 40 + progress*55, "Downloading to device..."
	case "UPDATE_STATUS_VERIFYING", "UPDATE_STATUS_FINALIZING", "UPDATE_STATUS_UPDATED_NEED_REBOOT":
		return "installing", 100, "Installing..."
	default:
		return "downloading", 40, "Downloading to device..."
	}
}

func (a *App) restoreLegacyUpdateConf(client *ssh.Client) bool {
	fs, err := rmfilesystem.NewSSH(client)
	if err != nil {
		return false
	}
	defer fs.Close()

	conf, err := fs.ReadFile(updateConfPath)
	if err != nil {
		return false
	}
	backupExists, _ := fs.Exists(updateConfBackupPath)
	hijacked := strings.Contains(updateconf.CurrentServer(conf), "127.0.0.1")
	if !backupExists && !hijacked {
		return false
	}

	if backupExists {
		if backup, err := fs.ReadFile(updateConfBackupPath); err == nil {
			a.withWritableRoot(client, updateConfPath, func() error {
				return fs.WriteFile(updateConfPath, backup, 0644)
			})
		}
		fs.Remove(updateConfBackupPath)
		return true
	}

	cleaned := updateconf.SetServer(conf, "https://updates.cloud.remarkable.com/")
	a.withWritableRoot(client, updateConfPath, func() error {
		return fs.WriteFile(updateConfPath, cleaned, 0644)
	})
	return true
}

func (a *App) maybeRecoverLegacyConf() {
	a.mu.Lock()
	client := a.client
	deviceType := a.connectedDeviceType
	firmware := a.connectedFirmware
	a.mu.Unlock()

	if client == nil {
		return
	}
	if deviceType != rmdevice.RM1 && deviceType != rmdevice.RM2 {
		return
	}
	if firmware == "" || rmversion.Compare(firmware, minSwupdateVersion) >= 0 {
		return
	}
	if a.restoreLegacyUpdateConf(client) {
		runtime.EventsEmit(a.ctx, "os:legacy-conf-restored", map[string]interface{}{})
	}
}
