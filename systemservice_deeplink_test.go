package main

import "testing"

func TestParseDeepLink(t *testing.T) {
	cases := []struct {
		name   string
		link   string
		target string
		pkg    string
	}{
		{"package link", "remanager://package/xovi", "package", "xovi"},
		{"trailing slash", "remanager://package/xovi/", "package", "xovi"},
		{"escaped name", "remanager://package/rm2-suspended-image%2Bextras", "package", "rm2-suspended-image+extras"},
		{"unknown host", "remanager://author/someone", "", ""},
		{"no name", "remanager://package", "", ""},
		{"another scheme", "https://index.vellum.delivery/package/xovi", "", ""},
		{"empty", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseDeepLink(c.link)
			if got.Target != c.target || got.Name != c.pkg {
				t.Errorf("parseDeepLink(%q) = %+v, want target %q name %q", c.link, got, c.target, c.pkg)
			}
		})
	}
}

func TestDeepLinkFromArgs(t *testing.T) {
	args := []string{"/Applications/reManager.app/Contents/MacOS/remanager", "remanager://package/xovi"}
	if got := deepLinkFromArgs(args); got != "remanager://package/xovi" {
		t.Errorf("deepLinkFromArgs(%v) = %q, want the link", args, got)
	}
	if got := deepLinkFromArgs([]string{"/usr/local/bin/remanager"}); got != "" {
		t.Errorf("deepLinkFromArgs with no link = %q, want empty", got)
	}
}

func TestTakeDeepLinkClearsWhatItReturns(t *testing.T) {
	app := &SystemService{core: &core{}}
	app.receiveDeepLink("remanager://package/xovi")

	if got := app.TakeDeepLink(); got.Name != "xovi" {
		t.Fatalf("TakeDeepLink() = %+v, want xovi", got)
	}
	if got := app.TakeDeepLink(); got.Target != "" {
		t.Errorf("second TakeDeepLink() = %+v, want empty", got)
	}
}

func TestReceiveDeepLinkIgnoresForeignSchemes(t *testing.T) {
	app := &SystemService{core: &core{}}
	app.receiveDeepLink("https://index.vellum.delivery/package/xovi")
	if got := app.TakeDeepLink(); got.Target != "" {
		t.Errorf("TakeDeepLink() = %+v, want empty", got)
	}
}
