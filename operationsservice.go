package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/crypto/ssh"

	"reManager/internal/backup"
	"reManager/internal/commands"
	"reManager/internal/component"
	"reManager/internal/debug"
	apperrors "reManager/internal/errors"
	"reManager/internal/executor"
	"reManager/internal/installer"
	"reManager/internal/vellum"
)

// The id the frontend chose matches every event to its run, so a late event from an earlier run is ignored rather than taken for the current one.

// Registered so the Wails binding generator emits these payload types, instead of the frontend keeping copies that drift.
const (
	eventOperationOutput   = "op:output"
	eventOperationProgress = "op:progress"
	eventOperationPrompt   = "op:prompt"
	eventOperationHook     = "op:hook"
	eventOperationDone     = "op:done"
)

func init() {
	application.RegisterEvent[OperationOutput](eventOperationOutput)
	application.RegisterEvent[OperationProgress](eventOperationProgress)
	application.RegisterEvent[OperationPrompt](eventOperationPrompt)
	application.RegisterEvent[OperationHook](eventOperationHook)
	application.RegisterEvent[OperationResult](eventOperationDone)
}

var (
	errOperationCanceled = errors.New("canceled")
	errNotConnected      = errors.New("not connected to your reMarkable")
)

type operationEmitter func(name string, data any)

type OperationsService struct {
	*core
}

type OperationOutput struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// Written while a hook runs; the frontend shows it on the hook's own surface, not in the log of the operation around it.
	Hook bool `json:"hook"`
}

type OperationProgress struct {
	ID        string             `json:"id"`
	Progress  InstallProgress    `json:"progress"`
	Backup    *backup.Progress   `json:"backup,omitempty"`
	OSInstall *OSInstallProgress `json:"osInstall,omitempty"`
}

type OperationPrompt struct {
	ID      string        `json:"id"`
	Request DialogRequest `json:"request"`
}

type OperationHook struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	ExpectedSeconds int    `json:"expectedSeconds"`
	Finished        bool   `json:"finished"`
	Error           string `json:"error"`
}

// Each kind's result sits in its own field so it reaches the frontend as a generated type, not an untyped value.
type OperationResult struct {
	ID            string                `json:"id"`
	OK            bool                  `json:"ok"`
	Canceled      bool                  `json:"canceled"`
	Error         string                `json:"error,omitempty"`
	Code          apperrors.ErrorCode   `json:"code,omitempty"`
	Install       *InstallResult        `json:"install,omitempty"`
	Upgrade       *UpgradeResult        `json:"upgrade,omitempty"`
	SupportBundle *SupportBundleResult  `json:"supportBundle,omitempty"`
	Reenable      vellum.ReenableStatus `json:"reenable,omitempty"`
	Value         string                `json:"value,omitempty"`
}

type Operation struct {
	ID string

	ctx     context.Context
	cancel  context.CancelFunc
	emit    operationEmitter
	prompts chan string
	finish  sync.Once

	mu            sync.Mutex
	work          *backgroundWork
	inHook        bool
	hookSeconds   int
	stopEstimate  func()
	pendingPrompt *DialogRequest
	stdin         io.Writer
}

func newOperation(parent context.Context, id string, emit operationEmitter) *Operation {
	ctx, cancel := context.WithCancel(parent)
	return &Operation{
		ID:      id,
		ctx:     ctx,
		cancel:  cancel,
		emit:    emit,
		prompts: make(chan string, 1),
	}
}

func (op *Operation) Output(text string) {
	if text == "" {
		return
	}
	op.mu.Lock()
	hook := op.inHook
	op.mu.Unlock()
	op.emit(eventOperationOutput, OperationOutput{ID: op.ID, Text: text, Hook: hook})
}

func (op *Operation) Progress(progress InstallProgress) {
	op.emit(eventOperationProgress, OperationProgress{ID: op.ID, Progress: progress})
	if work := op.currentWork(); work != nil {
		work.Progress(installProgressFraction(progress), installProgressSubtitle(progress))
	}
}

func (op *Operation) BackupProgress(progress backup.Progress) {
	op.emit(eventOperationProgress, OperationProgress{ID: op.ID, Backup: &progress})
	if work := op.currentWork(); work != nil {
		work.Progress(progress.Percentage/100, progress.Message)
	}
}

func (op *Operation) OSInstallProgress(stage OSInstallStage, percent float64, message string) {
	op.emit(eventOperationProgress, OperationProgress{ID: op.ID, OSInstall: &OSInstallProgress{Stage: stage, Percent: percent, Message: message}})
	if work := op.currentWork(); work != nil {
		work.Progress(percent/100, message)
	}
}

// Canceling the operation answers the prompt, so a prompt never outlives the run that raised it.
func (op *Operation) Prompt(request DialogRequest) string {
	op.mu.Lock()
	op.pendingPrompt = &request
	op.mu.Unlock()
	defer func() {
		op.mu.Lock()
		op.pendingPrompt = nil
		op.mu.Unlock()
	}()
	op.emit(eventOperationPrompt, OperationPrompt{ID: op.ID, Request: request})
	if work := op.currentWork(); work != nil {
		work.WaitOnYou(true)
		defer work.WaitOnYou(false)
	}
	notifyNeedsAttention(request.Title, cmp.Or(request.Message, request.Subtitle, "Open reManager to continue."))
	select {
	case response := <-op.prompts:
		return response
	case <-op.ctx.Done():
		return "cancel"
	}
}

func (op *Operation) respond(response string) {
	select {
	case op.prompts <- response:
	default:
	}
}

func (op *Operation) currentWork() *backgroundWork {
	op.mu.Lock()
	defer op.mu.Unlock()
	return op.work
}

// iOS's progress bar only moves forward, so a hook runs as its own job; the user just answered its prompt, so the app is foreground, where iOS lets a new job start.
func (op *Operation) BeginHook(title string, expectedSeconds int) {
	op.mu.Lock()
	op.inHook = true
	op.hookSeconds = expectedSeconds
	if op.work != nil {
		op.work.End()
		op.work = op.work.core.beginBackgroundWork(title)
		op.stopEstimate = op.work.EstimateFor(expectedSeconds)
	}
	op.mu.Unlock()
	op.emit(eventOperationHook, OperationHook{ID: op.ID, Title: title, ExpectedSeconds: expectedSeconds})
}

func (op *Operation) EndHook(title string, err error) {
	op.mu.Lock()
	op.inHook = false
	op.hookSeconds = 0
	if op.stopEstimate != nil {
		op.stopEstimate()
		op.stopEstimate = nil
	}
	op.mu.Unlock()
	hook := OperationHook{ID: op.ID, Title: title, Finished: true}
	if err != nil {
		hook.Error = err.Error()
	}
	op.emit(eventOperationHook, hook)
}

func (op *Operation) Canceled() bool {
	return op.ctx.Err() != nil
}

func (op *Operation) setStdin(stdin io.Writer) {
	op.mu.Lock()
	op.stdin = stdin
	op.mu.Unlock()
}

func (op *Operation) finishWith(result any, err error) {
	op.finish.Do(func() {
		done := OperationResult{ID: op.ID, OK: err == nil}
		switch r := result.(type) {
		case InstallResult:
			done.Install = &r
		case UpgradeResult:
			done.Upgrade = &r
		case SupportBundleResult:
			done.SupportBundle = &r
		case vellum.ReenableStatus:
			done.Reenable = r
		case string:
			done.Value = r
		}
		if err != nil {
			done.Error = err.Error()
			var userErr *apperrors.UserError
			if errors.As(err, &userErr) {
				done.Code = userErr.Code
			}
			done.Canceled = errors.Is(err, errOperationCanceled) || errors.Is(err, installer.ErrHookDeclined)
		}
		op.cancel()
		op.emit(eventOperationDone, done)
	})
}

// operationSink stands in for the Wails event bus in tests, which run without an application.
func (c *core) emitOperationEvent(name string, data any) {
	if c.operationSink != nil {
		c.operationSink(name, data)
		return
	}
	if c.wails != nil {
		c.wails.Event.Emit(name, data)
	}
}

// The bound methods that call this return before the work does, so a call never holds a WebView request open for the minutes an upgrade can take; completion arrives only as op:done.
func (c *core) runOperation(id, title string, body func(op *Operation) (any, error)) {
	parent := c.ctx
	if parent == nil {
		parent = context.Background()
	}
	op := newOperation(parent, id, c.emitOperationEvent)

	c.operationsMu.Lock()
	if c.operations == nil {
		c.operations = map[string]*Operation{}
	}
	c.operations[id] = op
	c.operationsMu.Unlock()

	op.work = c.beginBackgroundWork(title)

	go func() {
		defer func() { op.currentWork().End() }()

		var result any
		var err error
		defer func() {
			if recovered := recover(); recovered != nil {
				debug.Printf("[DEBUG] operation %s panicked: %v\n", id, recovered)
				err = fmt.Errorf("unexpected error: %v", recovered)
			}
			if err != nil && !errors.Is(err, errOperationCanceled) && !errors.Is(err, installer.ErrHookDeclined) {
				op.Output("\n" + sentence(err.Error()) + "\n")
				op.currentWork().Fail(err.Error())
			}
			c.operationsMu.Lock()
			delete(c.operations, id)
			c.operationsMu.Unlock()
			op.finishWith(result, err)
		}()
		result, err = body(op)
		// A stop kills the command the body was waiting on, so the body usually returns that command's failure rather than nil.
		if op.Canceled() {
			err = errOperationCanceled
		}
	}()
}

func sentence(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}
	message = strings.ToUpper(message[:1]) + message[1:]
	if !strings.HasSuffix(message, ".") {
		message += "."
	}
	return message
}

func (c *core) operationRunning() bool {
	c.operationsMu.Lock()
	defer c.operationsMu.Unlock()
	return len(c.operations) > 0
}

func (c *core) operation(id string) *Operation {
	c.operationsMu.Lock()
	defer c.operationsMu.Unlock()
	return c.operations[id]
}

// A command reading a terminal gets Ctrl+C first, since that lets a debug session exit cleanly; the session is closed shortly after if it has not.
func (s *OperationsService) CancelOperation(id string) {
	op := s.operation(id)
	if op == nil {
		return
	}
	op.mu.Lock()
	stdin := op.stdin
	op.mu.Unlock()
	if stdin != nil {
		if _, err := stdin.Write([]byte{0x03}); err != nil {
			debug.Printf("[DEBUG] CancelOperation: Ctrl+C failed: %v\n", err)
		}
	}
	op.cancel()
	s.stopStreamingCommands()
}

// The frontend clears its operation state when the connection drops, taking an open prompt with it while the operation still waits, and asks for the prompt again after a reconnect.
func (s *OperationsService) PendingOperationPrompt() *OperationPrompt {
	s.operationsMu.Lock()
	defer s.operationsMu.Unlock()
	for _, op := range s.operations {
		op.mu.Lock()
		pending := op.pendingPrompt
		op.mu.Unlock()
		if pending != nil {
			return &OperationPrompt{ID: op.ID, Request: *pending}
		}
	}
	return nil
}

// With no session left to answer into, an operation waiting on a prompt would otherwise wait forever.
func (c *core) cancelOperationsWaitingOnPrompts() {
	c.operationsMu.Lock()
	defer c.operationsMu.Unlock()
	for _, op := range c.operations {
		op.mu.Lock()
		waiting := op.pendingPrompt != nil
		op.mu.Unlock()
		if waiting {
			op.cancel()
		}
	}
}

func (s *OperationsService) RespondToOperation(id, response string) {
	if op := s.operation(id); op != nil {
		op.respond(response)
	}
}

func (c *core) outputToAllOperations(text string) {
	c.operationsMu.Lock()
	ops := make([]*Operation, 0, len(c.operations))
	for _, op := range c.operations {
		ops = append(ops, op)
	}
	c.operationsMu.Unlock()
	for _, op := range ops {
		op.Output(text)
	}
}

func stopSession(session *ssh.Session, waited <-chan error, err *error) bool {
	session.Signal(ssh.SIGTERM)
	select {
	case *err = <-waited:
		return true
	case <-time.After(2 * time.Second):
	}
	session.Signal(ssh.SIGKILL)
	session.Close()
	select {
	case *err = <-waited:
		return true
	case <-time.After(2 * time.Second):
		return false
	}
}

// Returns only once the command and both of its readers are done, so no output lands after the result.
func (c *core) runOperationCommand(op *Operation, script string, requiresPTY bool) error {
	op.Output(fmt.Sprintf("$ %s\n", script))

	if c.inDemoMode() {
		return c.demoRunOperationCommand(op)
	}

	cmdLog := c.operationLog
	ownLog := cmdLog == nil
	if ownLog && c.logger != nil {
		cmdLog = c.logger.StartCommandLog(c.connectedDeviceID, script)
	}
	if !ownLog {
		cmdLog.Write(fmt.Sprintf("\n$ %s\n", script))
	}
	defer func() {
		if ownLog && cmdLog != nil {
			cmdLog.Close()
		}
	}()

	client := c.getClient()
	if client == nil {
		return fmt.Errorf("not connected to your reMarkable")
	}
	c.sessionMu.Lock()
	session, err := client.NewSession()
	c.sessionMu.Unlock()
	if err != nil {
		if isConnectionDeadError(err) {
			go c.triggerConnectionCheck()
		}
		return err
	}
	defer session.Close()

	if requiresPTY {
		err = session.RequestPty("xterm-256color", 40, 120, ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		})
		if err != nil {
			return fmt.Errorf("could not open a terminal: %w", err)
		}
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	if err := session.Start(executor.WithDevicePath(script)); err != nil {
		return err
	}
	if requiresPTY {
		op.setStdin(stdin)
		defer op.setStdin(nil)
	} else {
		stdin.Close()
	}

	var readers sync.WaitGroup
	stream := func(r io.Reader) {
		defer readers.Done()
		buf := make([]byte, 1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				op.Output(chunk)
				cmdLog.Write(chunk)
			}
			if err != nil {
				return
			}
		}
	}
	readers.Add(2)
	go stream(stdout)
	go stream(stderr)

	waited := make(chan error, 1)
	go func() { waited <- session.Wait() }()

	select {
	case err = <-waited:
	case <-op.ctx.Done():
		// Closing the session alone leaves the command running on the reMarkable, so one that ignored Ctrl+C or had no terminal to receive it is signaled.
		select {
		case err = <-waited:
		case <-time.After(2 * time.Second):
			if !stopSession(session, waited, &err) {
				return errOperationCanceled
			}
		}
	}
	readers.Wait()

	if ownLog {
		cmdLog.WriteExitCode(err)
	}
	if vc := c.getVellumClient(); vc != nil {
		vc.InvalidateInstalledCache()
	}
	if op.Canceled() {
		return errOperationCanceled
	}
	return err
}

const writableRootRestoreTimeout = time.Minute

// The restore steps run even on a failed or canceled command, because a Paper Pro left with a writable root and no /etc overlay stays that way until it reboots.
func (c *core) runOperationCommandsWithWritableRoot(op *Operation, dev rmdevice.Type, cmds []component.CommandResult) (err error) {
	prepare, restore := commands.WritableRootSteps(dev)
	defer func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(op.ctx), writableRootRestoreTimeout)
		defer cancel()
		restoreOp := &Operation{ID: op.ID, ctx: ctx, cancel: cancel, emit: op.emit, prompts: op.prompts}
		for _, step := range restore {
			if restoreErr := c.runOperationCommand(restoreOp, step.Script, step.RequiresPTY); restoreErr != nil && err == nil {
				err = fmt.Errorf("could not restore the read-only root: %w", restoreErr)
			}
		}
	}()

	for _, step := range append(prepare, cmds...) {
		if err := c.runOperationCommand(op, step.Script, step.RequiresPTY); err != nil {
			return err
		}
	}
	return nil
}

func (c *core) runHook(op *Operation, hook *component.HookExecutionResult, exec *wailsExecutor) error {
	cfg := hook.DialogConfig
	if cfg == nil {
		return nil
	}

	response := op.Prompt(dialogRequestFromConfig(cfg))
	if response == "" || response == "cancel" {
		return installer.ErrHookDeclined
	}
	if cfg.InfoOnly {
		return nil
	}

	var chosen *component.DialogAction
	for i := range cfg.Actions {
		if cfg.Actions[i].Id == response && cfg.Actions[i].Type == "run_command" {
			chosen = &cfg.Actions[i]
			break
		}
	}

	if chosen != nil || hook.Command != nil {
		op.BeginHook(cfg.Title, cfg.ExpectedSeconds)
		var err error
		if chosen != nil {
			err = exec.Execute([]component.CommandResult{{Script: chosen.Value}})
		}
		if err == nil && hook.Command != nil {
			err = exec.Execute([]component.CommandResult{*hook.Command})
		}
		op.EndHook(cfg.Title, err)
		if err != nil {
			return err
		}
	}

	post := cfg.PostCommandDialog
	if post == nil {
		return nil
	}
	// End the job before the post prompt so it reads as finished in the system UI; a backgrounded app may lose the session while waiting, and the reconnect restores the prompt.
	if work := op.currentWork(); work != nil {
		work.End()
	}
	postResponse := op.Prompt(dialogRequestFromConfig(post))
	for _, action := range post.Actions {
		if action.Id == postResponse && action.Type == "run_command" {
			return exec.Execute([]component.CommandResult{{Script: action.Value}})
		}
	}
	return nil
}
