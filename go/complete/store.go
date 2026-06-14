package complete

import (
	"math/rand/v2"
	"sync"

	"github.com/samiulsami/go-deep.nvim/go/score"
	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

const defaultProjectSymbolCacheCapacity = 10_000

type ProjectSymbolCache struct {
	mu       sync.RWMutex
	symbols  []*symbol.Symbol
	items    map[string]int
	capacity int
}

func NewProjectSymbolCache(capacity int) *ProjectSymbolCache {
	if capacity <= 0 {
		capacity = defaultProjectSymbolCacheCapacity
	}
	return &ProjectSymbolCache{
		symbols:  make([]*symbol.Symbol, 0, capacity),
		items:    make(map[string]int, capacity),
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
	if idx, ok := c.items[dedupeKey]; ok {
		c.symbols[idx] = s
		return
	}
	if len(c.symbols) >= c.capacity {
		c.evictLocked()
	}
	idx := len(c.symbols)
	c.symbols = append(c.symbols, s)
	c.items[dedupeKey] = idx
}

func (c *ProjectSymbolCache) StoreBatch(syms []*symbol.Symbol) {
	if len(syms) == 0 {
		return
	}
	c.mu.Lock()
	for _, s := range syms {
		c.storeLocked(s)
	}
	c.mu.Unlock()
}

func (c *ProjectSymbolCache) evictLocked() {
	if len(c.symbols) == 0 {
		return
	}
	i := rand.IntN(len(c.symbols))
	victim := c.symbols[i]
	delete(c.items, c.symbolDedupKey(victim))
	last := len(c.symbols) - 1
	if i != last {
		c.symbols[i] = c.symbols[last]
		c.items[c.symbolDedupKey(c.symbols[i])] = i
	}
	c.symbols = c.symbols[:last]
}

func (c *ProjectSymbolCache) Match(query string, n int) []*symbol.Symbol {
	if n <= 0 || query == "" {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return score.Rank(score.RankOpts{
		Query:   query,
		Limit:   n,
		Symbols: c.symbols,
	})
}

func (c *ProjectSymbolCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.symbols = c.symbols[:0]
	clear(c.items)
}
