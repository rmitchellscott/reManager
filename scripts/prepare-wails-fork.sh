#!/usr/bin/env bash
# Materializes third_party/wails-v3 from the module cache and applies the
# patches in build/wails-patches. go.mod replaces the module with this
# directory, so the tree must exist before any build.
set -euo pipefail

cd "$(dirname "$0")/.."

version=$(awk '/github.com\/wailsapp\/wails\/v3 v/ {print $2; exit}' go.mod)
go mod download "github.com/wailsapp/wails/v3@${version}"
source_dir="$(go env GOMODCACHE)/github.com/wailsapp/wails/v3@${version}"

rm -rf third_party/wails-v3
mkdir -p third_party
cp -R "$source_dir" third_party/wails-v3
chmod -R u+w third_party/wails-v3

for patch in build/wails-patches/*.patch; do
	patch -p1 -d third_party/wails-v3 < "$patch"
done

echo "third_party/wails-v3 ready at ${version}"
