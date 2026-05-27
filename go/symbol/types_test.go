package symbol

import "testing"

func TestHashIgnoresLocationForSameSymbol(t *testing.T) {
	a := &Symbol{
		Name:       "NewChaCha8",
		ImportPath: "math/rand/v2",
		Kind:       FunctionKind,
		Location: Location{Range: Range{
			Start: Position{Line: 10, Character: 2},
			End:   Position{Line: 10, Character: 12},
		}},
	}
	b := &Symbol{
		Name:       "NewChaCha8",
		ImportPath: "math/rand/v2",
		Kind:       FunctionKind,
		Location: Location{Range: Range{
			Start: Position{Line: 42, Character: 7},
			End:   Position{Line: 42, Character: 17},
		}},
	}

	if Hash(a) != Hash(b) {
		t.Fatalf("expected same hash for same symbol identity, got %q and %q", Hash(a), Hash(b))
	}
}

func TestHashDiffersForDifferentImportPath(t *testing.T) {
	a := &Symbol{Name: "New", ImportPath: "example.com/a", Kind: FunctionKind}
	b := &Symbol{Name: "New", ImportPath: "example.com/b", Kind: FunctionKind}

	if Hash(a) == Hash(b) {
		t.Fatalf("expected different hashes for different import paths")
	}
}
