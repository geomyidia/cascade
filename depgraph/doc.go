// Package depgraph builds a directed import graph from a slice of
// golist.Package values and exposes reverse-transitive closure traversal.
//
// All operations are pure — no io, no syscalls. Tests are table-driven
// over synthetic graphs.
//
// API stability: pre-v1.0, package surface may change. See repo README.
package depgraph
