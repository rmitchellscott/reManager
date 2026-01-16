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
			Message: "This process will restart the tablet interface twice and may take up to 2 minutes.",
			Steps: []string{
				"Restart the tablet interface",
				"Ask for your passcode (if you have one set)",
				"Generate hashtable (~1 minute)",
				"Restart the tablet interface again",
			},
			ConfirmText:       "Proceed",
			InProgressMessage: "Please enter your passcode on the tablet if prompted",
		},
		Command: &component.CommandResult{
			Script:      "/home/root/xovi/rebuild_hashtable",
			Description: "Rebuild Qt resource hashtable",
		},
	}, nil
}
