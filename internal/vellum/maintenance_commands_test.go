package vellum

import "testing"

// The xovi scripts ship in xovi-extensions, and the hosted metadata cannot move or remove the promoted commands the app looks up by name.
func TestMaintenanceCommandsComeFromTheCompiledTable(t *testing.T) {
	m := NewMetadataStore()
	m.remanager = RemanagerMetadata{}

	ids := func(name string) map[string]bool {
		out := map[string]bool{}
		for _, command := range m.GetMaintenanceCommands(name) {
			out[command.ID] = true
		}
		return out
	}

	for _, id := range []string{"start", "stock", "debug"} {
		if !ids("xovi-extensions")[id] {
			t.Errorf("xovi-extensions is missing %q", id)
		}
	}
	if len(ids("xovi")) != 0 {
		t.Errorf("xovi should declare no commands, got %v", ids("xovi"))
	}
	if !ids("qt-resource-rebuilder")["rebuild_hashtable"] {
		t.Error("qt-resource-rebuilder is missing rebuild_hashtable")
	}
	for _, id := range []string{"start-launcher", "stop-launcher"} {
		if !ids("launcherctl")[id] {
			t.Errorf("launcherctl is missing %q", id)
		}
	}
}
