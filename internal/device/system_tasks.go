package device

import "reManager/internal/component"

type SystemTask struct {
	ID                 string
	Label              string
	Description        string
	DeviceTypes        []component.DeviceType
	Command            func(ctx component.CommandContext) []component.CommandResult
	RequiresTerminal   bool
	NeedsWriteableRoot bool
}

var SystemTasks = []SystemTask{
	{
		ID:          "enable-updates",
		Label:       "Enable Auto-Updates",
		Description: "Re-enable automatic software updates",
		Command: func(ctx component.CommandContext) []component.CommandResult {
			return []component.CommandResult{
				{
					Script:      "systemctl enable update-engine.service",
					Description: "Enable update service",
				},
				{
					Script:      "systemctl start update-engine.service",
					Description: "Start update service",
				},
			}
		},
		RequiresTerminal:   true,
		NeedsWriteableRoot: true,
	},
	{
		ID:          "disable-updates",
		Label:       "Disable Auto-Updates",
		Description: "Prevent automatic software updates",
		Command: func(ctx component.CommandContext) []component.CommandResult {
			return []component.CommandResult{
				{
					Script:      "systemctl stop update-engine.service",
					Description: "Stop update service",
				},
				{
					Script:      "systemctl disable update-engine.service",
					Description: "Disable update service",
				},
			}
		},
		RequiresTerminal:   true,
		NeedsWriteableRoot: true,
	},
	{
		ID:          "restart-xochitl",
		Label:       "Restart reMarkable UI",
		Description: "Restart the Xochitl UI service",
		Command: func(ctx component.CommandContext) []component.CommandResult {
			return []component.CommandResult{
				{
					Script:      "systemctl restart xochitl",
					Description: "Restart Xochitl service",
				},
			}
		},
		RequiresTerminal:   true,
		NeedsWriteableRoot: false,
	},
}

func GetSystemTask(id string) *SystemTask {
	for i := range SystemTasks {
		if SystemTasks[i].ID == id {
			return &SystemTasks[i]
		}
	}
	return nil
}
