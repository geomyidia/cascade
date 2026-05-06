// Package pkgb sits one step up from pkga in the sample module's
// import chain. Used by the golist Layer-2 smoke test.
package pkgb

import "example.test/sample/pkga"

// Hello prefixes pkga.Greet with a colon.
func Hello() string { return pkga.Greet() + "!" }
