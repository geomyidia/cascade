package depgraph

import (
	"slices"
	"sort"
)

// Stats summarises the shape of a Graph. Returned by Graph.Stats.
//
// Fields are computed once at Build time and cached on the Graph;
// Stats() is a cheap accessor that returns a copy callers may freely
// store or modify.
type Stats struct {
	// Nodes is the count of distinct ImportPath values that appeared
	// as Package.ImportPath in Build's input.
	Nodes int

	// Edges is the count of forward edges (importer → imported) across
	// the graph. Reverse edges are not double-counted. An edge from a
	// known node to an absent node (e.g. a stdlib import the caller did
	// not include in pkgs) contributes one to this count.
	Edges int

	// MaxInDegree is the largest reverse-edge count across known nodes
	// — the most-depended-on node's importer count. Zero on an empty
	// graph or a graph with no edges between known nodes.
	MaxInDegree int

	// MaxOutDegree is the largest forward-edge count across known nodes
	// — the most-importing node's import count. Zero on an empty graph.
	MaxOutDegree int
}

// Has reports whether path is a node in the graph. A path is a node
// iff it appeared as a Package.ImportPath in Build's input; paths
// that appear only as edge targets (e.g. stdlib imports the caller
// did not include in pkgs) are not nodes.
func (g *Graph) Has(path string) bool {
	_, ok := g.nodes[path]
	return ok
}

// DirectImports returns the import paths directly imported by path
// — the union of Imports + TestImports + XTestImports of the
// underlying Package, sorted lexicographically. Returns nil if path
// is not in the graph.
//
// The returned slice is a copy; callers may freely store or mutate it
// without affecting the Graph's internal state.
func (g *Graph) DirectImports(path string) []string {
	if _, ok := g.nodes[path]; !ok {
		return nil
	}
	return slices.Clone(g.forward[path])
}

// DirectImporters returns the import paths that directly import path,
// sorted lexicographically. Returns nil if path is not in the graph.
//
// The returned slice is a copy; callers may freely store or mutate it
// without affecting the Graph's internal state.
func (g *Graph) DirectImporters(path string) []string {
	if _, ok := g.nodes[path]; !ok {
		return nil
	}
	return slices.Clone(g.reverse[path])
}

// RevDepClosure returns the seeds plus every package that transitively
// imports any of them — the reverse-transitive closure under the
// imports relation. The output is sorted lexicographically for
// deterministic CI behaviour.
//
// Seeds not present in the graph are silently skipped. Cycle-safe via
// a visited set. Complexity: O(V + E) over the reachable subgraph,
// plus O(k log k) for the final sort on result size k.
func (g *Graph) RevDepClosure(seeds []string) []string {
	visited := make(map[string]struct{})
	var queue []string
	for _, s := range seeds {
		if _, ok := g.nodes[s]; ok {
			queue = append(queue, s)
		}
	}

	var result []string
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if _, seen := visited[n]; seen {
			continue
		}
		visited[n] = struct{}{}
		result = append(result, n)
		for _, importer := range g.reverse[n] {
			if _, seen := visited[importer]; !seen {
				queue = append(queue, importer)
			}
		}
	}

	sort.Strings(result)
	return result
}

// Stats returns a summary of the graph's shape. The returned value is
// a copy; callers may freely store or modify it.
func (g *Graph) Stats() Stats {
	return g.stats
}
