module reManager

go 1.26.3

toolchain go1.26.8

require (
	github.com/Microsoft/go-winio v0.6.2
	github.com/google/uuid v1.6.0
	github.com/klauspost/compress v1.18.7
	github.com/pdfcpu/pdfcpu v0.15.0
	github.com/pkg/sftp v1.13.10
	github.com/rmitchellscott/remarkable-go v0.4.1
	github.com/rymdport/portal v0.4.3-0.20260225172009-01112360d2cb
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/wailsapp/wails/v3 v3.0.0-beta.26
	github.com/zalando/go-keyring v0.2.6
	golang.org/x/crypto v0.56.0
	gopkg.in/ini.v1 v1.67.1
)

require (
	github.com/google/btree v1.1.2 // indirect
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/time v0.15.0 // indirect
	gvisor.dev/gvisor v0.0.0-20261006185811-fdfbe30a6785
)

require (
	al.essio.dev/pkg/shellescape v1.6.0 // indirect
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/hhrutter/tiff v1.0.6 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.27 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/sys v0.47.0
	golang.org/x/text v0.41.0 // indirect
)

replace github.com/wailsapp/wails/v3 => ./third_party/wails-v3
