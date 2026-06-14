package index

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
	"golang.org/x/sync/errgroup"
)

const (
	stdlibIndexProgressLogInterval = 2 * time.Second
)

type stdPackage struct {
	ImportPath   string
	Name         string
	Dir          string
	GoFiles      []string
	CgoFiles     []string
	TestGoFiles  []string
	XTestGoFiles []string
}

func crawlStdlib(ctx context.Context) ([]*symbol.Symbol, error) {
	pkgs, err := listStdPackages(ctx)
	if err != nil {
		return nil, err
	}

	packages := make([]stdPackage, 0, len(pkgs))
	for _, pkg := range pkgs {
		if containsPathComponent(pkg.ImportPath, "internal") || containsPathComponent(pkg.ImportPath, "vendor") {
			continue
		}
		packages = append(packages, pkg)
	}
	total := len(packages)

	log.Printf("index: indexing %d stdlib packages", total)
	startedAt := time.Now()

	var currentPackage atomic.Int64
	var currentSymbols atomic.Int64

	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(stdlibIndexProgressLogInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Printf(
					"index: still indexing stdlib (%d/%d packages, %d symbols, %s)",
					currentPackage.Load(),
					total,
					currentSymbols.Load(),
					time.Since(startedAt).Round(time.Millisecond),
				)
			}
		}
	}()

	if total == 0 {
		return nil, nil
	}

	workers := max(1, runtime.GOMAXPROCS(0))

	rowsByPackage := make([][]*symbol.Symbol, total)
	g, crawlCtx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i, pkg := range packages {
		g.Go(func() error {
			if crawlCtx.Err() != nil {
				return crawlCtx.Err()
			}

			rows, err := crawlPackage(pkg)
			if err != nil {
				return err
			}

			rowsByPackage[i] = rows
			processed := currentPackage.Add(1)
			symbols := currentSymbols.Add(int64(len(rows)))
			if processed%100 == 0 || processed == int64(total) {
				log.Printf("index: indexed %d/%d stdlib packages (%d symbols, %s)", processed, total, symbols, time.Since(startedAt).Round(time.Millisecond))
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	out := make([]*symbol.Symbol, 0, currentSymbols.Load())
	for _, rows := range rowsByPackage {
		out = append(out, rows...)
	}
	return out, nil
}

func listStdPackages(ctx context.Context) ([]stdPackage, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-json", "std")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list std: %w", err)
	}
	var pkgs []stdPackage
	if err := json.Unmarshal(out, &pkgs); err != nil {
		return nil, err
	}
	return pkgs, nil
}

func crawlPackage(pkg stdPackage) ([]*symbol.Symbol, error) {
	fset := token.NewFileSet()
	var out []*symbol.Symbol
	files := append([]string{}, pkg.GoFiles...)
	files = append(files, pkg.CgoFiles...)
	for _, name := range files {
		syms, err := crawlFile(fset, pkg, name)
		if err != nil {
			return nil, err
		}
		out = append(out, syms...)
	}
	return out, nil
}

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
