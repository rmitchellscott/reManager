package vellum

import "testing"

func testStore() *MetadataStore {
	pm := PackagesMetadata{
		Packages: map[string]map[string]PackageVersion{
			"koreader": {
				"1.0-r0": {Depends: []string{"launcher"}, Arch: []string{"aarch64"}},
			},
			"appload": {
				"0.5.3-r2": {Pkgdesc: "XOVI extension", Provides: []string{"launcher"}, Arch: []string{"aarch64"}},
			},
			"oxide": {
				"3.1-r1": {Pkgdesc: "Launcher application", Provides: []string{"launcher"}, Arch: []string{"aarch64"}},
			},
			"tilem": {
				"0.1.4-r3": {Depends: []string{"curl", "launcher"}, Arch: []string{"aarch64"}},
			},
			"solo": {
				"1.0-r0": {Depends: []string{"only-thing"}, Arch: []string{"aarch64"}},
			},
			"provider": {
				"1.0-r0": {Provides: []string{"only-thing"}, Arch: []string{"aarch64"}},
			},
		},
	}
	m := &MetadataStore{packages: pm, providers: buildProviderIndex(&pm), loaded: true}
	return m
}

func TestResolveVirtualChoices(t *testing.T) {
	m := testStore()

	choices := m.ResolveVirtualChoices([]string{"koreader"}, nil, "", "", "aarch64")
	if len(choices) != 1 {
		t.Fatalf("expected 1 choice, got %d (%+v)", len(choices), choices)
	}
	if choices[0].Virtual != "launcher" {
		t.Errorf("virtual = %q, want launcher", choices[0].Virtual)
	}
	if len(choices[0].Providers) != 2 {
		t.Errorf("providers = %+v, want appload and oxide", choices[0].Providers)
	}
	if len(choices[0].RequiredBy) != 1 || choices[0].RequiredBy[0] != "koreader" {
		t.Errorf("requiredBy = %v, want [koreader]", choices[0].RequiredBy)
	}
}

func TestResolveVirtualChoicesDefault(t *testing.T) {
	m := testStore()
	m.remanager = RemanagerMetadata{Virtuals: map[string]VirtualConfig{
		"launcher": {Default: "appload"},
	}}

	choices := m.ResolveVirtualChoices([]string{"koreader"}, nil, "", "", "aarch64")
	if len(choices) != 1 || choices[0].Default != "appload" {
		t.Fatalf("expected appload as default, got %+v", choices)
	}

	m.remanager = RemanagerMetadata{Virtuals: map[string]VirtualConfig{
		"launcher": {Default: "not-a-package"},
	}}
	choices = m.ResolveVirtualChoices([]string{"koreader"}, nil, "", "", "aarch64")
	if len(choices) != 1 || choices[0].Default != "" {
		t.Errorf("a default that is not an available provider should be dropped, got %+v", choices)
	}

	m.remanager = RemanagerMetadata{}
	choices = m.ResolveVirtualChoices([]string{"koreader"}, nil, "", "", "aarch64")
	if len(choices) != 1 || choices[0].Default != "" {
		t.Errorf("no configured default should leave it empty, got %+v", choices)
	}
}

func TestFallbackMetadataHasLauncherDefault(t *testing.T) {
	m := NewMetadataStore()
	if got := m.remanager.Virtuals["launcher"].Default; got != "appload" {
		t.Errorf("embedded fallback launcher default = %q, want appload", got)
	}
}

func TestResolveVirtualChoicesSatisfied(t *testing.T) {
	m := testStore()

	if choices := m.ResolveVirtualChoices([]string{"koreader"}, []string{"oxide"}, "", "", "aarch64"); len(choices) != 0 {
		t.Errorf("installed provider should suppress the prompt, got %+v", choices)
	}
	if choices := m.ResolveVirtualChoices([]string{"koreader", "appload"}, nil, "", "", "aarch64"); len(choices) != 0 {
		t.Errorf("queued provider should suppress the prompt, got %+v", choices)
	}
}

func TestResolveVirtualChoicesSingleProvider(t *testing.T) {
	m := testStore()

	choices := m.ResolveVirtualChoices([]string{"solo"}, nil, "", "", "aarch64")
	if len(choices) != 1 {
		t.Fatalf("a sole provider still has to be shown before it is installed, got %+v", choices)
	}
	if len(choices[0].Providers) != 1 || choices[0].Providers[0].Name != "provider" {
		t.Errorf("providers = %+v, want just provider", choices[0].Providers)
	}
}

func TestResolveVirtualChoicesIncompatibleProviderListed(t *testing.T) {
	m := testStore()
	max := "3.28"
	m.packages.Packages["appload"]["0.5.3-r2"] = PackageVersion{
		Pkgdesc:  "XOVI extension",
		Provides: []string{"launcher"},
		Arch:     []string{"aarch64"},
		OSMax:    &max,
	}
	m.providers = buildProviderIndex(&m.packages)
	m.remanager = RemanagerMetadata{Virtuals: map[string]VirtualConfig{"launcher": {Default: "appload"}}}

	choices := m.ResolveVirtualChoices([]string{"koreader"}, nil, "", "3.28.0.162", "aarch64")
	if len(choices) != 1 {
		t.Fatalf("expected the launcher choice, got %+v", choices)
	}

	byName := map[string]ProviderOption{}
	for _, p := range choices[0].Providers {
		byName[p.Name] = p
	}
	if p, ok := byName["appload"]; !ok || p.Compatible {
		t.Errorf("appload should be listed as incompatible, got %+v (present=%v)", p, ok)
	} else if p.IncompatibleReason != "os" {
		t.Errorf("appload incompatibleReason = %q, want os", p.IncompatibleReason)
	}
	if p := byName["oxide"]; !p.Compatible {
		t.Errorf("oxide should be compatible, got %+v", p)
	}
	if choices[0].Default != "appload" {
		t.Errorf("default = %q, want appload even when it is unavailable, so the UI can still label it", choices[0].Default)
	}
}

func TestResolveVirtualChoicesDedupesAcrossPackages(t *testing.T) {
	m := testStore()

	choices := m.ResolveVirtualChoices([]string{"koreader", "tilem"}, nil, "", "", "aarch64")
	if len(choices) != 1 {
		t.Fatalf("expected the launcher choice once, got %+v", choices)
	}
	if len(choices[0].RequiredBy) != 2 {
		t.Errorf("requiredBy = %v, want both packages", choices[0].RequiredBy)
	}
}

func TestDepsInstallableVirtual(t *testing.T) {
	m := testStore()

	if !m.depsInstallable("koreader", "", "", "aarch64", map[string]bool{}) {
		t.Error("a virtual dep with an installable provider should be installable")
	}
	if !m.depsInstallable("tilem", "", "", "aarch64", map[string]bool{}) {
		t.Error("an unknown dep name should not make a package uninstallable")
	}
}

func TestBuildProviderIndexIgnoresRealPackages(t *testing.T) {
	pm := PackagesMetadata{
		Packages: map[string]map[string]PackageVersion{
			"launcherctl":         {"1.0-r0": {}},
			"launcherctl-appload": {"1.0-r0": {Provides: []string{"launcherctl"}}},
			"launcherctl-oxide":   {"1.0-r0": {Provides: []string{"launcherctl"}}},
		},
	}
	if providers := buildProviderIndex(&pm)["launcherctl"]; providers != nil {
		t.Errorf("a real package is not virtual, got providers %v", providers)
	}
}
