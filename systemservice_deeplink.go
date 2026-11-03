package main

import (
	"net/url"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"reManager/internal/debug"
)

const deepLinkScheme = "remanager"

const eventDeepLinkReceived = "deeplink:received"

func init() {
	application.RegisterEvent[application.Void](eventDeepLinkReceived)
}

// DeepLink is a resolved remanager:// URL; an empty Target is how the frontend tells "no link" from a link it can't act on yet.
type DeepLink struct {
	Target string `json:"target"`
	Name   string `json:"name"`
}

// A deep link can arrive before the webview has mounted its listeners, so it is held for the frontend to take rather than emitted and lost.
func (s *SystemService) receiveDeepLink(link string) {
	resolved := parseDeepLink(link)
	if resolved.Target == "" {
		return
	}
	debug.Println("[DEBUG] deep link received:", link)
	s.deepLinkMu.Lock()
	s.pendingDeepLink = link
	s.deepLinkMu.Unlock()
	if s.wails != nil {
		s.wails.Event.Emit(eventDeepLinkReceived)
	}
}

func (s *SystemService) TakeDeepLink() DeepLink {
	s.deepLinkMu.Lock()
	link := s.pendingDeepLink
	s.pendingDeepLink = ""
	s.deepLinkMu.Unlock()
	if link != "" {
		debug.Println("[DEBUG] deep link handed to the frontend:", link)
	}
	return parseDeepLink(link)
}

// The link path is the website's route verbatim: remanager://package/<name>.
func parseDeepLink(link string) DeepLink {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != deepLinkScheme {
		return DeepLink{}
	}
	name, err := url.PathUnescape(strings.Trim(parsed.Path, "/"))
	if err != nil || name == "" {
		return DeepLink{}
	}
	if parsed.Host != "package" {
		return DeepLink{}
	}
	return DeepLink{Target: parsed.Host, Name: name}
}

// A second launch arrives as argv; on macOS the URL is not in argv -- wails appends it from the kAEGetURL Apple Event.
func deepLinkFromArgs(args []string) string {
	for _, arg := range args {
		if parseDeepLink(arg).Target != "" {
			return arg
		}
	}
	return ""
}
