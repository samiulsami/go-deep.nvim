package index

import (
	"context"
	"encoding/json"
	"fmt"
	"go/token"
	"log"
	"os/exec"
	"runtime"
	"strings"
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
		if IsInternalImportPath(pkg.ImportPath) || isVendorImportPath(pkg.ImportPath) {
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
	dec := json.NewDecoder(strings.NewReader(string(out)))
	var pkgs []stdPackage
	for dec.More() {
		var pkg stdPackage
		if err := dec.Decode(&pkg); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, pkg)
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
