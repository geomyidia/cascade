// Package golist is a thin io shell around `go list -deps -json`.
// It exposes typed Package values parsed from `go list`'s JSON output,
// without going through golang.org/x/tools/go/packages.
//
// This package is the only part of cascade that shells out to `go`.
//
// API stability: pre-v1.0, package surface may change. See repo README.
package golist
