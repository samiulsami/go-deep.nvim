package index

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

func crawlFile(fset *token.FileSet, pkg stdPackage, name string) ([]*symbol.Symbol, error) {
	path := filepath.Join(pkg.Dir, name)
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []*symbol.Symbol
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if s := collectFuncSymbol(d, fset, pkg, path); s != nil {
				out = append(out, s)
			}
		case *ast.GenDecl:
			out = append(out, collectGenDeclSymbols(d, fset, pkg, path)...)
		}
	}
	return out, nil
}

func collectFuncSymbol(d *ast.FuncDecl, fset *token.FileSet, pkg stdPackage, path string) *symbol.Symbol {
	if d.Recv != nil || !d.Name.IsExported() {
		return nil
	}
	pos := fset.Position(d.Name.Pos())
	sym := &symbol.Symbol{
		Name:        d.Name.Name,
		ImportPath:  pkg.ImportPath,
		PackageName: pkg.Name,
		Kind:        symbol.FunctionKind,
		Location: symbol.Location{
			Path: path,
			Range: symbol.Range{
				Start: symbol.Position{Line: pos.Line - 1, Character: pos.Column - 1},
				End:   symbol.Position{Line: pos.Line - 1, Character: pos.Column - 1},
			},
		},
	}
	sym.Haystack = symbol.BuildHaystack(sym)
	return sym
}

func collectGenDeclSymbols(d *ast.GenDecl, fset *token.FileSet, pkg stdPackage, path string) []*symbol.Symbol {
	var out []*symbol.Symbol
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if sym := collectTypeSpecSymbol(s, fset, pkg, path); sym != nil {
				out = append(out, sym)
			}
		case *ast.ValueSpec:
			out = append(out, collectValueSpecSymbols(s, fset, pkg, path, d.Tok)...)
		}
	}
	return out
}

func collectTypeSpecSymbol(s *ast.TypeSpec, fset *token.FileSet, pkg stdPackage, path string) *symbol.Symbol {
	if !s.Name.IsExported() {
		return nil
	}
	pos := fset.Position(s.Name.Pos())
	sym := &symbol.Symbol{
		Name:        s.Name.Name,
		ImportPath:  pkg.ImportPath,
		PackageName: pkg.Name,
		Kind:        typeSpecKind(s),
		Location: symbol.Location{
			Path: path,
			Range: symbol.Range{
				Start: symbol.Position{Line: pos.Line - 1, Character: pos.Column - 1},
				End:   symbol.Position{Line: pos.Line - 1, Character: pos.Column - 1},
			},
		},
	}
	sym.Haystack = symbol.BuildHaystack(sym)
	return sym
}

func collectValueSpecSymbols(s *ast.ValueSpec, fset *token.FileSet, pkg stdPackage, path string, tok token.Token) []*symbol.Symbol {
	kind := symbol.VariableKind
	if tok == token.CONST {
		kind = symbol.ConstantKind
	}
	var out []*symbol.Symbol
	for _, name := range s.Names {
		if !name.IsExported() {
			continue
		}
		pos := fset.Position(name.Pos())
		sym := &symbol.Symbol{
			Name:        name.Name,
			ImportPath:  pkg.ImportPath,
			PackageName: pkg.Name,
			Kind:        kind,
			Location: symbol.Location{
				Path: path,
				Range: symbol.Range{
					Start: symbol.Position{Line: pos.Line - 1, Character: pos.Column - 1},
					End:   symbol.Position{Line: pos.Line - 1, Character: pos.Column - 1},
				},
			},
		}
		sym.Haystack = symbol.BuildHaystack(sym)
		out = append(out, sym)
	}
	return out
}

func typeSpecKind(spec *ast.TypeSpec) symbol.Kind {
	switch spec.Type.(type) {
	case *ast.InterfaceType:
		return symbol.InterfaceKind
	case *ast.StructType:
		return symbol.StructKind
	default:
		return symbol.TypeKind
	}
}
