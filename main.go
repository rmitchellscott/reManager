package main

import (
	"embed"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"reManager/internal/httputil"
)

//go:embed all:frontend/dist
var assets embed.FS

var version = "dev"

const eventFilesDropped = "files:dropped"

func init() {
	application.RegisterEvent[[]string](eventFilesDropped)
}

type services struct {
	connection *ConnectionService
	operations *OperationsService
	os         *OSService
	config     *ConfigService
	system     *SystemService
	shell      *ShellService
	device     *DeviceService
	packages   *PackagesService
	support    *SupportService
	files      *FilesService
}

func newServices(c *core) *services {
	s := &services{
		operations: &OperationsService{core: c},
		os:         &OSService{core: c},
		config:     &ConfigService{core: c},
		shell:      &ShellService{core: c},
		device:     &DeviceService{core: c},
	}
	s.system = &SystemService{core: c, os: s.os}
	s.packages = &PackagesService{core: c, os: s.os}
	s.support = &SupportService{core: c, system: s.system, os: s.os, device: s.device, packages: s.packages}
	s.files = &FilesService{core: c, config: s.config, device: s.device}
	s.connection = &ConnectionService{core: c, os: s.os, shell: s.shell, device: s.device, packages: s.packages, files: s.files}
	return s
}

func main() {
	httputil.Init(version)
	services := newServices(&core{})

	var window *application.WebviewWindow

	wailsApp := application.New(application.Options{
		Name:        "reManager",
		Description: "Manage mods on reMarkable tablets",
		// Wails starts services in this order and stops them in reverse; connection must be first so the logger and stores it opens are up for the others and are closed only after every other service has stopped.
		Services: []application.Service{
			application.NewService(services.connection),
			application.NewService(services.operations),
			application.NewService(services.os),
			application.NewService(services.config),
			application.NewService(services.system),
			application.NewService(services.shell),
			application.NewService(services.device),
			application.NewService(services.packages),
			application.NewService(services.support),
			application.NewService(services.files),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "io.scottlabs.reManager",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if link := deepLinkFromArgs(data.Args); link != "" {
					services.system.receiveDeepLink(link)
				}
				window.Restore()
				window.Show()
				window.Focus()
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Linux: application.LinuxOptions{
			ApplicationID: "io.scottlabs.reManager",
		},
		IOS: application.IOSOptions{
			DisableBounce: true,
		},
		Android: application.AndroidOptions{},
	})

	window = wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "reManager",
		Width:            900,
		Height:           800,
		MinWidth:         640,
		MinHeight:        600,
		BackgroundColour: application.NewRGB(250, 250, 250),
		URL:              "/",
		EnableFileDrop:   true,
	})

	// Wails v3 delivers drops to Go, not the page, and only for data-file-drop-target elements, so relay to the frontend.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		wailsApp.Event.Emit(eventFilesDropped, e.Context().DroppedFiles())
	})

	// A cold start carries the URL in argv (Windows, Linux) or a kAEGetURL Apple Event (macOS); wails reports both as this one event.
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
		services.system.receiveDeepLink(e.Context().URL())
	})
	listenForPlatformDeepLinks(wailsApp, services.system)
	listenForForegroundReturn(wailsApp, services.connection)
	registerDeepLinkScheme()

	if err := wailsApp.Run(); err != nil {
		println("Error:", err.Error())
	}
}
