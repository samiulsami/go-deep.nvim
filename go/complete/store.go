package complete

import (
	"container/list"
	"sync"

	"github.com/samiulsami/go-deep.nvim/go/score"
	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

const defaultProjectSymbolCacheCapacity = 10_000

type cacheEntry struct {
	key    string
	symbol *symbol.Symbol
}

type ProjectSymbolCache struct {
	mu       sync.Mutex
	items    map[string]*list.Element
	recent   *list.List
	capacity int
}

func NewProjectSymbolCache(capacity int) *ProjectSymbolCache {
	if capacity <= 0 {
		capacity = defaultProjectSymbolCacheCapacity
	}
	return &ProjectSymbolCache{
		items:    make(map[string]*list.Element, capacity),
		recent:   list.New(),
		capacity: capacity,
	}
}

func (c *ProjectSymbolCache) symbolDedupKey(s *symbol.Symbol) string {
	if s == nil {
		return ""
	}
	return s.Name + "#" + s.ImportPath
}

func (c *ProjectSymbolCache) storeLocked(s *symbol.Symbol) {
	if s == nil {
		return
	}
	dedupeKey := c.symbolDedupKey(s)
	if dedupeKey == "" {
		return
	}
	if elem, ok := c.items[dedupeKey]; ok {
		entry := elem.Value.(*cacheEntry)
		entry.symbol = s
		c.recent.MoveToFront(elem)
		return
	}
	if len(c.items) >= c.capacity {
		c.evictLocked()
	}
	elem := c.recent.PushFront(&cacheEntry{key: dedupeKey, symbol: s})
	c.items[dedupeKey] = elem
}

func (c *ProjectSymbolCache) StoreBatch(syms []*symbol.Symbol) {
	if len(syms) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range syms {
		c.storeLocked(s)
	}
}

func (c *ProjectSymbolCache) evictLocked() {
	if len(c.items) == 0 {
		return
	}
	victim := c.recent.Back()
	if victim == nil {
		return
	}
	entry := victim.Value.(*cacheEntry)
	delete(c.items, entry.key)
	c.recent.Remove(victim)
}

func (c *ProjectSymbolCache) Match(query string, n int) []*symbol.Symbol {
	if n <= 0 || query == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	syms := make([]*symbol.Symbol, 0, len(c.items))
	for elem := c.recent.Front(); elem != nil; elem = elem.Next() {
		syms = append(syms, elem.Value.(*cacheEntry).symbol)
	}
	matched := score.Rank(score.RankOpts{
		Query:   query,
		Limit:   n,
		Symbols: syms,
	})
	for _, sym := range matched {
		if elem, ok := c.items[c.symbolDedupKey(sym)]; ok {
			c.recent.MoveToFront(elem)
		}
	}
	return matched
}

func (c *ProjectSymbolCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recent.Init()
	clear(c.items)
}
