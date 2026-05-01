package vellum

import "reManager/internal/component"

type HookFunc func(ctx component.CommandContext) (*component.HookExecutionResult, error)

var hookRegistry = map[string]HookFunc{
	"rebuild_hashtable_dialog": RebuildHashtableDialog,
}

func GetHook(name string) HookFunc {
	return hookRegistry[name]
}

func RebuildHashtableDialog(ctx component.CommandContext) (*component.HookExecutionResult, error) {
	return &component.HookExecutionResult{
		DialogConfig: &component.DialogConfig{
			Title:             "Rebuild Hashtable",
			Message:           "The hashtable is a map mods use to identify interface components. It's separate from system files and required for UI mods.\n\nBuilding takes about a minute and your reMarkable's interface will restart.",
			Note:              "You'll need to rebuild after each OS update.\nYou can rebuild the hashtable from the Maintenance tab.",
			ConfirmText:       "Rebuild",
			CancelText:        "Cancel",
			InProgressMessage: "If you have a passcode, enter it on your reMarkable.",
			PostCommandDialog: &component.DialogConfig{
				Title:   "Setup Complete",
				Message: "The hashtable has been rebuilt. The reMarkable interface is currently stopped. How would you like to start it?",
				Actions: []component.DialogAction{
					{
						Id:    "start_with_mods",
						Label: "Start UI with Mods",
						Type:  "run_command",
						Value: "/home/root/xovi/start",
					},
					{
						Id:    "start_without_mods",
						Label: "Start UI without Mods",
						Type:  "run_command",
						Value: "systemctl start xochitl",
					},
				},
			},
		},
		Command: &component.CommandResult{
			Script:      "/home/root/xovi/rebuild_hashtable",
			Description: "Rebuild Qt resource hashtable",
		},
	}, nil
}
