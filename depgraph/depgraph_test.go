package depgraph_test

import (
	"sort"
	"sync"
	"testing"

	"github.com/geomyidia/cascade/depgraph"
	"github.com/geomyidia/cascade/golist"
)

// TestRevDepClosure_StandardTopologies (F-8, plus F-13 under -count=10
// and F-14 via the "cycle" / "self-loop" subtests) walks 16 synthetic
// graph topologies that together cover the BFS algorithm's branches
// and corner cases.
func TestRevDepClosure_StandardTopologies(t *testing.T) {
	tests := []struct {
		name  string
		edges map[string][]string
		seeds []string
		want  []string
	}{
		{
			name:  "empty_graph_no_seeds",
			edges: nil,
			seeds: nil,
			want:  nil,
		},
		{
			name:  "empty_graph_one_seed",
			edges: nil,
			seeds: []string{"x"},
			want:  nil,
		},
		{
			name:  "single_node_no_edges_seed_self",
			edges: map[string][]string{"a": nil},
			seeds: []string{"a"},
			want:  []string{"a"},
		},
		{
			name:  "single_node_no_edges_seed_other",
			edges: map[string][]string{"a": nil},
			seeds: []string{"x"},
			want:  nil,
		},
		{
			name: "linear_chain_seed_leaf",
			edges: map[string][]string{
				"a": {"b"},
				"b": {"c"},
				"c": nil,
			},
			seeds: []string{"c"},
			want:  []string{"a", "b", "c"},
		},
		{
			name: "linear_chain_seed_middle",
			edges: map[string][]string{
				"a": {"b"},
				"b": {"c"},
				"c": nil,
			},
			seeds: []string{"b"},
			want:  []string{"a", "b"},
		},
		{
			name: "linear_chain_seed_root",
			edges: map[string][]string{
				"a": {"b"},
				"b": {"c"},
				"c": nil,
			},
			seeds: []string{"a"},
			want:  []string{"a"},
		},
		{
			name: "diamond_seed_leaf",
			edges: map[string][]string{
				"a": {"b", "c"},
				"b": {"d"},
				"c": {"d"},
				"d": nil,
			},
			seeds: []string{"d"},
			want:  []string{"a", "b", "c", "d"},
		},
		{
			name: "diamond_seed_middle",
			edges: map[string][]string{
				"a": {"b", "c"},
				"b": {"d"},
				"c": {"d"},
				"d": nil,
			},
			seeds: []string{"b"},
			want:  []string{"a", "b"},
		},
		{
			name: "cycle_3node",
			edges: map[string][]string{
				"a": {"b"},
				"b": {"c"},
				"c": {"a"},
			},
			seeds: []string{"a"},
			want:  []string{"a", "b", "c"},
		},
		{
			name: "self-loop",
			edges: map[string][]string{
				"a": {"a"},
			},
			seeds: []string{"a"},
			want:  []string{"a"},
		},
		{
			name: "disconnected_seed_one_component",
			edges: map[string][]string{
				"a": {"b"},
				"b": nil,
				"x": {"y"},
				"y": nil,
			},
			seeds: []string{"b"},
			want:  []string{"a", "b"},
		},
		{
			name: "disconnected_multi_seed",
			edges: map[string][]string{
				"a": {"b"},
				"b": nil,
				"x": {"y"},
				"y": nil,
			},
			seeds: []string{"b", "y"},
			want:  []string{"a", "b", "x", "y"},
		},
		{
			name: "duplicate_seeds",
			edges: map[string][]string{
				"a": {"b"},
				"b": nil,
			},
			seeds: []string{"b", "b"},
			want:  []string{"a", "b"},
		},
		{
			name: "unsorted_seeds",
			edges: map[string][]string{
				"a": {"b"},
				"b": {"c"},
				"c": nil,
			},
			seeds: []string{"c", "a"},
			want:  []string{"a", "b", "c"},
		},
		{
			name: "mixed_seeds_some_absent",
			edges: map[string][]string{
				"a": {"b"},
				"b": nil,
			},
			seeds: []string{"absent", "b", "also-absent"},
			want:  []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := buildTestGraph(t, tt.edges)
			got := g.RevDepClosure(tt.seeds)
			if !stringSlicesEqual(got, tt.want) {
				t.Errorf("RevDepClosure(%v) = %v, want %v", tt.seeds, got, tt.want)
			}
			if !sort.StringsAreSorted(got) {
				t.Errorf("RevDepClosure(%v) = %v is not sorted lexicographically", tt.seeds, got)
			}
		})
	}
}

// TestRevDepClosure_HandTraceable (F-9) is the spec's exit-criterion
// example — a 5-package graph with hand-derived expected output. It
// documents the closure semantics for future maintainers and acts as
// an integration sanity check.
//
//	pkg/a → pkg/b
//	pkg/c → pkg/b
//	pkg/b → pkg/d
//	pkg/e (no imports)
func TestRevDepClosure_HandTraceable(t *testing.T) {
	edges := map[string][]string{
		"pkg/a": {"pkg/b"},
		"pkg/b": {"pkg/d"},
		"pkg/c": {"pkg/b"},
		"pkg/d": nil,
		"pkg/e": nil,
	}
	g := buildTestGraph(t, edges)

	tests := []struct {
		name  string
		seeds []string
		want  []string
	}{
		{"seed_a", []string{"pkg/a"}, []string{"pkg/a"}},
		{"seed_b", []string{"pkg/b"}, []string{"pkg/a", "pkg/b", "pkg/c"}},
		{"seed_d", []string{"pkg/d"}, []string{"pkg/a", "pkg/b", "pkg/c", "pkg/d"}},
		{"seed_e", []string{"pkg/e"}, []string{"pkg/e"}},
		{"seed_a_and_e", []string{"pkg/a", "pkg/e"}, []string{"pkg/a", "pkg/e"}},
		{"empty_seeds", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.RevDepClosure(tt.seeds)
			if !stringSlicesEqual(got, tt.want) {
				t.Errorf("RevDepClosure(%v) = %v, want %v", tt.seeds, got, tt.want)
			}
		})
	}
}

// TestBuild_TestAndXTestImportsBecomeEdges (F-10) verifies that a
// Package's TestImports and XTestImports merge with Imports to form
// the outgoing-edge set, and that overlapping entries across the
// three slices are deduplicated.
func TestBuild_TestAndXTestImportsBecomeEdges(t *testing.T) {
	pkgs := []golist.Package{
		{
			ImportPath:   "p",
			Imports:      []string{"i", "shared"},
			TestImports:  []string{"t", "shared"},
			XTestImports: []string{"xt"},
		},
		{ImportPath: "i"},
		{ImportPath: "t"},
		{ImportPath: "xt"},
		{ImportPath: "shared"},
	}
	g := depgraph.Build(pkgs)

	// p must directly import all four targets, deduplicated and sorted.
	wantImports := []string{"i", "shared", "t", "xt"}
	if got := g.DirectImports("p"); !stringSlicesEqual(got, wantImports) {
		t.Errorf("DirectImports(p) = %v, want %v", got, wantImports)
	}

	// Each of i, t, xt, shared must report p as a direct importer.
	for _, target := range []string{"i", "t", "xt", "shared"} {
		got := g.DirectImporters(target)
		if !stringSlicesEqual(got, []string{"p"}) {
			t.Errorf("DirectImporters(%s) = %v, want [p]", target, got)
		}
	}

	// RevDepClosure of any of those targets must include p.
	for _, target := range []string{"t", "xt", "shared"} {
		got := g.RevDepClosure([]string{target})
		if !stringSlicesEqual(got, []string{target, "p"}) && target == "shared" {
			// "p" < "shared" lexicographically, so the sort orders
			// "p" before the target.
			want := []string{"p", "shared"}
			if !stringSlicesEqual(got, want) {
				t.Errorf("RevDepClosure([%s]) = %v, want %v", target, got, want)
			}
			continue
		}
		want := []string{"p", target}
		sort.Strings(want)
		if !stringSlicesEqual(got, want) {
			t.Errorf("RevDepClosure([%s]) = %v, want %v", target, got, want)
		}
	}
}

// TestDirectImports (F-11) covers DirectImports across present nodes
// (with multiple imports, sorted), present-but-no-imports, and absent
// paths.
func TestDirectImports(t *testing.T) {
	g := buildTestGraph(t, map[string][]string{
		"a": {"c", "b"}, // intentionally unsorted on input
		"b": nil,
		"c": nil,
	})

	tests := []struct {
		name string
		path string
		want []string
	}{
		{"multi_import_sorted", "a", []string{"b", "c"}},
		{"present_no_imports", "b", nil},
		{"absent_path", "ghost", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.DirectImports(tt.path)
			if !stringSlicesEqual(got, tt.want) {
				t.Errorf("DirectImports(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}

	// Mutating the returned slice must not corrupt the Graph's state.
	got := g.DirectImports("a")
	if len(got) > 0 {
		got[0] = "MUTATED"
	}
	if again := g.DirectImports("a"); !stringSlicesEqual(again, []string{"b", "c"}) {
		t.Errorf("DirectImports leaked internal slice: after mutation got %v", again)
	}
}

// TestDirectImporters (F-11) covers DirectImporters across present
// nodes (with multiple importers, sorted), present-but-no-importers,
// and absent paths.
func TestDirectImporters(t *testing.T) {
	g := buildTestGraph(t, map[string][]string{
		"a": {"c"},
		"b": {"c"},
		"c": nil,
	})

	tests := []struct {
		name string
		path string
		want []string
	}{
		{"multi_importer_sorted", "c", []string{"a", "b"}},
		{"present_no_importers", "a", nil},
		{"absent_path", "ghost", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.DirectImporters(tt.path)
			if !stringSlicesEqual(got, tt.want) {
				t.Errorf("DirectImporters(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}

	// Mutating the returned slice must not corrupt the Graph's state.
	got := g.DirectImporters("c")
	if len(got) > 0 {
		got[0] = "MUTATED"
	}
	if again := g.DirectImporters("c"); !stringSlicesEqual(again, []string{"a", "b"}) {
		t.Errorf("DirectImporters leaked internal slice: after mutation got %v", again)
	}
}

// TestHas (F-11) covers Has for present and absent paths.
func TestHas(t *testing.T) {
	g := buildTestGraph(t, map[string][]string{"a": nil, "b": nil})

	tests := []struct {
		path string
		want bool
	}{
		{"a", true},
		{"b", true},
		{"ghost", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := g.Has(tt.path); got != tt.want {
				t.Errorf("Has(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestBuild_DuplicatePackagesIdempotent (F-12) verifies that two
// Package entries sharing an ImportPath collapse to a single node
// with the last entry's edge set winning. Also covers the "empty
// ImportPath skipped" branch in Build.
func TestBuild_DuplicatePackagesIdempotent(t *testing.T) {
	pkgs := []golist.Package{
		{ImportPath: "", Imports: []string{"ignored"}}, // skipped: empty ImportPath
		{ImportPath: "p", Imports: []string{"first"}},
		{ImportPath: "p", Imports: []string{"second"}}, // last entry wins
		{ImportPath: "first"},
		{ImportPath: "second"},
		{ImportPath: "ignored"}, // not connected to anything
	}
	g := depgraph.Build(pkgs)

	if !g.Has("p") {
		t.Error("Has(p) = false, want true")
	}
	if g.Has("") {
		t.Error("Has(\"\") = true, want false (empty ImportPath must not become a node)")
	}

	// p's outgoing edges should be [second], not [first] or [first, second].
	got := g.DirectImports("p")
	if !stringSlicesEqual(got, []string{"second"}) {
		t.Errorf("DirectImports(p) = %v, want [second] (last entry wins)", got)
	}

	// The "first" target should have no importers; only "second" should.
	if got := g.DirectImporters("first"); len(got) != 0 {
		t.Errorf("DirectImporters(first) = %v, want empty (first entry was overwritten)", got)
	}
	if got := g.DirectImporters("second"); !stringSlicesEqual(got, []string{"p"}) {
		t.Errorf("DirectImporters(second) = %v, want [p]", got)
	}

	// "ignored" must not appear connected to anything (the skipped
	// package's Imports were dropped along with the empty-path entry).
	if got := g.DirectImporters("ignored"); len(got) != 0 {
		t.Errorf("DirectImporters(ignored) = %v, want empty", got)
	}
}

// TestGraphConcurrentRead (F-17) fans out N goroutines calling
// RevDepClosure, DirectImports, DirectImporters, Has, and Stats on
// the same *Graph and asserts each goroutine sees identical output.
// The race detector (`go test -race`) catches any actual races; the
// identical-output check protects the doc-claimed concurrent-read
// safety.
func TestGraphConcurrentRead(t *testing.T) {
	g := buildTestGraph(t, map[string][]string{
		"a": {"b", "c"},
		"b": {"d"},
		"c": {"d"},
		"d": nil,
	})

	const goroutines = 8

	wantClosure := g.RevDepClosure([]string{"d"})
	wantImports := g.DirectImports("a")
	wantImporters := g.DirectImporters("d")
	wantStats := g.Stats()

	var wg sync.WaitGroup
	mismatches := make(chan string, goroutines*4)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := g.RevDepClosure([]string{"d"}); !stringSlicesEqual(got, wantClosure) {
				mismatches <- "RevDepClosure"
			}
			if got := g.DirectImports("a"); !stringSlicesEqual(got, wantImports) {
				mismatches <- "DirectImports"
			}
			if got := g.DirectImporters("d"); !stringSlicesEqual(got, wantImporters) {
				mismatches <- "DirectImporters"
			}
			if got := g.Stats(); got != wantStats {
				mismatches <- "Stats"
			}
			if !g.Has("a") {
				mismatches <- "Has"
			}
		}()
	}
	wg.Wait()
	close(mismatches)

	for m := range mismatches {
		t.Errorf("concurrent reader saw divergent output for %s", m)
	}
}

// TestStats verifies that Stats reports correct Nodes / Edges /
// MaxInDegree / MaxOutDegree counts for known graph shapes. Also
// covers the empty-graph case where MaxInDegree and MaxOutDegree
// remain zero.
func TestStats(t *testing.T) {
	tests := []struct {
		name  string
		edges map[string][]string
		want  depgraph.Stats
	}{
		{
			name:  "empty",
			edges: nil,
			want:  depgraph.Stats{},
		},
		{
			name: "single_isolated_node",
			edges: map[string][]string{
				"a": nil,
			},
			want: depgraph.Stats{Nodes: 1},
		},
		{
			name: "diamond",
			// a → b, c     ; b → d ; c → d
			// in-degrees:  a=0 b=1 c=1 d=2  → max 2
			// out-degrees: a=2 b=1 c=1 d=0  → max 2
			// edges total: 2 + 1 + 1 + 0 = 4
			edges: map[string][]string{
				"a": {"b", "c"},
				"b": {"d"},
				"c": {"d"},
				"d": nil,
			},
			want: depgraph.Stats{Nodes: 4, Edges: 4, MaxInDegree: 2, MaxOutDegree: 2},
		},
		{
			name: "edges_to_absent_nodes_count_for_out_not_in",
			// "a" imports "stdlib_path" (not a node). Forward edge
			// counted; no in-degree contribution because stdlib_path
			// isn't a node.
			edges: map[string][]string{
				"a": {"stdlib_path"},
			},
			want: depgraph.Stats{Nodes: 1, Edges: 1, MaxOutDegree: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := buildTestGraph(t, tt.edges)
			if got := g.Stats(); got != tt.want {
				t.Errorf("Stats() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
