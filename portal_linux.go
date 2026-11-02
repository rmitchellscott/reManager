//go:build !android

package main

import (
	"strings"

	"github.com/rymdport/portal/filechooser"
	"github.com/rymdport/portal/openuri"
	"github.com/rymdport/portal/settings"
	"github.com/rymdport/portal/settings/appearance"
)

func portalOpenFiles(title string, options portalOpenOptions) ([]string, error) {
	chooserOptions := &filechooser.OpenFileOptions{
		CurrentFolder: options.dir,
		Multiple:      options.multiple,
		Directory:     options.directory,
	}
	if len(options.filterPatterns) > 0 {
		rules := make([]filechooser.Rule, len(options.filterPatterns))
		for i, pattern := range options.filterPatterns {
			rules[i] = filechooser.Rule{Type: filechooser.GlobPattern, Pattern: pattern}
		}
		chooserOptions.Filters = []*filechooser.Filter{{Name: options.filterName, Rules: rules}}
	}
	files, err := filechooser.OpenFile("", title, chooserOptions)
	return trimFileURIs(files), err
}

func portalSaveFile(title, dir, filename string) ([]string, error) {
	files, err := filechooser.SaveFile("", title, &filechooser.SaveFileOptions{
		CurrentFolder: dir,
		CurrentName:   filename,
	})
	return trimFileURIs(files), err
}

func trimFileURIs(files []string) []string {
	for i, f := range files {
		files[i] = strings.TrimPrefix(f, "file://")
	}
	return files
}

func portalOpenDirectory(fd uintptr) error {
	return openuri.OpenDirectory("", fd, nil)
}

func watchSystemColorScheme(onChange func(ColorScheme)) {
	settings.OnSignalSettingChanged(func(changed settings.Changed) {
		if changed.Namespace == appearance.Namespace && changed.Key == "color-scheme" {
			scheme, err := appearance.ValueToColorScheme(changed.Value)
			if err == nil {
				onChange(colorSchemeName(scheme))
			}
		}
	})
}

func systemColorScheme() (ColorScheme, error) {
	scheme, err := appearance.GetColorScheme()
	if err != nil {
		return ColorSchemeUnknown, err
	}
	return colorSchemeName(scheme), nil
}

// The portal spec leaves "no preference" to the app's default, which is light.
// GNOME's Default appearance sends it, so it must not read as unknown.
func colorSchemeName(scheme appearance.ColorScheme) ColorScheme {
	if scheme == appearance.Dark {
		return ColorSchemeDark
	}
	return ColorSchemeLight
}
