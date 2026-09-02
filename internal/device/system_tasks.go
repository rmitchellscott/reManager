package device

import (
	"fmt"

	rmdevice "github.com/rmitchellscott/remarkable-go/device"

	"reManager/internal/component"
)

const xochitlConfPath = "/home/root/.config/remarkable/xochitl.conf"

const minEnforcedAutoUpdateVersion = "3.28.0.0"

func setAutoUpdateScript(enabled bool) string {
	value := "false"
	if enabled {
		value = "true"
	}
	return fmt.Sprintf(`CONF=%s; [ -f "$CONF" ] || exit 0; `+
		`if grep -q '^AutoUpdate=' "$CONF"; then sed -i 's/^AutoUpdate=.*/AutoUpdate=%s/' "$CONF"; `+
		`else sed -i '/^\[General\]/a AutoUpdate=%s' "$CONF"; fi`, xochitlConfPath, value, value)
}

type SystemTask struct {
	ID                   string
	Label                string
	Description          string
	DeviceTypes          []rmdevice.Type
	Command              func(ctx component.CommandContext) []component.CommandResult
	RequiresTerminal     bool
	NeedsWriteableRoot   bool
	WriteableRootBelowOS string
}

var SystemTasks = []SystemTask{
	{
		ID:          "enable-updates",
		Label:       "Enable Auto-Updates",
		Description: "Re-enable automatic software updates",
		Command: func(ctx component.CommandContext) []component.CommandResult {
			return []component.CommandResult{
				{
					Script:      setAutoUpdateScript(true),
					Description: "Allow auto-updates in xochitl settings",
				},
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
		RequiresTerminal:     true,
		NeedsWriteableRoot:   true,
		WriteableRootBelowOS: minEnforcedAutoUpdateVersion,
	},
	{
		ID:          "disable-updates",
		Label:       "Disable Auto-Updates",
		Description: "Prevent automatic software updates",
		Command: func(ctx component.CommandContext) []component.CommandResult {
			return []component.CommandResult{
				{
					Script:      setAutoUpdateScript(false),
					Description: "Turn off auto-updates in xochitl settings",
				},
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
		RequiresTerminal:     true,
		NeedsWriteableRoot:   true,
		WriteableRootBelowOS: minEnforcedAutoUpdateVersion,
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
