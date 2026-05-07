// Package pkgc sits at the top of the sample module's import chain
// (it imports pkgb, which imports pkga). Has a companion test file.
package pkgc

import "example.test/sample/pkgb"

// Loud upper-cases the first character of pkgb.Hello.
func Loud() string {
	s := pkgb.Hello()
	if len(s) == 0 {
		return s
	}
	if c := s[0]; c >= 'a' && c <= 'z' {
		return string(c-'a'+'A') + s[1:]
	}
	return s
}
