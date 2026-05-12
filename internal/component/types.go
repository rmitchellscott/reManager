package component

import (
	rmdevice "github.com/rmitchellscott/remarkable-go/device"
)

type CommandContext struct {
	Arch   rmdevice.Architecture
	Device rmdevice.Type
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

type DialogConfig struct {
	Title             string
	Message           string
	Note              string
	Steps             []string
	ConfirmText       string
	CancelText        string
	InProgressMessage string
	InfoOnly          bool
	InstallFlow       bool
	Actions           []DialogAction
	PostCommandDialog *DialogConfig
}

type HookExecutionResult struct {
	DialogConfig *DialogConfig
	Command      *CommandResult
}
