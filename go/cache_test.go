package main

import (
	"testing"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

func TestSymCache(t *testing.T) {
	c := newSymCache(2)

	if _, ok := c.Get("a"); ok {
		t.Fatal("empty cache should not return 'a'")
	}

	c.Put("a", []*symbol.Symbol{{Name: "A"}})
	syms, ok := c.Get("a")
	if !ok || syms[0].Name != "A" {
		t.Fatal("should find 'a' with sym A")
	}

	c.Put("b", []*symbol.Symbol{{Name: "B"}})
	c.Put("c", []*symbol.Symbol{{Name: "C"}})

	if _, ok := c.Get("a"); ok {
		t.Fatal("'a' should have been evicted on capacity overrun")
	}
	if _, ok := c.Get("b"); !ok {
		t.Fatal("'b' should still be present")
	}

	c.Get("b")
	c.Put("d", []*symbol.Symbol{{Name: "D"}})

	if _, ok := c.Get("c"); ok {
		t.Fatal("'c' should have been evicted — 'b' was moved to front by Get")
	}
}

func TestSymCacheUpdate(t *testing.T) {
	c := newSymCache(3)

	c.Put("x", []*symbol.Symbol{{Name: "X"}})
	c.Put("x", []*symbol.Symbol{{Name: "X2"}})

	syms, ok := c.Get("x")
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

	c.Put("New\x00"+cwd, []*symbol.Symbol{{Name: "NewChaCha8"}})
	c.Put("NewC\x00"+cwd, []*symbol.Symbol{{Name: "NewChaCha8"}, {Name: "NewClient"}})

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
		t.Fatal("Lookup should not find 'xyz' — below fallback threshold")
	}
	_ = syms
}
