package vellum

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseConstraint(t *testing.T) {
	tests := []struct {
		entry   string
		name    string
		op      string
		version string
	}{
		{"tripletap", "tripletap", "", ""},
		{"xovi-extensions<18.0.0", "xovi-extensions", "<", "18.0.0"},
		{"qt-resource-rebuilder<18.0.0", "qt-resource-rebuilder", "<", "18.0.0"},
		{"liboxide=3.1-r1", "liboxide", "=", "3.1-r1"},
		{"appload>=0.5.0", "appload", ">=", "0.5.0"},
		{"foo<=2.0", "foo", "<=", "2.0"},
		{"foo>1.0", "foo", ">", "1.0"},
	}

	for _, tt := range tests {
		got := ParseConstraint(tt.entry)
		if got.Name != tt.name || got.Op != tt.op || got.Version != tt.version {
			t.Errorf("ParseConstraint(%q) = {%q %q %q}, want {%q %q %q}",
				tt.entry, got.Name, got.Op, got.Version, tt.name, tt.op, tt.version)
		}
	}
}

func TestProhibitsRealMetadataConflicts(t *testing.T) {
	tests := []struct {
		entry   string
		version string
		want    bool
	}{
		{"xovi-extensions<18.0.0", "16.0.0-r2", true},
		{"xovi-extensions<18.0.0", "17.0.0-r5", true},
		{"xovi-extensions<18.0.0", "18.0.0-r6", false},
		{"xovi-extensions<18.0.0", "19.0.0-r3", false},

		{"qt-resource-rebuilder<18.0.0", "16.0.0-r2", true},
		{"qt-resource-rebuilder<18.0.0", "17.0.0-r5", true},
		{"qt-resource-rebuilder<18.0.0", "18.0.0-r6", false},
		{"qt-resource-rebuilder<18.0.0", "19.0.0-r3", false},
	}

	for _, tt := range tests {
		if got := ParseConstraint(tt.entry).Prohibits(tt.version); got != tt.want {
			t.Errorf("ParseConstraint(%q).Prohibits(%q) = %v, want %v",
				tt.entry, tt.version, got, tt.want)
		}
	}
}

func TestProhibitsBareNameMatchesEveryVersion(t *testing.T) {
	c := ParseConstraint("tripletap")
	for _, version := range []string{"1.0.0-r4", "1.1.0-r0", "2.0.0-r0", ""} {
		if !c.Prohibits(version) {
			t.Errorf("bare name should prohibit %q", version)
		}
	}
}

func TestProhibitsFailsOpen(t *testing.T) {
	if ParseConstraint("tripletap<2.0").Prohibits("") {
		t.Error("unknown version should not be prohibited")
	}
	if (VersionConstraint{Name: "foo", Op: "~="}).Prohibits("1.0") {
		t.Error("unrecognized operator should not prohibit")
	}
	if (VersionConstraint{Name: "foo", Op: "<"}).Prohibits("1.0") {
		t.Error("missing constraint version should not prohibit")
	}
}

func TestProhibitsVersionedTripletapConflict(t *testing.T) {
	c := ParseConstraint("tripletap<2.0")
	if !c.Prohibits("1.1.0-r0") {
		t.Error("tripletap 1.1.0-r0 should conflict with tripletap<2.0")
	}
	if c.Prohibits("2.0.0-r0") {
		t.Error("tripletap 2.0.0-r0 should not conflict with tripletap<2.0")
	}
}

func TestStripDepVersionUnchanged(t *testing.T) {
	tests := map[string]string{
		"liboxide=3.1-r1":        "liboxide",
		"oxide-utils=3.1-r1":     "oxide-utils",
		"xovi-extensions<18.0.0": "xovi-extensions",
		"lockscreen_hook":        "lockscreen_hook",
		"/bin/sh":                "/bin/sh",
	}

	for dep, want := range tests {
		if got := stripDepVersion(dep); got != want {
			t.Errorf("stripDepVersion(%q) = %q, want %q", dep, got, want)
		}
	}
}

func conflictStore() *MetadataStore {
	pm := PackagesMetadata{
		Packages: map[string]map[string]PackageVersion{
			"xovi": {
				"0.3.1-r1": {Arch: []string{"aarch64"}},
				"0.3.3-r2": {Conflicts: []string{"xovi-extensions<18.0.0"}, Arch: []string{"aarch64"}},
			},
			"xovi-extensions": {
				"17.0.0-r5": {Arch: []string{"aarch64"}},
				"19.0.0-r3": {Conflicts: []string{"qt-resource-rebuilder<18.0.0"}, Arch: []string{"aarch64"}},
			},
			"qt-resource-rebuilder": {
				"17.0.0-r5": {Arch: []string{"aarch64"}},
				"19.0.0-r3": {Arch: []string{"aarch64"}},
			},
			"tripletap": {
				"1.1.0-r0": {Conflicts: []string{"launcherctl"}, Arch: []string{"aarch64"}},
			},
			"launcherctl": {
				"3.1-r1": {Conflicts: []string{"tripletap"}, Arch: []string{"aarch64"}},
			},
		},
	}
	return &MetadataStore{packages: pm, providers: buildProviderIndex(&pm), loaded: true}
}

func TestFindConflictsVersionedForwardDirection(t *testing.T) {
	m := conflictStore()

	conflicts := m.FindConflicts(map[string]string{"xovi-extensions": "17.0.0-r5"}, nil, "", "", "aarch64")
	if _, found := conflicts["xovi"]; !found {
		t.Error("xovi should conflict with installed xovi-extensions 17.0.0-r5")
	}

	conflicts = m.FindConflicts(map[string]string{"xovi-extensions": "19.0.0-r3"}, nil, "", "", "aarch64")
	if c, found := conflicts["xovi"]; found {
		t.Errorf("xovi should not conflict with xovi-extensions 19.0.0-r3, got %+v", c)
	}
}

func TestFindConflictsVersionedReverseDirection(t *testing.T) {
	m := conflictStore()

	conflicts := m.FindConflicts(map[string]string{"xovi-extensions": "19.0.0-r3"}, nil, "", "", "aarch64")
	if c, found := conflicts["qt-resource-rebuilder"]; found {
		t.Errorf("qt-resource-rebuilder best version 19.0.0-r3 is not prohibited, got %+v", c)
	}
}

func oldExtensionsStore() *MetadataStore {
	pm := PackagesMetadata{
		Packages: map[string]map[string]PackageVersion{
			"xovi": {
				"0.3.1-r1": {Arch: []string{"aarch64"}},
				"0.3.3-r2": {Conflicts: []string{"xovi-extensions<18.0.0"}, Arch: []string{"aarch64"}},
			},
			"xovi-extensions": {
				"17.0.0-r5": {Arch: []string{"aarch64"}},
			},
		},
	}
	return &MetadataStore{packages: pm, providers: buildProviderIndex(&pm), loaded: true}
}

func TestFindConflictsUsesInstalledVersionNotLatest(t *testing.T) {
	m := oldExtensionsStore()

	conflicts := m.FindConflicts(map[string]string{"xovi": "0.3.3-r2"}, nil, "", "", "aarch64")
	if _, found := conflicts["xovi-extensions"]; !found {
		t.Error("xovi 0.3.3-r2 conflicts should apply to xovi-extensions 17.0.0-r5")
	}

	conflicts = m.FindConflicts(map[string]string{"xovi": "0.3.1-r1"}, nil, "", "", "aarch64")
	if c, found := conflicts["xovi-extensions"]; found {
		t.Errorf("xovi 0.3.1-r1 declares no conflicts, got %+v", c)
	}
}

func TestFindConflictsAllowsNewerUnprohibitedVersion(t *testing.T) {
	m := conflictStore()

	conflicts := m.FindConflicts(map[string]string{"xovi": "0.3.3-r2"}, nil, "", "", "aarch64")
	if c, found := conflicts["xovi-extensions"]; found {
		t.Errorf("xovi-extensions best version 19.0.0-r3 is outside xovi-extensions<18.0.0, got %+v", c)
	}
}

func TestFindConflictsAgainstQueue(t *testing.T) {
	m := conflictStore()

	conflicts := m.FindConflicts(nil, []string{"launcherctl"}, "", "", "aarch64")
	c, found := conflicts["tripletap"]
	if !found {
		t.Fatal("tripletap should conflict with queued launcherctl")
	}
	if c.ConflictsWith != "launcherctl" || !c.Queued {
		t.Errorf("got %+v, want launcherctl queued", c)
	}
}

func TestFindConflictsMarksInstalledVersusQueued(t *testing.T) {
	m := conflictStore()

	conflicts := m.FindConflicts(map[string]string{"launcherctl": "3.1-r1"}, nil, "", "", "aarch64")
	c := conflicts["tripletap"]
	if c.ConflictsWith != "launcherctl" || c.Queued {
		t.Errorf("got %+v, want launcherctl installed", c)
	}
}

func TestFindConflictsSkipsPresentPackages(t *testing.T) {
	m := conflictStore()

	conflicts := m.FindConflicts(map[string]string{"launcherctl": "3.1-r1", "tripletap": "1.1.0-r0"}, nil, "", "", "aarch64")
	if _, found := conflicts["tripletap"]; found {
		t.Error("already-installed packages should not be reported as conflicts")
	}
}

func TestResolutionFailureExtractsConflict(t *testing.T) {
	err := fmt.Errorf(`ERROR: unable to select packages:
  tripletap-1.1.0-r0:
    breaks: launcherctl-3.1-r1[!tripletap]
    satisfies: world[tripletap=1.1.0-r0]`)

	got := ResolutionFailure(err)
	if !strings.Contains(got, "tripletap-1.1.0-r0") || !strings.Contains(got, "breaks: launcherctl-3.1-r1") {
		t.Errorf("conflict detail missing from %q", got)
	}
	if strings.Contains(got, "satisfies: world[") {
		t.Errorf("world noise should be dropped, got %q", got)
	}
	if strings.HasPrefix(got, "ERROR: ") {
		t.Errorf("ERROR prefix should be stripped, got %q", got)
	}
}

func TestResolutionFailureIgnoresTransportErrors(t *testing.T) {
	for _, err := range []error{
		nil,
		fmt.Errorf("ssh: connection lost"),
		fmt.Errorf("exit status 1"),
		fmt.Errorf("context deadline exceeded"),
	} {
		if got := ResolutionFailure(err); got != "" {
			t.Errorf("ResolutionFailure(%v) = %q, want empty", err, got)
		}
	}
}

func TestParseResolutionConflict(t *testing.T) {
	message := `unable to select packages:
  tripletap-1.1.0-r0:
    breaks: launcherctl-3.1-r1[!tripletap]`

	conflicts := ParseResolutionConflicts(message)
	if len(conflicts) != 1 {
		t.Fatalf("expected one conflict, got %+v", conflicts)
	}
	if conflicts[0].Package != "tripletap" || conflicts[0].Breaks != "launcherctl" {
		t.Errorf("got %+v, want tripletap / launcherctl", conflicts[0])
	}
}

func TestParseResolutionConflictHandlesDashedNames(t *testing.T) {
	message := `unable to select packages:
  launcherctl-oxide-3.1-r1:
    breaks: some-other-pkg-1.2.0-r3[!launcherctl-oxide]`

	conflicts := ParseResolutionConflicts(message)
	if len(conflicts) != 1 {
		t.Fatalf("expected one conflict, got %+v", conflicts)
	}
	if conflicts[0].Package != "launcherctl-oxide" || conflicts[0].Breaks != "some-other-pkg" {
		t.Errorf("got %+v, want launcherctl-oxide / some-other-pkg", conflicts[0])
	}
}

func TestParseResolutionConflictIgnoresUnrelated(t *testing.T) {
	if got := ParseResolutionConflicts("some unrelated failure"); len(got) != 0 {
		t.Errorf("unrelated output should not parse as a conflict, got %+v", got)
	}
}

func TestAttributeConflictToQueueMapsDependencyToItsQueueEntry(t *testing.T) {
	pm := PackagesMetadata{
		Packages: map[string]map[string]PackageVersion{
			"tripletap":         {"1.1.0-r0": {Arch: []string{"aarch64"}}},
			"launcherctl":       {"3.1-r1": {Arch: []string{"aarch64"}}},
			"launcherctl-oxide": {"3.1-r1": {Depends: []string{"oxide=3.1-r1", "launcherctl"}, Arch: []string{"aarch64"}}},
		},
	}
	m := &MetadataStore{packages: pm, providers: buildProviderIndex(&pm), loaded: true}

	conflicts := []ResolutionConflict{{Package: "tripletap", Breaks: "launcherctl"}}
	entries := m.AttributeConflictsToQueue(conflicts, []string{"launcherctl-oxide", "tripletap"}, "", "", "aarch64")

	if len(entries) != 2 {
		t.Fatalf("expected both sides attributed, got %+v", entries)
	}

	byEntry := map[string]QueueConflictEntry{}
	for _, e := range entries {
		byEntry[e.QueueEntry] = e
	}

	if byEntry["tripletap"].Package != "tripletap" {
		t.Errorf("tripletap should map to itself, got %+v", byEntry["tripletap"])
	}
	if byEntry["launcherctl-oxide"].Package != "launcherctl" {
		t.Errorf("launcherctl should map to launcherctl-oxide, got %+v", byEntry["launcherctl-oxide"])
	}
}

func TestParseResolutionConflictsCollapsesBidirectionalPairs(t *testing.T) {
	message := `unable to select packages:
  createpages-rm2-1.0.4-r3:
    breaks: createpages-paperpro-1.0.4-r3[!createpages-rm2]
  createpages-paperpro-1.0.4-r3:
    breaks: createpages-rm2-1.0.4-r3[!createpages-paperpro]
  quicksheet-use-template-1.0.1-r3:
    breaks: default-template-1.0.0-r0[!quicksheet-use-template]
  fav-tag-button-1.0.7-r2:
    breaks: dock-buttons-1.0.0-r0[!fav-tag-button]`

	conflicts := ParseResolutionConflicts(message)
	if len(conflicts) != 3 {
		t.Fatalf("expected 3 distinct conflicts, got %d: %+v", len(conflicts), conflicts)
	}

	pairs := map[string]bool{}
	for _, c := range conflicts {
		pairs[c.Package+"/"+c.Breaks] = true
	}
	if !pairs["createpages-rm2/createpages-paperpro"] {
		t.Errorf("createpages pair missing or reversed: %+v", conflicts)
	}
	if !pairs["quicksheet-use-template/default-template"] || !pairs["fav-tag-button/dock-buttons"] {
		t.Errorf("later conflicts dropped: %+v", conflicts)
	}
}

func TestParseResolutionConflictsWithSatisfiesContinuationLines(t *testing.T) {
	message := `unable to select packages:
  gestures-fouzr-1.0.11-r2:
    breaks: gestik-1.1.2-r0[!gestures-fouzr]
    satisfies: world[gestures-fouzr=1.0.11-r2]
               gesture-colour-settings-1.0.10-r2[gestures-fouzr]
  tripletap-1.1.0-r0:
    breaks: launcherctl-3.1-r1[!tripletap]
    satisfies: world[tripletap=1.1.0-r0]`

	conflicts := ParseResolutionConflicts(message)
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d: %+v", len(conflicts), conflicts)
	}
	if conflicts[0].Package != "gestures-fouzr" || conflicts[0].Breaks != "gestik" {
		t.Errorf("first conflict wrong: %+v", conflicts[0])
	}
	if conflicts[1].Package != "tripletap" || conflicts[1].Breaks != "launcherctl" {
		t.Errorf("second conflict wrong: %+v", conflicts[1])
	}
}
