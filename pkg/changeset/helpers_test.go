package changeset_test

// Test fixtures use POSIX-style absolute paths (e.g. "/m/pkga"). CI is
// Linux-only; Windows-portability is documented as out-of-scope in the
// M4 spec's §"Risks & mitigations."

// stringSlicesEqual compares two []string treating nil and []string{} as
// equivalent. Avoids the reflect.DeepEqual nil-vs-empty asymmetry that
// would otherwise force every "empty result" case to write a literal nil
// even when production code returns nil naturally. Mirrors pkg/depgraph's
// helper of the same name.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
