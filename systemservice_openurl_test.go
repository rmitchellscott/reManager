package main

import "testing"

func TestIsBrowsableURL(t *testing.T) {
	browsable := []string{
		"https://github.com/rmitchellscott/remanager",
		"http://example.com/path?q=1",
		"mailto:someone@example.com",
	}
	for _, u := range browsable {
		if !isBrowsableURL(u) {
			t.Errorf("rejected %q", u)
		}
	}

	rejected := []string{
		"file:///tmp/rm-audit/",
		"smb://host/share",
		`\\host\share\app.exe`,
		"/Applications/Calculator.app",
		"javascript:alert(1)",
		"https:///no-host",
		"ms-settings:",
		"mailto:",
		"",
	}
	for _, u := range rejected {
		if isBrowsableURL(u) {
			t.Errorf("accepted %q", u)
		}
	}
}
