// Package changeset maps a list of changed file paths into the set of
// import paths whose packages those files belong to.
//
// All operations are pure — no io. Tests are table-driven.
//
// API stability: pre-v1.0, package surface may change. See repo README.
package changeset
