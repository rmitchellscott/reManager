package component

import (
	rmdevice "github.com/rmitchellscott/remarkable-go/device"
)

type CommandRunner interface {
	ExecuteWithOutput(command string) (string, error)
}

type CommandContext struct {
	Arch   rmdevice.Architecture
	Device rmdevice.Type
	Exec   CommandRunner
}

type CommandResult struct {
	Script      string
	Description string
	RequiresPTY bool
}

type DialogAction struct {
	Id    string
	Label string
	Type  string // "install_package", "run_command", "dismiss"
	Value string
}

// DialogSeverity is the worst outcome across every branch the prompt offers, not what it is asking for; empty reads as neutral.
type DialogSeverity string

const (
	DialogWarning DialogSeverity = "warning"
	DialogInfo    DialogSeverity = "info"
	DialogSuccess DialogSeverity = "success"
	DialogNeutral DialogSeverity = "neutral"
)

type DialogConfig struct {
	Title string
	// Subtitle renders in the header beside the badge, so keep it short; longer text belongs in Message, which gets the panel's full width.
	Subtitle          string
	Message           string
	Note              string
	Steps             []string
	ConfirmText       string
	CancelText        string
	InProgressMessage string
	InfoOnly          bool
	Severity          DialogSeverity
	ExpectedSeconds   int
	InstallFlow       bool
	Success           bool
	PrimaryAction     string
	Actions           []DialogAction
	// Choices marks the actions as options to pick between rather than things to do; they stack at any count.
	Choices           bool
	PostCommandDialog *DialogConfig
}

type HookExecutionResult struct {
	DialogConfig *DialogConfig
	Command      *CommandResult
}
