package vellum

import "reManager/internal/executor"

// Compiled in rather than read from the hosted file: shipped apps hardcode which package owns each promoted command, so moving one in the hosted file would break them. Keyed by the installing package.
var maintenanceCommands = map[string][]MaintenanceCommand{
	"xovi-extensions": {
		{
			ID:               "start",
			Label:            "Start with mods",
			ProgressLabel:    "Starting with mods",
			Description:      "Start or restart xovi",
			Command:          executor.WithXochitlStartReset("/home/root/xovi/start"),
			RequiresTerminal: true,
		},
		{
			ID:               "stock",
			Label:            "Start without mods",
			ProgressLabel:    "Starting without mods",
			Description:      "Restart the reMarkable UI without mods",
			Command:          executor.WithXochitlStartReset("/home/root/xovi/stock"),
			RequiresTerminal: true,
		},
		{
			ID:               "debug",
			Label:            "Debug",
			ProgressLabel:    "Running xovi in debug mode",
			Description:      "Run xovi in debug mode for troubleshooting",
			Command:          "/home/root/xovi/debug",
			RequiresTerminal: true,
			ShowOutput:       true,
			AllowStop:        true,
		},
	},
	"qt-resource-rebuilder": {
		{
			ID:          "rebuild_hashtable",
			Label:       "Rebuild hashtable",
			Description: "Rebuild the Qt resource hashtable (needed after OS updates)",
			Command:     "/home/root/xovi/rebuild_hashtable",
			Hook:        "rebuild_hashtable_dialog",
		},
	},
	"launcherctl": {
		{
			ID:            "start-launcher",
			Label:         "Start",
			ProgressLabel: "Starting the launcher",
			Description:   "Start the current launcher",
			Command:       executor.WithXochitlStartReset("/home/root/.vellum/bin/launcherctl start-launcher"),
		},
		{
			ID:            "restart-launcher",
			Label:         "Restart",
			ProgressLabel: "Restarting the launcher",
			Description:   "Restart the current launcher",
			Command:       executor.WithXochitlStartReset("/home/root/.vellum/bin/launcherctl restart-launcher"),
		},
		{
			ID:            "stop-launcher",
			Label:         "Stop",
			ProgressLabel: "Stopping the launcher",
			Description:   "Stop the current launcher",
			Command:       "/home/root/.vellum/bin/launcherctl stop-launcher",
		},
		{
			ID:          "choose-launcher",
			Label:       "Switch",
			Description: "Pick which launcher starts on your reMarkable",
			Hook:        "launcher_select",
		},
		{
			ID:            "launcher-status",
			Label:         "Status",
			ProgressLabel: "Checking the launcher",
			Description:   "Show the current launcher and its running apps",
			Command:       "/home/root/.vellum/bin/launcherctl status",
			ShowOutput:    true,
		},
	},
}
