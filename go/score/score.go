package score

import (
	"container/heap"
	"sort"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

// Fuzzy scoring algorithm adapted from junegunn/fzf.
// See: https://github.com/junegunn/fzf/blob/master/src/algo/algo.go

const (
	ccNone  byte = 0
	ccLower byte = 1
	ccUpper byte = 2
	ccDigit byte = 3
)

var (
	charClass [256]byte
	foldTable [256]byte
)

func init() {
	for i := range 256 {
		foldTable[i] = byte(i)
	}
	for c := 'A'; c <= 'Z'; c++ {
		charClass[c] = ccUpper
		foldTable[c] = byte(c + ('a' - 'A'))
	}
	for c := 'a'; c <= 'z'; c++ {
		charClass[c] = ccLower
	}
	for c := '0'; c <= '9'; c++ {
		charClass[c] = ccDigit
	}
}

const (
	scoreMatch       = 8
	scoreConsecutive = 12
	scoreBoundary    = 10
	scoreGapOpen     = -3
	scoreGapExtend   = -1
	scoreExact       = 20
	scorePrefix      = 12
)

func boundaryBonus(haystack string, idx int) int {
	if idx == 0 {
		return scorePrefix
	}
	prev := charClass[haystack[idx-1]]
	cur := charClass[haystack[idx]]
	if prev == ccNone {
		return scoreBoundary
	}
	if prev == ccLower && cur == ccUpper {
		return scoreBoundary
	}
	return 0
}

func Score(query, haystack string) int {
	if query == "" || len(query) > len(haystack) {
		return 0
	}

	score := 0
	qi := 0
	firstMatch := -1
	lastMatch := -1
	streak := 0

	for hi := 0; hi < len(haystack) && qi < len(query); hi++ {
		if foldTable[query[qi]] != foldTable[haystack[hi]] {
			continue
		}

		if firstMatch < 0 {
			firstMatch = hi
		} else {
			gap := hi - lastMatch - 1
			if gap > 0 {
				score += scoreGapOpen + gap*scoreGapExtend
			}
		}

		bonus := scoreMatch + boundaryBonus(haystack, hi)
		if lastMatch == hi-1 {
			streak++
			bonus += scoreConsecutive + streak*2
		} else {
			streak = 0
		}

		if query[qi] == haystack[hi] {
			bonus++
		}

		score += bonus
		lastMatch = hi
		qi++
	}
	if qi != len(query) {
		return 0
	}

	score -= firstMatch
	score -= len(haystack) - lastMatch - 1
	if len(query) == len(haystack) {
		eq := true
		for i := 0; i < len(query); i++ {
			if foldTable[query[i]] != foldTable[haystack[i]] {
				eq = false
				break
			}
		}
		if eq {
			score += scoreExact
		}
	}

	if score < 1 {
		score = 1
	}
	return score
}

type ScoredItem struct {
	Symbol *symbol.Symbol
	Score  int
}

type rankHeap struct {
	data []ScoredItem
}

func (h *rankHeap) Len() int      { return len(h.data) }
func (h *rankHeap) Swap(i, j int) { h.data[i], h.data[j] = h.data[j], h.data[i] }
func (h *rankHeap) Push(x any)    { h.data = append(h.data, x.(ScoredItem)) }
func (h *rankHeap) Pop() any {
	old := h.data
	n := len(old)
	x := old[n-1]
	h.data = old[:n-1]
	return x
}

func (h *rankHeap) Less(i, j int) bool {
	return betterByScoreImportPathName(h.data[j], h.data[i])
}

type RankOpts struct {
	Query string
	Limit int
}

func betterByScoreImportPathName(a, b ScoredItem) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.Symbol.ImportPath != b.Symbol.ImportPath {
		return a.Symbol.ImportPath < b.Symbol.ImportPath
	}
	return a.Symbol.Name < b.Symbol.Name
}

func Match(opts RankOpts, lists ...[]*symbol.Symbol) []*symbol.Symbol {
	n := opts.Limit
	if n <= 0 || opts.Query == "" {
		return nil
	}

	totalLen := 0
	for _, l := range lists {
		totalLen += len(l)
	}
	if totalLen == 0 {
		return nil
	}

	h := &rankHeap{
		data: make([]ScoredItem, 0, n),
	}

	for _, list := range lists {
		for _, sym := range list {
			sc := Score(opts.Query, sym.Haystack)
			if sc <= 0 {
				continue
			}
			item := ScoredItem{Symbol: sym, Score: sc}
			if h.Len() < n {
				heap.Push(h, item)
				continue
			}
			if !betterByScoreImportPathName(item, h.data[0]) {
				continue
			}
			h.data[0] = item
			heap.Fix(h, 0)
		}
	}

	sort.Slice(h.data, func(i, j int) bool {
		return betterByScoreImportPathName(h.data[i], h.data[j])
	})

	out := make([]*symbol.Symbol, len(h.data))
	for i := range h.data {
		out[i] = h.data[i].Symbol
	}
	return out
}
