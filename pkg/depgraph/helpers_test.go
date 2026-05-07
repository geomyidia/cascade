package depgraph_test

import (
	"testing"

	"github.com/geomyidia/cascade/pkg/depgraph"
	"github.com/geomyidia/cascade/pkg/golist"
)

// buildTestGraph constructs a Graph from a compact map representation
// for tests. Keys are import paths (each becomes a node); values are
// direct imports merged into Imports per the Build contract. A nil or
// empty edge map yields an empty graph. TestImports and XTestImports
// are exercised via dedicated tests using golist.Package literals
// directly, not this helper.
func buildTestGraph(t *testing.T, edges map[string][]string) *depgraph.Graph {
	t.Helper()
	pkgs := make([]golist.Package, 0, len(edges))
	for path, imports := range edges {
		pkgs = append(pkgs, golist.Package{
			ImportPath: path,
			Imports:    imports,
		})
	}
	return depgraph.Build(pkgs)
}

// stringSlicesEqual compares two []string treating nil and []string{}
// as equivalent. Avoids the reflect.DeepEqual nil-vs-empty asymmetry
// that would otherwise force every "empty result" test case to write
// a literal nil even when the production code returns nil naturally.
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
