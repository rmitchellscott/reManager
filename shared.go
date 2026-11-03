package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"reManager/internal/component"
	"reManager/internal/debug"
	"reManager/internal/executor"
	"reManager/internal/logger"
	"reManager/internal/platform"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	eventMetadataLoaded = "metadata:loaded"
	eventMetadataError  = "metadata:error"
)

func init() {
	application.RegisterEvent[application.Void](eventMetadataLoaded)
	application.RegisterEvent[string](eventMetadataError)
}

type portalOpenOptions struct {
	dir            string
	multiple       bool
	directory      bool
	filterName     string
	filterPatterns []string
}

func resolveDialogDir(defaultDir string) string {
	if defaultDir != "" {
		if info, err := os.Stat(defaultDir); err == nil && info.IsDir() {
			return defaultDir
		}
	}
	return dialogHomeDir()
}

// The App Sandbox points HOME at the app's container, which is no place to start a file dialog, so the account's own home comes from the user database.
func dialogHomeDir() string {
	if account, err := user.Current(); err == nil && account.HomeDir != "" {
		return account.HomeDir
	}
	home, _ := os.UserHomeDir()
	return home
}

func openFileDialog(title, defaultDir string) (string, error) {
	dir := resolveDialogDir(defaultDir)
	if platform.IsRunningInFlatpak() {
		files, err := portalOpenFiles(title, portalOpenOptions{dir: dir})
		debug.Println("[DEBUG] openFileDialog: files:", files, "err:", err)
		if err != nil {
			return "", err
		}
		if len(files) == 0 {
			return "", nil
		}
		return files[0], nil
	}
	return application.Get().Dialog.OpenFile().
		CanChooseFiles(true).
		SetTitle(title).
		SetDirectory(dir).
		PromptForSingleSelection()
}

func openMultipleFilesDialog(title, defaultDir string) ([]string, error) {
	dir := resolveDialogDir(defaultDir)
	if platform.IsRunningInFlatpak() {
		files, err := portalOpenFiles(title, portalOpenOptions{dir: dir, multiple: true})
		debug.Println("[DEBUG] openMultipleFilesDialog: files:", files, "err:", err)
		if err != nil {
			return nil, err
		}
		return files, nil
	}
	return application.Get().Dialog.OpenFile().
		CanChooseFiles(true).
		SetTitle(title).
		SetDirectory(dir).
		PromptForMultipleSelection()
}

func saveFileDialog(title, defaultFilename, defaultDir string) (string, error) {
	if isMobile() {
		return sandboxSavePath(defaultFilename)
	}
	dir := resolveDialogDir(defaultDir)
	if platform.IsRunningInFlatpak() {
		files, err := portalSaveFile(title, dir, defaultFilename)
		debug.Println("[DEBUG] saveFileDialog: files:", files, "err:", err)
		if err != nil {
			return "", err
		}
		if len(files) == 0 {
			return "", nil
		}
		return files[0], nil
	}
	return application.Get().Dialog.SaveFile().
		SetMessage(title).
		SetFilename(defaultFilename).
		SetDirectory(dir).
		PromptForSingleSelection()
}

func openDirectoryDialog(title, defaultDir string) (string, error) {
	dir := resolveDialogDir(defaultDir)
	if platform.IsRunningInFlatpak() {
		files, err := portalOpenFiles(title, portalOpenOptions{dir: dir, directory: true})
		debug.Println("[DEBUG] openDirectoryDialog: files:", files, "err:", err)
		if err != nil {
			return "", err
		}
		if len(files) == 0 {
			return "", nil
		}
		return files[0], nil
	}
	return application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle(title).
		SetDirectory(dir).
		PromptForSingleSelection()
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "-", "<", "_", ">", "_", "\"", "_", "|", "_", "?", "_", "*", "_")
	return replacer.Replace(name)
}

type DialogActionRequest struct {
	Id    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type DialogRequest struct {
	Title             string                   `json:"title"`
	Subtitle          string                   `json:"subtitle"`
	Message           string                   `json:"message"`
	Note              string                   `json:"note"`
	Steps             []string                 `json:"steps"`
	ConfirmText       string                   `json:"confirmText"`
	CancelText        string                   `json:"cancelText"`
	InProgressMessage string                   `json:"inProgressMessage"`
	InfoOnly          bool                     `json:"infoOnly"`
	Severity          component.DialogSeverity `json:"severity,omitempty"`
	InstallFlow       bool                     `json:"installFlow"`
	Success           bool                     `json:"success"`
	PrimaryAction     string                   `json:"primaryAction"`
	Actions           []DialogActionRequest    `json:"actions"`
	Choices           bool                     `json:"choices"`
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
		Subtitle:          cfg.Subtitle,
		Message:           cfg.Message,
		Note:              cfg.Note,
		Steps:             cfg.Steps,
		ConfirmText:       cfg.ConfirmText,
		CancelText:        cfg.CancelText,
		InProgressMessage: cfg.InProgressMessage,
		InfoOnly:          cfg.InfoOnly,
		Severity:          cfg.Severity,
		InstallFlow:       cfg.InstallFlow,
		Success:           cfg.Success,
		PrimaryAction:     cfg.PrimaryAction,
		Actions:           actions,
		Choices:           cfg.Choices,
	}
}

// op is the operation a hook's commands belong to. Only Execute reads it; the
// query and streaming methods serve the vellum client, which has none.
type wailsExecutor struct {
	core *core
	op   *Operation
}

func (e *wailsExecutor) Execute(cmds []component.CommandResult) error {
	if e.op == nil {
		return fmt.Errorf("no operation to run commands in")
	}
	var operationErr error
	if e.core.logger != nil && len(cmds) > 0 {
		name := cmds[0].Description
		if name == "" {
			name = "execute"
		}
		e.core.operationLog = e.core.logger.StartCommandLog(e.core.connectedDeviceID, name)
		defer func() {
			e.core.operationLog.WriteExitCode(operationErr)
			e.core.operationLog.Close()
			e.core.operationLog = nil
		}()
	}

	for _, cmd := range cmds {
		if err := e.core.runOperationCommand(e.op, cmd.Script, cmd.RequiresPTY); err != nil {
			operationErr = err
			return operationErr
		}
	}
	return nil
}

func (e *wailsExecutor) ExecuteWithOutput(cmd string) (string, error) {
	if e.core.inDemoMode() {
		return "", nil
	}
	return e.core.runCommand(cmd)
}

func (e *wailsExecutor) ExecuteStreaming(cmd string, onOutput func(line string)) error {
	var cmdLog *logger.CommandLog
	if e.core.logger != nil {
		cmdLog = e.core.logger.StartCommandLog(e.core.connectedDeviceID, cmd)
	}
	defer cmdLog.Close()

	client := e.core.getClient()
	if client == nil {
		return fmt.Errorf("not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		if isConnectionDeadError(err) {
			go e.core.triggerConnectionCheck()
		}
		return err
	}
	defer session.Close()
	e.core.trackStreamingSession(session)
	defer e.core.untrackStreamingSession(session)

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	if err := session.Start(executor.WithDevicePath("echo " + streamingPIDMarker + "$$; " + cmd)); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)

	readLines := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			if pid, ok := strings.CutPrefix(line, streamingPIDMarker); ok {
				if n, err := strconv.Atoi(pid); err == nil {
					e.core.setStreamingPID(session, n)
				}
				continue
			}
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

func (c *core) trackStreamingSession(session *ssh.Session) {
	c.streamingMu.Lock()
	defer c.streamingMu.Unlock()
	if c.streamingSessions == nil {
		c.streamingSessions = make(map[*ssh.Session]int)
	}
	c.streamingSessions[session] = 0
}

func (c *core) setStreamingPID(session *ssh.Session, pid int) {
	c.streamingMu.Lock()
	defer c.streamingMu.Unlock()
	if _, ok := c.streamingSessions[session]; ok {
		c.streamingSessions[session] = pid
	}
}

func (c *core) untrackStreamingSession(session *ssh.Session) {
	c.streamingMu.Lock()
	defer c.streamingMu.Unlock()
	delete(c.streamingSessions, session)
}

func (c *core) streamingSessionOpen(session *ssh.Session) bool {
	c.streamingMu.Lock()
	defer c.streamingMu.Unlock()
	_, ok := c.streamingSessions[session]
	return ok
}

const (
	streamingPIDMarker  = "remanager-pid:"
	remoteSignalTimeout = 5 * time.Second
)

// The SSH server signals only the shell it started. vellum and apk under it
// keep running until a write to the closed channel kills them, which can be
// after apk has committed, so the whole tree under the shell is signaled from
// a second session. It is found through the shell, so this runs before the
// session signal that ends the shell and orphans the tree.
func signalProcessTreeScript(root int, signal string) string {
	return fmt.Sprintf(`pids=$(cat /proc/[0-9]*/stat 2>/dev/null | awk -v root=%d '{parent[$1] = $4} END {for (p in parent) {q = p; while (q != root && (q in parent)) q = parent[q]; if (q == root) print p}}'); [ -z "$pids" ] || kill -%s $pids 2>/dev/null; echo $pids`, root, signal)
}

func parsePIDs(output string) []string {
	var pids []string
	for _, field := range strings.Fields(output) {
		if _, err := strconv.Atoi(field); err == nil {
			pids = append(pids, field)
		}
	}
	return pids
}

// The vellum client streams its commands without an operation to cancel, so a
// stop ends every streaming command. Only one runs at a time, since the app
// holds other commands back while one is running.
func (c *core) stopStreamingCommands() {
	c.streamingMu.Lock()
	sessions := make(map[*ssh.Session]int, len(c.streamingSessions))
	for session, pid := range c.streamingSessions {
		sessions[session] = pid
	}
	c.streamingMu.Unlock()
	if len(sessions) == 0 {
		return
	}
	go func() {
		signaled := make(map[*ssh.Session][]string, len(sessions))
		for session, pid := range sessions {
			if pid > 0 {
				output, err := c.runCommandWithTimeout(signalProcessTreeScript(pid, "TERM"), remoteSignalTimeout)
				if err != nil {
					debug.Printf("[DEBUG] stopStreamingCommands: could not signal the processes under %d: %v\n", pid, err)
				}
				signaled[session] = parsePIDs(output)
			}
			session.Signal(ssh.SIGTERM)
		}
		time.Sleep(2 * time.Second)
		for session := range sessions {
			if pids := signaled[session]; len(pids) > 0 && c.streamingSessionOpen(session) {
				c.runCommandWithTimeout("kill -KILL "+strings.Join(pids, " ")+" 2>/dev/null", remoteSignalTimeout)
			}
			session.Signal(ssh.SIGKILL)
			session.Close()
		}
	}()
}
