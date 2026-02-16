package vellum

import "reManager/internal/component"

type HookFunc func(ctx component.CommandContext) (*component.HookExecutionResult, error)

var hookRegistry = map[string]HookFunc{
	"rebuild_hashtable_dialog":  RebuildHashtableDialog,
	"xovi_post_install_info":    XoviPostInstallInfo,
}

func GetHook(name string) HookFunc {
	return hookRegistry[name]
}

func XoviPostInstallInfo(ctx component.CommandContext) (*component.HookExecutionResult, error) {
	return &component.HookExecutionResult{
		DialogConfig: &component.DialogConfig{
			Title:   "Xovi Does Not Auto-Start",
			Message: "For technical reasons, xovi does not auto-start after reboots. Each time your device restarts, you will need to manually start xovi using one of the following methods:",
			Steps: []string{
				"(Recommended) Install the tripletap package, then triple-press the power button on every boot",
				"Use the xovi Start maintenance command in reManager",
				"Connect via SSH and run: /home/root/xovi/start",
			},
			InfoOnly:    true,
			ConfirmText: "Got it",
		},
	}, nil
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
