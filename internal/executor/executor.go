package executor

import "reManager/internal/component"

type ProgressStatus string

const (
	StatusInstalling ProgressStatus = "installing"
	StatusCompleted  ProgressStatus = "completed"
	StatusError      ProgressStatus = "error"
)

type ProgressInfo struct {
	CurrentComponent string
	TotalComponents  int
	CurrentIndex     int
	Status           ProgressStatus
	Message          string
}

type ProgressCallback func(progress ProgressInfo)

type CommandExecutor interface {
	Execute(commands []component.CommandResult) error
	ExecuteWithOutput(cmd string) (string, error)
	ExecuteStreaming(cmd string, onOutput func(line string)) error
}
