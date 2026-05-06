//go:build linux

// Package pkgd exercises build-tag selection in the Layer-2 smoke test.
// pkgd_linux.go is selected on Linux; pkgd_darwin.go on macOS.
package pkgd

// Platform identifies the build platform that selected this file.
const Platform = "linux"
