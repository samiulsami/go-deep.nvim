package complete

import (
	"context"
	"fmt"
	"strings"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

const completionCandidateOverfetchFactor = 4

type ProcessOptions struct {
	MaxItems           int  `msgpack:"max_items"`
	MaxFromSamePackage int  `msgpack:"max_from_same_package"`
	ExcludeImported    bool `msgpack:"exclude_imported"`
	ExcludeVendored    bool `msgpack:"exclude_vendored"`
	ExcludeInternal    bool `msgpack:"exclude_internal"`
	ExcludeTestFiles   bool `msgpack:"exclude_test_files"`
}

type Request struct {
	Prefix          string
	Filepath        string
	CWD             string
	ImportedPaths   map[string]string
	MinPrefixLength int
	Options         ProcessOptions
}

type CompletionItem struct {
	Word     string `msgpack:"word"`
	Abbr     string `msgpack:"abbr"`
	Menu     string `msgpack:"menu"`
	Info     string `msgpack:"info"`
	Kind     string `msgpack:"kind"`
	ICase    int    `msgpack:"icase"`
	Dup      int    `msgpack:"dup"`
	UserData string `msgpack:"user_data"`
}

type SymbolMatcher interface {
	Match(query string, n int) ([]*symbol.Symbol, error)
}

type SymbolStore interface {
	Match(query string, n int) ([]*symbol.Symbol, error)
	StoreBatch(syms []*symbol.Symbol)
}

type Result struct {
	Items []CompletionItem
}

type processContext struct {
	request       Request
	seenSymbols   map[string]bool
	packageCounts map[string]int
	includeSymbol func(Request, *symbol.Symbol) bool
}

func newProcessContext(req Request, includeSymbol func(Request, *symbol.Symbol) bool) processContext {
	return processContext{
		request:       req,
		seenSymbols:   make(map[string]bool),
		packageCounts: make(map[string]int),
		includeSymbol: includeSymbol,
	}
}

func symbolMatches(ctx processContext, s *symbol.Symbol, requirePrefix bool) bool {
	if s == nil {
		return false
	}
	if requirePrefix {
		if !strings.HasPrefix(strings.ToLower(s.Name), strings.ToLower(ctx.request.Prefix)) {
			return false
		}
	}
	if ctx.includeSymbol == nil {
		return false
	}
	return ctx.includeSymbol(ctx.request, s)
}

func processSymbolBatch(ctx processContext, candidates []*symbol.Symbol, result []*symbol.Symbol) []*symbol.Symbol {
	matched := result
	for _, requirePrefix := range []bool{true, false} {
		matched = appendFilteredSymbols(ctx, matched, candidates, requirePrefix)
		if len(matched) >= ctx.request.Options.MaxItems {
			return matched
		}
	}
	return matched
}

func filterSymbols(procCtx processContext, candidates []*symbol.Symbol) []*symbol.Symbol {
	return appendFilteredSymbols(procCtx, nil, candidates, false)
}

func appendFilteredSymbols(ctx processContext, result []*symbol.Symbol, candidates []*symbol.Symbol, requirePrefix bool) []*symbol.Symbol {
	matched := result
	for _, s := range candidates {
		if len(matched) >= ctx.request.Options.MaxItems {
			break
		}
		hash := symbol.Hash(s)
		if ctx.seenSymbols[hash] {
			continue
		}
		if !symbolMatches(ctx, s, requirePrefix) {
			continue
		}
		if ctx.request.Options.MaxFromSamePackage > 0 && ctx.packageCounts[s.ImportPath] >= ctx.request.Options.MaxFromSamePackage {
			continue
		}
		ctx.seenSymbols[hash] = true
		ctx.packageCounts[s.ImportPath]++
		matched = append(matched, s)
	}
	return matched
}

type DefaultCompleter struct {
	projectSymbolCache SymbolStore
	stdlibCache        SymbolMatcher
	provider           *Provider
}

func NewDefaultWithProvider(
	projectSymbolCache SymbolStore,
	stdlibCache SymbolMatcher,
	provider *Provider,
) *DefaultCompleter {
	return &DefaultCompleter{
		projectSymbolCache: projectSymbolCache,
		stdlibCache:        stdlibCache,
		provider:           provider,
	}
}

func (c *DefaultCompleter) validate(req Request) (*Provider, error) {
	if len(req.Prefix) < req.MinPrefixLength {
		return nil, nil
	}
	if c.provider == nil {
		return nil, fmt.Errorf("completer: nil provider")
	}
	if !c.provider.ValidPrefix(req.Prefix) {
		return nil, nil
	}
	if req.Options.MaxItems <= 0 {
		return nil, nil
	}
	return c.provider, nil
}

func (c *DefaultCompleter) Complete(req Request) (Result, error) {
	prov, err := c.validate(req)
	if err != nil {
		return Result{}, err
	}
	if prov == nil {
		return Result{}, nil
	}

	n := req.Options.MaxItems * completionCandidateOverfetchFactor

	var candidates []*symbol.Symbol
	if c.stdlibCache != nil {
		matched, err := c.stdlibCache.Match(req.Prefix, n)
		if err != nil {
			return Result{}, err
		}
		candidates = append(candidates, matched...)
	}
	if c.projectSymbolCache != nil {
		matched, err := c.projectSymbolCache.Match(req.Prefix, n)
		if err != nil {
			return Result{}, err
		}
		candidates = append(candidates, matched...)
	}
	if len(candidates) == 0 {
		return Result{}, nil
	}

	filtered := filterSymbols(newProcessContext(req, prov.IncludeSymbol), candidates)
	if len(filtered) == 0 {
		return Result{}, nil
	}
	items, err := prov.BuildItems(req, filtered)
	if err != nil {
		return Result{}, err
	}
	return Result{Items: items}, nil
}

func (c *DefaultCompleter) DirectComplete(ctx context.Context, req Request) (Result, error) {
	prov, err := c.validate(req)
	if err != nil {
		return Result{}, err
	}
	if prov == nil {
		return Result{}, nil
	}

	syms, err := c.querySymbols(ctx, req)
	if err != nil {
		return Result{}, err
	}

	filtered := processSymbolBatch(newProcessContext(req, prov.IncludeSymbol), syms, nil)
	if len(filtered) == 0 {
		return Result{}, nil
	}
	items, err := prov.BuildItems(req, filtered)
	if err != nil {
		return Result{}, err
	}
	return Result{Items: items}, nil
}

func (c *DefaultCompleter) WarmupCache(ctx context.Context, req Request) error {
	if c.projectSymbolCache == nil {
		return nil
	}
	syms, err := c.provider.FetchSymbols(ctx, req)
	if err != nil {
		return err
	}
	c.projectSymbolCache.StoreBatch(syms)
	return nil
}

func (c *DefaultCompleter) querySymbols(ctx context.Context, req Request) ([]*symbol.Symbol, error) {
	var indexed []*symbol.Symbol
	if c.stdlibCache != nil {
		var err error
		indexed, err = c.stdlibCache.Match(req.Prefix, req.Options.MaxItems*completionCandidateOverfetchFactor)
		if err != nil {
			return nil, err
		}
	}
	workspaceSyms, err := c.provider.FetchSymbols(ctx, req)
	if err != nil {
		if len(indexed) > 0 {
			return indexed, nil
		}
		return nil, err
	}
	return append(indexed, workspaceSyms...), nil
}
