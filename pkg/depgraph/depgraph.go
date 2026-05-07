package depgraph

import (
	"sort"

	"github.com/geomyidia/cascade/pkg/golist"
)

// Graph is a directed import graph constructed from a slice of
// golist.Package values. Edges run importer → imported, mirroring
// `go list -deps`'s view. The Graph also stores reverse edges
// internally so RevDepClosure runs in O(V + E) without rebuilding.
//
// A Graph is immutable after Build returns. Methods are safe for
// concurrent reads from multiple goroutines.
//
// API stability: pre-v1.0, the Graph type is opaque (no exported
// fields, no exported methods beyond those documented). Internal
// representation may change without notice.
type Graph struct {
	// nodes is the set of import paths the graph knows about. A path
	// is a node iff it appeared as a Package.ImportPath in Build's
	// input. Paths that appear only as edge targets (e.g. imports of
	// packages not in the input) are NOT nodes.
	nodes map[string]struct{}

	// forward[p] is the sorted, deduplicated slice of import paths that
	// p directly imports (the union of Imports + TestImports + XTestImports
	// from p's golist.Package).
	forward map[string][]string

	// reverse[p] is the sorted, deduplicated slice of import paths that
	// directly import p (the inverse of forward).
	reverse map[string][]string

	// stats is computed once at Build time and returned by Stats.
	stats Stats
}

// Build constructs a Graph from the given slice of packages.
//
// Edge semantics: for each Package P, the union of P.Imports +
// P.TestImports + P.XTestImports forms P's outgoing-edge set. This
// merging reflects affected-set intent — if a package's tests import
// X and X changes, the package needs re-testing.
//
// Build never fails. Empty input yields an empty Graph (zero nodes,
// zero edges). Duplicate ImportPath entries are treated as a single
// node, with the last entry's edge set winning.
//
// Build does not validate that every imported path appears as a node
// in pkgs. Edges to absent nodes are recorded in the forward map but
// don't add nodes — Has on an absent path returns false.
func Build(pkgs []golist.Package) *Graph {
	g := &Graph{
		nodes:   make(map[string]struct{}, len(pkgs)),
		forward: make(map[string][]string, len(pkgs)),
		reverse: make(map[string][]string),
	}

	// First pass: register nodes and collect raw edges.
	// A second per-path entry overrides the first (last-entry-wins).
	for _, p := range pkgs {
		if p.ImportPath == "" {
			continue
		}
		g.nodes[p.ImportPath] = struct{}{}
		g.forward[p.ImportPath] = mergeImports(p.Imports, p.TestImports, p.XTestImports)
	}

	// Second pass: build the reverse map from the (now stable) forward map.
	for src, targets := range g.forward {
		for _, dst := range targets {
			g.reverse[dst] = append(g.reverse[dst], src)
		}
	}

	// Sort the reverse map for deterministic, O(1)-access methods. The
	// forward map is already sorted by mergeImports; the reverse map is
	// dedup-by-construction (each src-imports-dst pair appends once),
	// so no dedup pass is required.
	sortValues(g.forward)
	sortValues(g.reverse)

	g.stats = computeStats(g)
	return g
}

// mergeImports returns the sorted, deduplicated union of three import
// slices. Empty / nil inputs are tolerated. Used by Build to merge
// Package.Imports + TestImports + XTestImports into a single edge set.
func mergeImports(a, b, c []string) []string {
	if len(a)+len(b)+len(c) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(a)+len(b)+len(c))
	out := make([]string, 0, len(a)+len(b)+len(c))
	for _, s := range a {
		if _, dup := seen[s]; !dup {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	for _, s := range b {
		if _, dup := seen[s]; !dup {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	for _, s := range c {
		if _, dup := seen[s]; !dup {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// sortValues sorts each value slice in m in place. Used by Build to
// ensure DirectImports / DirectImporters / RevDepClosure return
// deterministically-ordered output without per-call sorts.
//
// Slices of length 0 or 1 are already trivially sorted; the guard
// keeps Build allocation-free for sparse graphs.
func sortValues(m map[string][]string) {
	for _, v := range m {
		if len(v) > 1 {
			sort.Strings(v)
		}
	}
}

// computeStats walks the graph's internal maps and produces a Stats
// snapshot. Called once at the end of Build; cached on the Graph and
// returned (by value) from Stats.
//
// MaxInDegree and MaxOutDegree are computed across known nodes only
// (i.e. import paths present as Package.ImportPath in Build's input).
// Edges to absent nodes (typically stdlib paths the caller didn't
// include in pkgs) contribute to Edges and to MaxOutDegree but do not
// themselves appear as nodes, so they don't get an in-degree count.
func computeStats(g *Graph) Stats {
	s := Stats{Nodes: len(g.nodes)}
	for path := range g.nodes {
		out := len(g.forward[path])
		s.Edges += out
		if out > s.MaxOutDegree {
			s.MaxOutDegree = out
		}
		if in := len(g.reverse[path]); in > s.MaxInDegree {
			s.MaxInDegree = in
		}
	}
	return s
}
