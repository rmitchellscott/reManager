.PHONY: dev build build-all clean

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

# Development mode with hot reload
dev:
	wails dev

# Build for current platform
build:
	wails build $(LDFLAGS)

# Build for all platforms
build-all:
	wails build -platform darwin/amd64 $(LDFLAGS) -o reManager-darwin-amd64
	wails build -platform darwin/arm64 $(LDFLAGS) -o reManager-darwin-arm64
	wails build -platform linux/amd64 $(LDFLAGS) -o reManager-linux-amd64
	wails build -platform windows/amd64 $(LDFLAGS) -o reManager-windows-amd64.exe

# Build for macOS universal binary
build-darwin-universal:
	wails build -platform darwin/universal $(LDFLAGS)

# Clean build artifacts
clean:
	rm -rf build/bin
	rm -rf frontend/dist
