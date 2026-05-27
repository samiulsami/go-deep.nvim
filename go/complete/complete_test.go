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

func (s *stubSymbolStore) Match(query string, n int) ([]*symbol.Symbol, error) {
	if n > len(s.results) {
		n = len(s.results)
	}
	return s.results[:n], nil
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
	sym := &symbol.Symbol{Name: "Println", ImportPath: "fmt"}

	cache.symbols = []*symbol.Symbol{sym}
	cache.items[cache.symbolDedupKey(sym)] = 0

	cache.evictLocked()

	if len(cache.symbols) != 0 {
		t.Fatalf("expected empty symbols after eviction, got %d", len(cache.symbols))
	}
	if len(cache.items) != 0 {
		t.Fatalf("expected empty items after eviction, got %d", len(cache.items))
	}

	cache.StoreBatch([]*symbol.Symbol{sym})
	if len(cache.symbols) != 1 {
		t.Fatalf("expected symbol to store cleanly after eviction, got %d", len(cache.symbols))
	}
	if got := cache.items[cache.symbolDedupKey(sym)]; got != 0 {
		t.Fatalf("expected stored symbol index 0, got %d", got)
	}
	if cache.symbols[0] != sym {
		t.Fatalf("expected stored symbol pointer to match input")
	}
}

func TestProjectSymbolCacheStoreBatchUpsertsExistingSymbol(t *testing.T) {
	cache := NewProjectSymbolCache(2)
	original := &symbol.Symbol{Name: "Println", ImportPath: "fmt", PackageName: "fmt"}
	updated := &symbol.Symbol{Name: "Println", ImportPath: "fmt", PackageName: "fmt_alias"}

	cache.StoreBatch([]*symbol.Symbol{original})
	cache.StoreBatch([]*symbol.Symbol{updated})

	if len(cache.symbols) != 1 {
		t.Fatalf("expected one deduplicated symbol, got %d", len(cache.symbols))
	}
	if cache.symbols[0] != updated {
		t.Fatalf("expected cached symbol to be updated in place")
	}
	if got := cache.items[cache.symbolDedupKey(updated)]; got != 0 {
		t.Fatalf("expected updated symbol index 0, got %d", got)
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
