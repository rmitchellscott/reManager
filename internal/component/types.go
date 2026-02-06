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
	DeviceRMPPM DeviceType = "rmppm"
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

type DialogConfig struct {
	Title             string
	Message           string
	Steps             []string
	ConfirmText       string
	InProgressMessage string
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
