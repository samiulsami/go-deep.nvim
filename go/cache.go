package main

import (
	"container/list"
	"sync"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

type symCacheEntry struct {
	key  string
	syms []*symbol.Symbol
}

type symCache struct {
	mu    sync.Mutex
	max   int
	items map[string]*list.Element
	ll    *list.List
}

func newSymCache(max int) *symCache {
	return &symCache{
		max:   max,
		items: make(map[string]*list.Element),
		ll:    list.New(),
	}
}

const (
	minFallbackPrefixLen = 3
	defaultCacheSize     = 100
)

func (c *symCache) Lookup(prefix, cwd string) ([]*symbol.Symbol, bool) {
	for len(prefix) >= minFallbackPrefixLen {
		if syms, ok := c.Get(cacheKey(prefix, cwd)); ok {
			return syms, true
		}
		prefix = prefix[:len(prefix)-1]
	}
	return nil, false
}

func (c *symCache) Get(key string) ([]*symbol.Symbol, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(elem)
	return elem.Value.(*symCacheEntry).syms, true
}

func (c *symCache) Put(prefix, cwd string, syms []*symbol.Symbol) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := cacheKey(prefix, cwd)
	if elem, ok := c.items[key]; ok {
		elem.Value.(*symCacheEntry).syms = syms
		c.ll.MoveToFront(elem)
		return
	}
	elem := c.ll.PushFront(&symCacheEntry{key: key, syms: syms})
	c.items[key] = elem
	if c.ll.Len() > c.max {
		elem = c.ll.Back()
		delete(c.items, elem.Value.(*symCacheEntry).key)
		c.ll.Remove(elem)
	}
}

func cacheKey(prefix, cwd string) string {
	return prefix + "\x00" + cwd
}
