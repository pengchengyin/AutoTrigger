#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
RUNTIME_DIR="$PWD/../dist"
mkdir -p "$RUNTIME_DIR/test"
AT_GO_BINARY="${AT_GO_BINARY:-go}"
AT_RELEASE="${AT_RELEASE:-0}"
case "$($AT_GO_BINARY version)" in
  *go1.20.*) ;;
  *) echo "Go 1.20.x is required for Windows 7 support" >&2; exit 1 ;;
esac
export CGO_ENABLED=0
test -z "$($AT_GO_BINARY fmt ./...)"
$AT_GO_BINARY vet ./...
$AT_GO_BINARY test ./... -coverprofile="$RUNTIME_DIR/test/coverage.out"
$AT_GO_BINARY run ./tools/schema-gen
$AT_GO_BINARY run ./tools/icon-gen
if [ "$AT_RELEASE" = 1 ]; then
  $AT_GO_BINARY run ./tools/template-gen --masters masters --out internal/resources/assets/builtin/templates --release
fi
GOOS=windows GOARCH=amd64 $AT_GO_BINARY build -trimpath -ldflags='-s -w' -o "$RUNTIME_DIR/AutoTrigger.exe" ./cmd/autotrigger
GOOS=windows GOARCH=amd64 $AT_GO_BINARY vet ./...
GOOS=linux GOARCH=amd64 $AT_GO_BINARY build ./...
GOOS=darwin GOARCH=amd64 $AT_GO_BINARY build ./...
$AT_GO_BINARY run ./tools/pe-check "$RUNTIME_DIR/AutoTrigger.exe"
if [ "$AT_RELEASE" = 1 ]; then
  $AT_GO_BINARY run ./cmd/autotrigger --self-test templates
fi
if command -v golangci-lint >/dev/null 2>&1; then
  golangci-lint run
elif [ "$AT_RELEASE" = 1 ]; then
  echo "Release requires golangci-lint v1.54.x built with Go 1.20" >&2
  exit 1
fi
echo "Built $RUNTIME_DIR/AutoTrigger.exe" >&2
