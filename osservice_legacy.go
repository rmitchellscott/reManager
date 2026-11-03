package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

func (s *OSService) installLegacyOSVersion(op *Operation, version string) error {
	s.mu.Lock()
	client := s.client
	deviceType := s.connectedDeviceType
	deviceID := s.connectedDeviceID
	s.mu.Unlock()

	if client == nil {
		return errNotConnected
	}

	sess := &installSession{
		deviceID: deviceID,
		version:  version,
		mode:     "legacy",
		resume:   make(chan *ssh.Client, 1),
	}
	s.setInstallSession(sess)

	ctx := op.ctx

	entry, err := swupdate.LegacyEntry(string(deviceType), version)
	if err != nil {
		return err
	}

	op.OSInstallProgress(OSInstallDownloading, 0, "Downloading image…")
	tmpFile, err := os.CreateTemp("", "remanager-signed-*.signed")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := downloadLegacyImage(ctx, swupdate.LegacyImageURL(entry.Key), tmpFile, entry.Size, op.Canceled, func(pct float64) {
		op.OSInstallProgress(OSInstallDownloading, pct*0.4, "Downloading image…")
	}); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	if op.Canceled() {
		return errOperationCanceled
	}

	if err := verifyLegacyImage(tmpPath, entry.Size, entry.SHA256); err != nil {
		return err
	}

	op.OSInstallProgress(OSInstallDownloading, 40, "Downloading to your reMarkable…")
	lis, err := client.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("device refused the update tunnel (sshd AllowTcpForwarding may be disabled): %w", err)
	}
	tunnelURL := "http://" + lis.Addr().String() + "/"

	var cmdLog *logger.CommandLog
	if s.logger != nil {
		cmdLog = s.logger.StartCommandLog(deviceID, "legacy-update")
		defer cmdLog.Close()
	}

	s.mu.Lock()
	firmware := s.connectedFirmware
	s.mu.Unlock()
	activeSlot, targetSlot, activeVer := 0, 0, ""
	if info, e := s.GetPartitionInfo(); e == nil && info != nil {
		activeSlot, targetSlot, activeVer = info.Active.Number, info.Fallback.Number, info.Active.Version
		sess.targetPartition = targetSlot
	}
	logResult := func(status, detail string) {
		if cmdLog != nil {
			cmdLog.Write(fmt.Sprintf("[result] %s %s\n", status, detail))
		}
		if s.logger != nil {
			s.logger.LogInstall("legacy", version, status+": "+detail)
		}
	}
	if cmdLog != nil {
		cmdLog.Write(fmt.Sprintf("[install] mode=legacy target_version=%s image=%s size=%d sha256=%s\n", version, filepath.Base(entry.Key), entry.Size, entry.SHA256))
		cmdLog.Write(fmt.Sprintf("[install] device=%s firmware=%s active_slot=%d target_slot=%d active_version=%s\n", deviceType, firmware, activeSlot, targetSlot, activeVer))
	}
	if s.logger != nil {
		s.logger.LogInstall("legacy", version, fmt.Sprintf("start: device=%s firmware=%s active_slot=%d target_slot=%d", deviceType, firmware, activeSlot, targetSlot))
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
		return fmt.Errorf("failed to open device filesystem: %w", err)
	}
	defer fs.Close()

	orig, err := fs.ReadFile(updateConfPath)
	if err != nil {
		return fmt.Errorf("failed to read update.conf (is this a legacy reMarkable?): %w", err)
	}
	if exists, _ := fs.Exists(updateConfBackupPath); !exists {
		if err := fs.WriteFile(updateConfBackupPath, orig, 0644); err != nil {
			return fmt.Errorf("failed to back up update.conf: %w", err)
		}
	}

	restore := func() {
		s.withWritableRoot(client, updateConfPath, func() error {
			return fs.WriteFile(updateConfPath, orig, 0644)
		})
		fs.Remove(updateConfBackupPath)
	}

	newConf := updateconf.SetServer(orig, tunnelURL)
	if err := s.withWritableRoot(client, updateConfPath, func() error {
		return fs.WriteFile(updateConfPath, newConf, 0644)
	}); err != nil {
		restore()
		return fmt.Errorf("failed to update update.conf: %w", err)
	}

	failAfterRewrite := func(err error) error {
		restore()
		logResult("failed", err.Error())
		return err
	}

	if op.Canceled() {
		return failAfterRewrite(errOperationCanceled)
	}

	if _, err := exec.Run(ctx, "systemctl", "restart", "update-engine"); err != nil {
		return failAfterRewrite(fmt.Errorf("failed to restart update-engine: %w", err))
	}
	time.Sleep(2 * time.Second)

	statusDone := make(chan struct{})
	go s.pollLegacyStatus(ctx, exec, statusDone, op.OSInstallProgress)

	op.OSInstallProgress(OSInstallDownloading, 40, "Downloading to your reMarkable…")
	var out io.Writer = io.Discard
	if cmdLog != nil {
		out = &cmdLogWriter{cmdLog: cmdLog}
	}
	updateErr := exec.RunStreaming(ctx, "update_engine_client", out, out, "-update")
	close(statusDone)

	if updateErr != nil {
		if op.Canceled() {
			return failAfterRewrite(errOperationCanceled)
		}
		sess.mu.Lock()
		lost := sess.connLost
		sess.mu.Unlock()
		if lost {
			return s.reconcileLegacyInstall(op, sess, restore, logResult)
		}
		return failAfterRewrite(fmt.Errorf("update failed: %w", updateErr))
	}

	// Restore before reboot: the edit is on the active partition, which update_engine has flipped away from for next boot.
	op.OSInstallProgress(OSInstallInstalling, 100, "Installing…")
	restore()

	logResult("success", "standby slot written, awaiting reboot")
	op.OSInstallProgress(OSInstallComplete, 100, "Complete")
	return nil
}

func (s *OSService) reconcileLegacyInstall(op *Operation, sess *installSession, restore func(), logResult func(string, string)) error {
	op.OSInstallProgress(OSInstallInstalling, 95, "Connection lost during install. Reconnecting to check your reMarkable…")

	var client *ssh.Client
	select {
	case client = <-sess.resume:
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-time.After(3 * time.Minute):
		logResult("unknown", "lost contact during install; never reconnected")
		return errors.New("Lost contact with your reMarkable during the install. The standby slot may be incomplete, so reconnect and check it before switching partitions.")
	}

	exec := rmexecutor.NewSSH(client, rmexecutor.WithLogger(debug.SlogLogger()))
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	overall := time.After(10 * time.Minute)
	idleSeen := 0

	failIncomplete := func() error {
		restore()
		logResult("failed", "update did not complete after reconnect (standby slot incomplete)")
		return errors.New("The update did not complete. The standby slot may be incomplete. Reinstall before switching to it.")
	}

	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case <-overall:
			logResult("unknown", "timed out verifying update after reconnect")
			return errors.New("Timed out checking the update after reconnecting. Check your reMarkable before switching partitions.")
		case <-ticker.C:
		}

		res, err := exec.Run(s.ctx, "update_engine_client", "-status")
		if err != nil || res == nil {
			if c := s.getClient(); c != nil {
				exec = rmexecutor.NewSSH(c, rmexecutor.WithLogger(debug.SlogLogger()))
			}
			continue
		}

		status, prog := parseUpdateStatus(res.Stdout)
		op.OSInstallProgress(legacyPhase(status, prog))

		switch status {
		case "UPDATE_STATUS_UPDATED_NEED_REBOOT":
			restore()
			logResult("success", "update completed after reconnect, awaiting reboot")
			op.OSInstallProgress(OSInstallComplete, 100, "Complete")
			return nil
		case "UPDATE_STATUS_REPORTING_ERROR_EVENT":
			return failIncomplete()
		case "UPDATE_STATUS_IDLE":
			// IDLE here means aborted; require two reads to avoid a reconnect race.
			idleSeen++
			if idleSeen >= 2 {
				return failIncomplete()
			}
		default:
			idleSeen = 0
		}
	}
}

func downloadLegacyImage(ctx context.Context, url string, w io.Writer, totalSize int64, canceled func() bool, onProgress func(float64)) error {
	resp, err := httputil.GetStreaming(ctx, url)
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
		if canceled() || ctx.Err() != nil {
			return fmt.Errorf("canceled")
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

func (s *OSService) pollLegacyStatus(ctx context.Context, exec *rmexecutor.SSH, done <-chan struct{}, emitProg func(OSInstallStage, float64, string)) {
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
			emitProg(legacyPhase(parseUpdateStatus(res.Stdout)))
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

func legacyPhase(op string, progress float64) (stage OSInstallStage, pct float64, msg string) {
	switch op {
	case "UPDATE_STATUS_CHECKING_FOR_UPDATE", "UPDATE_STATUS_UPDATE_AVAILABLE":
		return OSInstallDownloading, 40, "Downloading to your reMarkable…"
	case "UPDATE_STATUS_DOWNLOADING":
		return OSInstallDownloading, 40 + progress*55, "Downloading to your reMarkable…"
	case "UPDATE_STATUS_VERIFYING", "UPDATE_STATUS_FINALIZING", "UPDATE_STATUS_UPDATED_NEED_REBOOT":
		return OSInstallInstalling, 100, "Installing…"
	default:
		return OSInstallDownloading, 40, "Downloading to your reMarkable…"
	}
}

func (s *OSService) restoreLegacyUpdateConf(client *ssh.Client) {
	fs, err := rmfilesystem.NewSSH(client)
	if err != nil {
		return
	}
	defer fs.Close()

	conf, err := fs.ReadFile(updateConfPath)
	if err != nil {
		return
	}
	backupExists, _ := fs.Exists(updateConfBackupPath)
	hijacked := strings.Contains(updateconf.CurrentServer(conf), "127.0.0.1")
	if !backupExists && !hijacked {
		return
	}

	if backupExists {
		if backup, err := fs.ReadFile(updateConfBackupPath); err == nil {
			s.withWritableRoot(client, updateConfPath, func() error {
				return fs.WriteFile(updateConfPath, backup, 0644)
			})
		}
		fs.Remove(updateConfBackupPath)
		return
	}

	cleaned := updateconf.SetServer(conf, "https://updates.cloud.remarkable.com/")
	s.withWritableRoot(client, updateConfPath, func() error {
		return fs.WriteFile(updateConfPath, cleaned, 0644)
	})
}

func (s *OSService) maybeRecoverLegacyConf() {
	s.mu.Lock()
	client := s.client
	deviceType := s.connectedDeviceType
	firmware := s.connectedFirmware
	s.mu.Unlock()

	if client == nil {
		return
	}
	if deviceType != rmdevice.RM1 && deviceType != rmdevice.RM2 {
		return
	}
	if firmware == "" || rmversion.Compare(firmware, minSwupdateVersion) >= 0 {
		return
	}
	s.restoreLegacyUpdateConf(client)
}
