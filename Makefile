.PHONY: dev build build-all build-windows-amd64 build-windows-arm64 clean flatpak flatpak-deps flatpak-install flatpak-docker-arm64 flatpak-docker-gnome49 flatpak-clean

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

# Build for Windows amd64 via Docker
build-windows-amd64:
	./scripts/build-windows.sh amd64

# Build for Windows ARM64 via Docker
build-windows-arm64:
	./scripts/build-windows.sh arm64

# Clean build artifacts
clean:
	rm -rf build/bin
	rm -rf frontend/dist

# Generate flatpak sources for offline builds (runs in Docker)
# NOTE: node_modules must not exist when running flatpak-node-generator or it skips packages
# See: https://github.com/flatpak/flatpak-builder-tools/issues/377
flatpak-deps:
	go mod vendor && cp vendor/modules.txt flatpak/vendor-modules.txt && rm -r vendor
	rm -rf frontend/node_modules
	docker run --rm -v "$(PWD)":/build -w /build python:3.12-slim bash -c \
		"pip install -q flatpak-node-generator && \
		flatpak-node-generator npm frontend/package-lock.json -o flatpak/node-sources.json"
	docker run --rm -v "$(PWD)":/build -w /build golang:1.25 bash -c \
		"go install github.com/dennwc/flatpak-go-mod@latest && \
		flatpak-go-mod . && \
		mv go.mod.yml flatpak/go-sources.yml && \
		sed -i 's|path: modules.txt|path: vendor-modules.txt|' flatpak/go-sources.yml && \
		rm modules.txt"

# Build flatpak (requires flatpak-builder)
flatpak:
	flatpak-builder --force-clean --install-deps-from=flathub build-dir flatpak/io.scottlabs.reManager.yml

# Build and install flatpak locally
flatpak-install:
	flatpak-builder --user --install --force-clean --install-deps-from=flathub build-dir flatpak/io.scottlabs.reManager.yml

# Build flatpak for arm64 via Docker
flatpak-docker-arm64:
	docker run --rm --platform linux/arm64 --privileged \
		-v "$(PWD)":/build -w /build fedora:41 bash -c \
		"dnf install -y flatpak flatpak-builder && \
		flatpak remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo && \
		flatpak-builder --repo=repo --install-deps-from=flathub --force-clean build-dir flatpak/io.scottlabs.reManager.yml && \
		flatpak build-bundle repo reManager.flatpak io.scottlabs.reManager"

# Build flatpak for GNOME 49 via Docker (Fedora 42 has GNOME 49)
flatpak-docker-gnome49:
	docker run --rm --privileged \
		-v "$(PWD)":/build -w /build fedora:42 bash -c \
		"dnf install -y flatpak flatpak-builder && \
		flatpak remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo && \
		flatpak-builder --repo=repo --install-deps-from=flathub --force-clean build-dir flatpak/io.scottlabs.reManager.yml && \
		flatpak build-bundle repo reManager-gnome49.flatpak io.scottlabs.reManager"

# Clean flatpak build artifacts
flatpak-clean:
	rm -rf build-dir .flatpak-builder repo reManager.flatpak reManager-gnome49.flatpak
