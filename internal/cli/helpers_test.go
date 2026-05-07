package cli_test

import "os"

// writeAll writes data to path, creating or truncating. Test-only helper.
func writeAll(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
