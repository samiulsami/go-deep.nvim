package complete

import (
	"strings"

	"github.com/samiulsami/go-deep.nvim/go/score"
	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

type ProcessOptions struct {
	MaxItems           int  `msgpack:"max_items"`
	MaxFromSamePackage int  `msgpack:"max_from_same_package"`
	WorkspaceSymbols   bool `msgpack:"workspace_symbols"`
	ExcludeImported    bool `msgpack:"exclude_imported"`
	ExcludeVendored    bool `msgpack:"exclude_vendored"`
	ExcludeInternal    bool `msgpack:"exclude_internal"`
	ExcludeTestFiles   bool `msgpack:"exclude_test_files"`
}

type Request struct {
	RequestID     uint64            `msgpack:"request_id"`
	Prefix        string            `msgpack:"prefix"`
	Filepath      string            `msgpack:"filepath"`
	CWD           string            `msgpack:"cwd"`
	ImportedPaths map[string]string `msgpack:"imported_paths"`
	Options       *ProcessOptions   `msgpack:"options"`
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

func Build(req Request, seenHashes map[string]struct{}, lists ...[]*symbol.Symbol) []CompletionItem {
	var opts ProcessOptions
	if req.Options != nil {
		opts = *req.Options
	}

	ranked := score.Match(score.RankOpts{Query: req.Prefix, Limit: opts.MaxItems}, lists...)
	filtered := filterSymbols(ranked, opts, req.Filepath, req.ImportedPaths, seenHashes)
	return buildItems(req, filtered)
}

func filterSymbols(symbols []*symbol.Symbol, opts ProcessOptions, bufPath string, importedPaths map[string]string, seen map[string]struct{}) []*symbol.Symbol {
	if seen == nil {
		seen = make(map[string]struct{})
	}
	pkgCounts := make(map[string]int)
	matched := make([]*symbol.Symbol, 0)
	for _, s := range symbols {
		if s == nil {
			continue
		}
		if opts.ExcludeImported && importedPaths[s.ImportPath] != "" {
			continue
		}
		if s.Location.Path == bufPath {
			continue
		}
		if opts.ExcludeTestFiles && strings.HasSuffix(s.Location.Path, "_test.go") {
			continue
		}
		if opts.ExcludeVendored && s.IsVendored {
			continue
		}
		if opts.ExcludeInternal {
			if IsInternalImportPath(s.ImportPath) && (!s.IsLocal || !canImportInternal(bufPath, s.Location.Path)) {
				continue
			}
		}
		hash := symbol.Hash(s)
		if _, ok := seen[hash]; ok {
			continue
		}
		if opts.MaxFromSamePackage > 0 && pkgCounts[s.ImportPath] >= opts.MaxFromSamePackage {
			continue
		}
		seen[hash] = struct{}{}
		pkgCounts[s.ImportPath]++
		matched = append(matched, s)
	}
	return matched
}

func containsPathComponent(path, component string) bool {
	return path == component ||
		strings.HasPrefix(path, component+"/") ||
		strings.Contains(path, "/"+component+"/") ||
		strings.HasSuffix(path, "/"+component)
}

func IsInternalImportPath(path string) bool {
	return containsPathComponent(path, "internal")
}
