# justfile for ds-proxy - Task runner for building and releasing

# Set shell to PowerShell for all recipes
set shell := ["powershell.exe", "-c"]

# Default task: show available tasks
default:
  just --list

# Build binary for current platform
build:
  go build -o bin/ds-proxy main.go

# Build for Windows
build-windows:
  $env:GOOS="windows"
  $env:GOARCH="amd64"
  go build -o dist/ds-proxy-windows-amd64.exe main.go
  $env:GOARCH="386"
  go build -o dist/ds-proxy-windows-386.exe main.go

# Build for Linux
build-linux:
  $env:GOOS="linux"
  $env:GOARCH="amd64"
  go build -o dist/ds-proxy-linux-amd64 main.go
  $env:GOARCH="arm64"
  go build -o dist/ds-proxy-linux-arm64 main.go

# Build for macOS
build-macos:
  $env:GOOS="darwin"
  $env:GOARCH="amd64"
  go build -o dist/ds-proxy-macos-amd64 main.go
  $env:GOARCH="arm64"
  go build -o dist/ds-proxy-macos-arm64 main.go

# Build for all platforms
build-all:
  $env:GOOS="windows"
  $env:GOARCH="amd64"
  go build -o dist/ds-proxy-windows-amd64.exe main.go
  $env:GOARCH="386"
  go build -o dist/ds-proxy-windows-386.exe main.go
  
  $env:GOOS="linux"
  $env:GOARCH="amd64"
  go build -o dist/ds-proxy-linux-amd64 main.go
  $env:GOARCH="arm64"
  go build -o dist/ds-proxy-linux-arm64 main.go
  
  $env:GOOS="darwin"
  $env:GOARCH="amd64"
  go build -o dist/ds-proxy-macos-amd64 main.go
  $env:GOARCH="arm64"
  go build -o dist/ds-proxy-macos-arm64 main.go

# Clean build artifacts
clean:
  if (Test-Path bin) { Remove-Item -Recurse -Force bin }
  if (Test-Path dist) { Remove-Item -Recurse -Force dist }

# Run locally
run:
  go run main.go

# Run with debug mode
run-debug:
  go run main.go -debug

# Build all platforms binaries (for release)
release:
  # Build for all platforms
  $env:GOOS="windows"
  $env:GOARCH="amd64"
  go build -o dist/ds-proxy-windows-amd64.exe main.go
  $env:GOARCH="386"
  go build -o dist/ds-proxy-windows-386.exe main.go
  
  $env:GOOS="linux"
  $env:GOARCH="amd64"
  go build -o dist/ds-proxy-linux-amd64 main.go
  $env:GOARCH="arm64"
  go build -o dist/ds-proxy-linux-arm64 main.go
  
  $env:GOOS="darwin"
  $env:GOARCH="amd64"
  go build -o dist/ds-proxy-macos-amd64 main.go
  $env:GOARCH="arm64"
  go build -o dist/ds-proxy-macos-arm64 main.go
  
  echo "✅ All release binaries built"
  echo "📦 Files in dist/:"
  echo "   - ds-proxy-windows-amd64.exe"
  echo "   - ds-proxy-windows-386.exe"
  echo "   - ds-proxy-linux-amd64"
  echo "   - ds-proxy-linux-arm64"
  echo "   - ds-proxy-macos-amd64"
  echo "   - ds-proxy-macos-arm64"

# Test compilation
test:
  go test ./...

# Format code
fmt:
  go fmt ./...

# Show help
help:
  @echo "Available tasks:"
  @echo "  build        - Build for current platform"
  @echo "  build-windows - Build Windows binaries (amd64, 386)"
  @echo "  build-linux   - Build Linux binaries (amd64, arm64)"
  @echo "  build-macos   - Build macOS binaries (amd64, arm64)"
  @echo "  build-all    - Build for all platforms"
  @echo "  clean        - Remove build artifacts"
  @echo "  run          - Run locally"
  @echo "  run-debug    - Run with debug mode"
  @echo "  release      - Build binaries for all platforms (no zip)"
  @echo "  test         - Run tests"
  @echo "  fmt          - Format Go code"