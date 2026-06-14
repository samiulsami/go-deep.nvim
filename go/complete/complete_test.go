package complete

import (
	"path/filepath"
	"testing"

	"github.com/samiulsami/go-deep.nvim/go/gopls"
	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

type stubSymbolStore struct {
	results []*symbol.Symbol
}

func (s *stubSymbolStore) Match(query string, n int) []*symbol.Symbol {
	if n > len(s.results) {
		n = len(s.results)
	}
	return s.results[:n]
}

func (s *stubSymbolStore) StoreBatch(syms []*symbol.Symbol) {}

func TestCompleteHonorsRequestOptionOverrides(t *testing.T) {
	store := &stubSymbolStore{results: []*symbol.Symbol{
		{Name: "Println", ImportPath: "fmt", Location: symbol.Location{Path: "/tmp/fmt/print.go"}},
		{Name: "Printf", ImportPath: "fmt", Location: symbol.Location{Path: "/tmp/fmt/print.go"}},
	}}
	comp := NewDefaultWithProvider(nil, store, &Provider{})

	result, err := comp.Complete(Request{
		Prefix:          "Pr",
		Filepath:        "/workspace/main.go",
		CWD:             "/workspace",
		MinPrefixLength: 2,
		Options:         ProcessOptions{MaxItems: 1},
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected one item from request override, got %d", len(result.Items))
	}
}

func TestCompleteHonorsRequestExcludeImportedOverride(t *testing.T) {
	store := &stubSymbolStore{results: []*symbol.Symbol{
		{Name: "Println", ImportPath: "fmt", Location: symbol.Location{Path: "/tmp/fmt/print.go"}},
	}}
	comp := NewDefaultWithProvider(nil, store, &Provider{})

	result, err := comp.Complete(Request{
		Prefix:          "Pri",
		Filepath:        "/workspace/main.go",
		CWD:             "/workspace",
		MinPrefixLength: 3,
		ImportedPaths: map[string]string{
			"fmt": "fmt",
		},
		Options: ProcessOptions{MaxItems: 10, ExcludeImported: true},
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected imported package to be excluded, got %d items", len(result.Items))
	}
}

func TestFilterSymbolsHonorsExcludeTestFilesPerRequest(t *testing.T) {
	candidates := []*symbol.Symbol{
		{
			Name:       "Helper",
			ImportPath: "example.com/project/pkg",
			Location:   symbol.Location{Path: "/workspace/pkg/helper_test.go"},
		},
	}
	ctx := processContext{
		request: Request{
			Prefix:   "Hel",
			Filepath: "/workspace/main.go",
			Options:  ProcessOptions{MaxItems: 10, ExcludeTestFiles: true},
		},
		seenSymbols:   make(map[string]bool),
		packageCounts: make(map[string]int),
		includeSymbol: (&Provider{}).IncludeSymbol,
	}

	if got := filterSymbols(ctx, candidates); len(got) != 0 {
		t.Fatalf("expected test file symbol to be filtered, got %d", len(got))
	}

	ctx.request.Options.ExcludeTestFiles = false
	ctx.seenSymbols = make(map[string]bool)
	ctx.packageCounts = make(map[string]int)
	if got := filterSymbols(ctx, candidates); len(got) != 1 {
		t.Fatalf("expected test file symbol when ExcludeTestFiles is false, got %d", len(got))
	}
}

func TestProjectSymbolCacheEvictSingleClearsIndex(t *testing.T) {
	cache := NewProjectSymbolCache(1)
	first := &symbol.Symbol{Name: "Println", ImportPath: "fmt", Haystack: "fmt\x00Println"}
	second := &symbol.Symbol{Name: "Printf", ImportPath: "fmt", Haystack: "fmt\x00Printf"}

	cache.StoreBatch([]*symbol.Symbol{first})
	cache.StoreBatch([]*symbol.Symbol{second})

	if len(cache.items) != 1 {
		t.Fatalf("expected one cache entry after eviction, got %d", len(cache.items))
	}
	if got := cache.Match("Println", 1); len(got) != 0 {
		t.Fatalf("expected evicted symbol to be gone, got %d results", len(got))
	}
	if got := cache.Match("Printf", 1); len(got) != 1 || got[0] != second {
		t.Fatalf("expected remaining symbol to be Printf")
	}
}

func TestProjectSymbolCacheStoreBatchUpsertsExistingSymbol(t *testing.T) {
	cache := NewProjectSymbolCache(2)
	original := &symbol.Symbol{Name: "Println", ImportPath: "fmt", PackageName: "fmt", Haystack: "fmt\x00Println"}
	updated := &symbol.Symbol{Name: "Println", ImportPath: "fmt", PackageName: "fmt_alias", Haystack: "fmt\x00Println"}

	cache.StoreBatch([]*symbol.Symbol{original})
	cache.StoreBatch([]*symbol.Symbol{updated})

	if len(cache.items) != 1 {
		t.Fatalf("expected one deduplicated symbol, got %d", len(cache.items))
	}
	got := cache.Match("Println", 1)
	if len(got) != 1 || got[0] != updated {
		t.Fatalf("expected cached symbol to be updated in place")
	}
}

func TestProjectSymbolCacheMatchRefreshesRecency(t *testing.T) {
	cache := NewProjectSymbolCache(2)
	first := &symbol.Symbol{Name: "Println", ImportPath: "fmt", Haystack: "fmt\x00Println"}
	second := &symbol.Symbol{Name: "Sprintf", ImportPath: "fmt", Haystack: "fmt\x00Sprintf"}
	third := &symbol.Symbol{Name: "Errorf", ImportPath: "fmt", Haystack: "fmt\x00Errorf"}

	cache.StoreBatch([]*symbol.Symbol{first, second})
	if got := cache.Match("Print", 1); len(got) != 1 || got[0] != first {
		t.Fatalf("expected Println cache hit")
	}
	cache.StoreBatch([]*symbol.Symbol{third})

	if got := cache.Match("Print", 1); len(got) != 1 || got[0] != first {
		t.Fatalf("expected Println to stay resident after recency refresh")
	}
	if got := cache.Match("Sprintf", 1); len(got) != 0 {
		t.Fatalf("expected least-recently-used symbol to be evicted")
	}
}

func TestNormalizeConvertsURIToAbsolutePathAndClassifiesSymbol(t *testing.T) {
	cwd := filepath.Clean("/workspace/project")
	raw := &gopls.LspSymbol{
		Name:          "pkg.Println",
		ContainerName: "pkg",
		Kind:          symbol.FunctionKind,
		Location: gopls.FileLocation{
			URI: "file:///workspace/project/vendor/example.com/lib/print.go",
			Range: symbol.Range{
				Start: symbol.Position{Line: 10, Character: 2},
			},
		},
	}

	normalized, ok := normalize(raw, cwd)
	if !ok {
		t.Fatalf("expected symbol to normalize")
	}
	if normalized.Name != "Println" {
		t.Fatalf("expected trimmed name Println, got %q", normalized.Name)
	}
	if normalized.Location.Path != filepath.Clean("/workspace/project/vendor/example.com/lib/print.go") {
		t.Fatalf("unexpected normalized path %q", normalized.Location.Path)
	}
	if !normalized.IsVendored {
		t.Fatalf("expected vendored symbol")
	}
	if normalized.IsLocal {
		t.Fatalf("vendored symbol should not also be marked local")
	}
}

func TestNormalizeRejectsUnsupportedURI(t *testing.T) {
	base := &gopls.LspSymbol{
		Name:          "pkg.Println",
		ContainerName: "pkg",
		Kind:          symbol.FunctionKind,
	}

	if _, ok := normalize(&gopls.LspSymbol{
		Name:          base.Name,
		ContainerName: base.ContainerName,
		Kind:          base.Kind,
		Location:      gopls.FileLocation{URI: "https://example.com/print.go"},
	}, ""); ok {
		t.Fatalf("expected non-file URI to be rejected")
	}
}

func TestBuildItemsIgnoresPathLikeQualifiedPackageName(t *testing.T) {
	goProvider, err := NewProvider(&gopls.GoplsManager{})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	items, err := goProvider.BuildItems(Request{}, []*symbol.Symbol{{
		Name:        "NewFilteredManifestWorkReplicaSetInformer",
		ImportPath:  "open-cluster-management.io/api/client/work/informers/externalversions/work/v1alpha1",
		PackageName: "open_cluster_management.io/api/client/work/informers/externalversions/work/v1alpha1",
		Kind:        symbol.FunctionKind,
		Location: symbol.Location{
			Path: filepath.Clean("/workspace/project/informer.go"),
		},
	}})
	if err != nil {
		t.Fatalf("BuildItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if got := items[0].Word; got != "v1alpha1.NewFilteredManifestWorkReplicaSetInformer" {
		t.Fatalf("expected compact completion word, got %q", got)
	}
}

func TestBuildItemsNormalSymbolUsesImportPathTail(t *testing.T) {
	goProvider, err := NewProvider(&gopls.GoplsManager{})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	items, err := goProvider.BuildItems(Request{}, []*symbol.Symbol{{
		Name:        "Println",
		ImportPath:  "fmt",
		PackageName: "",
		Kind:        symbol.FunctionKind,
		Location: symbol.Location{
			Path: filepath.Clean("/workspace/project/print.go"),
		},
	}})
	if err != nil {
		t.Fatalf("BuildItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	expected := "fmt.Println"
	if got := items[0].Word; got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestBuildItemsUsesExistingImportAlias(t *testing.T) {
	goProvider, err := NewProvider(&gopls.GoplsManager{})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	items, err := goProvider.BuildItems(Request{
		ImportedPaths: map[string]string{
			"fmt": "fmt_renamed",
		},
	}, []*symbol.Symbol{{
		Name:        "Println",
		ImportPath:  "fmt",
		PackageName: "",
		Kind:        symbol.FunctionKind,
		Location: symbol.Location{
			Path: filepath.Clean("/workspace/project/print.go"),
		},
	}})
	if err != nil {
		t.Fatalf("BuildItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	expected := "fmt_renamed.Println"
	if got := items[0].Word; got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}
