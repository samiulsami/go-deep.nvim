package complete

import (
	"testing"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

func baseSym(name, impPath string, kind symbol.Kind) *symbol.Symbol { // nolint:unparam
	return &symbol.Symbol{
		Name:       name,
		ImportPath: impPath,
		Kind:       kind,
		Haystack:   impPath + "\x00" + name,
		Location:   symbol.Location{Path: "/proj/" + impPath + "/" + name + ".go"},
	}
}

func defaultOpts() ProcessOptions {
	return ProcessOptions{
		MaxItems:           30,
		MaxFromSamePackage: 4,
		ExcludeImported:    false,
		ExcludeVendored:    false,
		ExcludeInternal:    false,
		ExcludeTestFiles:   false,
	}
}

func TestFilterSymbolsDedup(t *testing.T) {
	syms := []*symbol.Symbol{
		baseSym("Println", "fmt", symbol.FunctionKind),
		baseSym("Println", "fmt", symbol.FunctionKind),
	}
	got := filterSymbols(syms, defaultOpts(), "", nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 after dedup, got %d", len(got))
	}
}

func TestFilterSymbolsSameFileExcluded(t *testing.T) {
	s := baseSym("Println", "fmt", symbol.FunctionKind)
	s.Location.Path = "/proj/main.go"
	got := filterSymbols([]*symbol.Symbol{s}, defaultOpts(), "/proj/main.go", nil, nil)
	if len(got) != 0 {
		t.Fatal("symbol from same file should be excluded")
	}
}

func TestFilterSymbolsMaxPerPackage(t *testing.T) {
	opts := defaultOpts()
	opts.MaxFromSamePackage = 2
	syms := []*symbol.Symbol{
		baseSym("A", "fmt", symbol.FunctionKind),
		baseSym("B", "fmt", symbol.FunctionKind),
		baseSym("C", "fmt", symbol.FunctionKind),
		baseSym("D", "log", symbol.FunctionKind),
	}
	got := filterSymbols(syms, opts, "", nil, nil)
	if len(got) != 3 {
		t.Fatalf("expected 3 (2 from fmt + 1 from log), got %d", len(got))
	}
}

func TestFilterSymbolsExcludeImported(t *testing.T) {
	opts := defaultOpts()
	opts.ExcludeImported = true
	syms := []*symbol.Symbol{
		baseSym("Println", "fmt", symbol.FunctionKind),
		baseSym("Print", "log", symbol.FunctionKind),
	}
	imported := map[string]string{"fmt": "fmt"}
	got := filterSymbols(syms, opts, "", imported, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 (fmt excluded), got %d", len(got))
	}
	if got[0].ImportPath != "log" {
		t.Fatalf("expected log, got %s", got[0].ImportPath)
	}
}

func TestFilterSymbolsExcludeTestFiles(t *testing.T) {
	opts := defaultOpts()
	opts.ExcludeTestFiles = true
	s := baseSym("TestFoo", "fmt", symbol.FunctionKind)
	s.Location.Path = "/proj/fmt/foo_test.go"
	got := filterSymbols([]*symbol.Symbol{s}, opts, "", nil, nil)
	if len(got) != 0 {
		t.Fatal("test file symbol should be excluded")
	}
}

func TestFilterSymbolsExcludeVendored(t *testing.T) {
	opts := defaultOpts()
	opts.ExcludeVendored = true
	s := baseSym("Foo", "vendor/pkg", symbol.FunctionKind)
	s.IsVendored = true
	got := filterSymbols([]*symbol.Symbol{s}, opts, "", nil, nil)
	if len(got) != 0 {
		t.Fatal("vendored symbol should be excluded")
	}
}

func TestIsInternalImportPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"foo/internal/bar", true},
		{"internal/bar", true},
		{"foo/internal", true},
		{"foo/bar", false},
		{"internalfoo", false},
		{"foointernal", false},
	}
	for _, c := range cases {
		if got := IsInternalImportPath(c.path); got != c.want {
			t.Errorf("IsInternalImportPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestBuildWithNoOptions(t *testing.T) {
	syms := []*symbol.Symbol{
		baseSym("Println", "fmt", symbol.FunctionKind),
	}
	req := Request{Prefix: "pri", Options: nil}
	items := Build(req, nil, syms)
	if len(items) != 0 {
		t.Fatalf("Build with nil options (MaxItems=0) should return no items, got %d", len(items))
	}
}
