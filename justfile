# justfile for llm-local-proxy

# Set shell to PowerShell for all recipes
set shell := ["powershell.exe", "-c"]

# Default task: show available tasks
default:
  just --list

# Build binary for current platform
build:
  go build -o bin/llm-local-proxy.exe .

# Build for Windows
build-windows:
  $env:GOOS="windows"
  $env:GOARCH="amd64"
  go build -o dist/llm-local-proxy-windows-amd64.exe .
  $env:GOARCH="386"
  go build -o dist/llm-local-proxy-windows-386.exe .

# Build for Linux
build-linux:
  $env:GOOS="linux"
  $env:GOARCH="amd64"
  go build -o dist/llm-local-proxy-linux-amd64 .
  $env:GOARCH="arm64"
  go build -o dist/llm-local-proxy-linux-arm64 .

# Build for macOS
build-macos:
  $env:GOOS="darwin"
  $env:GOARCH="amd64"
  go build -o dist/llm-local-proxy-macos-amd64 .
  $env:GOARCH="arm64"
  go build -o dist/llm-local-proxy-macos-arm64 .

# Build for all platforms
build-all: build-windows build-linux build-macos

# Clean build artifacts
clean:
  if (Test-Path bin) { Remove-Item -Recurse -Force bin }
  if (Test-Path dist) { Remove-Item -Recurse -Force dist }

# Run locally
run:
  go run .

# Run with debug mode
run-debug:
  go run . -debug

# Test
test:
  go test ./...

# Format code
fmt:
  go fmt ./...
