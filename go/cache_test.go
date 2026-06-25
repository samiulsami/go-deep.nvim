package main

import (
	"testing"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

func TestSymCache(t *testing.T) {
	c := newSymCache(2)
	cwd := "/proj"

	if _, ok := c.Get(cacheKey("a", cwd)); ok {
		t.Fatal("empty cache should not return 'a'")
	}

	c.Put("a", cwd, []*symbol.Symbol{{Name: "A"}})
	syms, ok := c.Get(cacheKey("a", cwd))
	if !ok || syms[0].Name != "A" {
		t.Fatal("should find 'a' with sym A")
	}

	c.Put("b", cwd, []*symbol.Symbol{{Name: "B"}})
	c.Put("c", cwd, []*symbol.Symbol{{Name: "C"}})

	if _, ok := c.Get(cacheKey("a", cwd)); ok {
		t.Fatal("'a' should have been evicted on capacity overrun")
	}
	if _, ok := c.Get(cacheKey("b", cwd)); !ok {
		t.Fatal("'b' should still be present")
	}

	c.Get(cacheKey("b", cwd))
	c.Put("d", cwd, []*symbol.Symbol{{Name: "D"}})

	if _, ok := c.Get(cacheKey("c", cwd)); ok {
		t.Fatal("'c' should have been evicted; 'b' was moved to front by Get")
	}
}

func TestSymCacheUpdate(t *testing.T) {
	c := newSymCache(3)
	cwd := "/proj"

	c.Put("x", cwd, []*symbol.Symbol{{Name: "X"}})
	c.Put("x", cwd, []*symbol.Symbol{{Name: "X2"}})

	syms, ok := c.Get(cacheKey("x", cwd))
	if !ok || syms[0].Name != "X2" {
		t.Fatal("Put on existing key should update value and move to front")
	}

	if c.ll.Len() != 1 {
		t.Fatal("updating same key should not grow the list")
	}
}

func TestSymCacheLookupFallback(t *testing.T) {
	c := newSymCache(10)
	cwd := "/proj"

	c.Put("New", cwd, []*symbol.Symbol{{Name: "NewChaCha8"}})
	c.Put("NewC", cwd, []*symbol.Symbol{{Name: "NewChaCha8"}, {Name: "NewClient"}})

	syms, ok := c.Lookup("NewCh", cwd)
	if !ok {
		t.Fatal("Lookup should fall back to 'NewC' for 'NewCh'")
	}
	if len(syms) != 2 {
		t.Fatalf("expected 2 syms from 'NewC' fallback, got %d", len(syms))
	}

	_, ok = c.Lookup("NewClientX", cwd)
	if !ok {
		t.Fatal("Lookup should fall back to 'NewC' for 'NewClientX'")
	}

	syms, ok = c.Lookup("xyz", cwd)
	if ok {
		t.Fatal("Lookup should not find 'xyz'; below fallback threshold")
	}
	_ = syms
}
