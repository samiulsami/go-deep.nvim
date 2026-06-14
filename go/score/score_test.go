package score

import (
	"testing"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

func TestMatchOrdersByScore(t *testing.T) {
	syms := []*symbol.Symbol{
		{Name: "Println", ImportPath: "fmt", Haystack: "fmt\x00Println"},
		{Name: "Printf", ImportPath: "fmt", Haystack: "fmt\x00Printf"},
		{Name: "Sprint", ImportPath: "fmt", Haystack: "fmt\x00Sprint"},
	}

	got := Match(RankOpts{Query: "pri", Limit: 3}, syms)
	if len(got) == 0 {
		t.Fatal("expected ranked results")
	}
	for i := 1; i < len(got); i++ {
		prev := Score("pri", got[i-1].Haystack)
		curr := Score("pri", got[i].Haystack)
		if prev < curr {
			t.Fatalf("results not sorted by descending score: %d < %d at %d", prev, curr, i)
		}
	}
}

func TestMatchUsesDeterministicTieBreakers(t *testing.T) {
	syms := []*symbol.Symbol{
		{Name: "Printa", ImportPath: "z/fmt", Haystack: "fmt\x00Printa"},
		{Name: "Printa", ImportPath: "a/fmt", Haystack: "fmt\x00Printa"},
		{Name: "Printb", ImportPath: "a/fmt", Haystack: "fmt\x00Printb"},
	}

	got := Match(RankOpts{Query: "print", Limit: 3}, syms)
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}
	if got[0].ImportPath != "a/fmt" || got[0].Name != "Printa" {
		t.Fatalf("unexpected first result: %s %s", got[0].ImportPath, got[0].Name)
	}
	if got[1].ImportPath != "a/fmt" || got[1].Name != "Printb" {
		t.Fatalf("unexpected second result: %s %s", got[1].ImportPath, got[1].Name)
	}
	if got[2].ImportPath != "z/fmt" || got[2].Name != "Printa" {
		t.Fatalf("unexpected third result: %s %s", got[2].ImportPath, got[2].Name)
	}
}

func TestMatchMultipleLists(t *testing.T) {
	list1 := []*symbol.Symbol{
		{Name: "Println", ImportPath: "fmt", Haystack: "fmt\x00Println"},
	}
	list2 := []*symbol.Symbol{
		{Name: "Printf", ImportPath: "fmt", Haystack: "fmt\x00Printf"},
	}

	got := Match(RankOpts{Query: "pri", Limit: 3}, list1, list2)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
}
