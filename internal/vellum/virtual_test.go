package vellum

import "testing"

const unselectedVirtualError = `ERROR: unable to select packages:
  launcher (virtual):
    note: please select one of the 'provided by'
          packages explicitly
    provided by: appload-0.5.3-r3 oxide-3.1.1-r3
    required by: koreader-2026.07.1-r1[launcher]
`

const conflictError = `ERROR: unable to select packages:
  chessmarkable-0.8.1-r4:
    breaks: rmppure-1.0-r0[rmppure]
`

func TestParseUnselectedVirtuals(t *testing.T) {
	virtuals := ParseUnselectedVirtuals(unselectedVirtualError)
	if len(virtuals) != 1 || virtuals[0] != "launcher" {
		t.Fatalf("virtuals = %v, want [launcher]", virtuals)
	}
}

func TestParseUnselectedVirtualsIgnoresConflicts(t *testing.T) {
	if virtuals := ParseUnselectedVirtuals(conflictError); len(virtuals) != 0 {
		t.Fatalf("virtuals = %v, want none", virtuals)
	}
}

func TestParseResolutionConflictsIgnoresUnselectedVirtuals(t *testing.T) {
	if conflicts := ParseResolutionConflicts(unselectedVirtualError); len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", conflicts)
	}
}

func TestPreferredProviderPicksCompatibleDefault(t *testing.T) {
	choice := VirtualChoice{
		Virtual: "launcher",
		Default: "appload",
		Providers: []ProviderOption{
			{Name: "oxide", Compatible: true},
			{Name: "appload", Compatible: true},
		},
	}
	if provider := choice.PreferredProvider(); provider != "appload" {
		t.Fatalf("provider = %q, want appload", provider)
	}
}

func TestPreferredProviderSkipsIncompatibleDefault(t *testing.T) {
	choice := VirtualChoice{
		Virtual: "launcher",
		Default: "appload",
		Providers: []ProviderOption{
			{Name: "appload", Compatible: false, IncompatibleReason: "os"},
			{Name: "oxide", Compatible: true},
		},
	}
	if provider := choice.PreferredProvider(); provider != "oxide" {
		t.Fatalf("provider = %q, want oxide", provider)
	}
}

func TestPreferredProviderEmptyWhenNoneCompatible(t *testing.T) {
	choice := VirtualChoice{
		Virtual: "launcher",
		Default: "appload",
		Providers: []ProviderOption{
			{Name: "appload", Compatible: false},
			{Name: "oxide", Compatible: false},
		},
	}
	if provider := choice.PreferredProvider(); provider != "" {
		t.Fatalf("provider = %q, want empty", provider)
	}
}
