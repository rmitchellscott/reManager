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
			Title:   "Rebuild Hashtable",
			Message: "This process may take up to 2 minutes. You will need to rebuild the hashtable again after every OS upgrade.",
			Steps: []string{
				"Restart the tablet interface",
				"Ask for your passcode (if you have one set)",
				"Generate hashtable (~1 minute)",
			},
			ConfirmText:       "Proceed",
			InProgressMessage: "Please enter your passcode on the tablet if prompted",
			PostCommandDialog: &component.DialogConfig{
				Title:   "Setup Complete",
				Message: "The hashtable has been rebuilt. The tablet interface is currently stopped. How would you like to start it?",
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
