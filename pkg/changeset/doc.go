// Package changeset maps a list of changed file paths into the set of
// import paths whose packages those files belong to.
//
// Operations are deterministic and table-test-friendly. The single io edge
// — a default os.Getwd fallback when WithModuleRoot is not supplied — is
// hooked through an internal function-variable seam so tests can be made
// fully pure by passing WithModuleRoot explicitly.
//
// API stability: pre-v1.0, package surface may change. See repo README.
package changeset
