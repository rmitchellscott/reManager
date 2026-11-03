package main

import (
	"os"
	"strings"

	"reManager/internal/platform"
)

type hostPlatform struct {
	OS      string
	Version string
	Model   string
}

func (s *SupportService) detectHostPlatform() hostPlatform {
	env := s.wails.Env.Info()
	host := hostPlatform{OS: env.OS}

	switch env.OS {
	case "darwin":
		host.OS = "macos"
		if env.OSInfo != nil {
			host.Version = env.OSInfo.Version
		}
	case "ios":
		systemName, _ := env.PlatformInfo["systemName"].(string)
		host.Version, _ = env.PlatformInfo["systemVersion"].(string)
		host.Model, _ = env.PlatformInfo["model"].(string)
		if strings.EqualFold(systemName, "iPadOS") || strings.HasPrefix(host.Model, "iPad") {
			host.OS = "ipados"
		}
	case "android":
		host.Version, _ = env.PlatformInfo["version"].(string)
		manufacturer, _ := env.PlatformInfo["manufacturer"].(string)
		model, _ := env.PlatformInfo["model"].(string)
		host.Model = strings.TrimSpace(manufacturer + " " + model)
	case "windows":
		if env.OSInfo != nil {
			host.Version = strings.TrimSpace(env.OSInfo.Branding + " " + env.OSInfo.ID)
		}
	case "linux":
		host.Version = linuxDistributionName()
	}

	return host
}

// Inside a Flatpak, /etc/os-release describes the Flatpak runtime rather than the host distribution.
func linuxDistributionName() string {
	osReleasePath := "/etc/os-release"
	if platform.IsRunningInFlatpak() {
		osReleasePath = "/run/host/os-release"
	}
	data, err := os.ReadFile(osReleasePath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if prettyName, found := strings.CutPrefix(line, "PRETTY_NAME="); found {
			return strings.Trim(prettyName, `"`)
		}
	}
	return ""
}
