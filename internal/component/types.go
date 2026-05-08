package component

type DeviceArchitecture string

const (
	ArchArm32   DeviceArchitecture = "armv7"
	ArchAarch64 DeviceArchitecture = "aarch64"
)

type DeviceType string

const (
	DeviceRM1   DeviceType = "rm1"
	DeviceRM2   DeviceType = "rm2"
	DeviceRMPP  DeviceType = "rmpp"
	DeviceRMPPMove DeviceType = "rmppmove"
	DeviceRMPPure  DeviceType = "rmppure"
)

type CommandContext struct {
	Arch   DeviceArchitecture
	Device DeviceType
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

type HookType string

const (
	HookTypeDialog       HookType = "dialog"
	HookTypeConfirmation HookType = "confirmation"
	HookTypeCustom       HookType = "custom"
)
