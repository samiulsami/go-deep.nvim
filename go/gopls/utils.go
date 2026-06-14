package gopls

import (
	"go/ast"
	"path/filepath"
	"strings"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

func ConvertLSPSymbolToSymbol(raw *LspSymbol, cwd string) (*symbol.Symbol, bool) {
	if raw == nil || !symbol.SupportedKind(raw.Kind) {
		return nil, false
	}
	name := raw.Name
	if dot := strings.LastIndex(name, "."); dot >= 0 && dot < len(name)-1 {
		name = name[dot+1:]
	}
	if name == "" || !ast.IsExported(name) {
		return nil, false
	}
	uri := raw.Location.URI
	if !strings.HasPrefix(uri, "file://") {
		return nil, false
	}
	path := filepath.Clean(strings.TrimPrefix(uri, "file://"))
	cwd = filepath.Clean(cwd)
	isLocal := false
	if cwd != "." && cwd != "" {
		if rel, err := filepath.Rel(cwd, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			isLocal = true
		}
	}
	isVendored := strings.Contains(path, string(filepath.Separator)+"vendor"+string(filepath.Separator))
	sym := &symbol.Symbol{
		Name:        name,
		ImportPath:  raw.ContainerName,
		PackageName: "",
		Kind:        raw.Kind,
		IsLocal:     isLocal && !isVendored,
		IsVendored:  isVendored,
		Location: symbol.Location{
			Path:  path,
			Range: raw.Location.Range,
		},
	}
	sym.Haystack = symbol.BuildHaystack(sym)
	return sym, true
}
