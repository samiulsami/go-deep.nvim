package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/neovim/go-client/msgpack/rpc"
	"github.com/samiulsami/go-deep.nvim/go/complete"
	"github.com/samiulsami/go-deep.nvim/go/gopls"
	"github.com/samiulsami/go-deep.nvim/go/index"
	"github.com/samiulsami/go-deep.nvim/go/pool"
	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

type serveConfig struct {
	Index              bool
	IndexFilePath      string
	MaxItems           int
	MaxFromSamePackage int
	WorkspaceTimeout   int
	WorkspaceSymbols   bool
	StdlibSymbols      bool
	ExcludeImported    bool
	ExcludeVendored    bool
	ExcludeInternal    bool
	ExcludeTestFiles   bool
}

type replyPayload struct {
	RequestID uint64                    `msgpack:"request_id"`
	Items     []complete.CompletionItem `msgpack:"items"`
	Final     bool                      `msgpack:"final"`
}

type statusPayload struct {
	IndexReady    bool `msgpack:"index_ready"`
	IndexBuilding bool `msgpack:"index_building"`
	IndexSymbols  int  `msgpack:"index_symbols"`
}

type serveHandler struct {
	ctx          context.Context
	cfg          serveConfig
	goplsManager *gopls.Manager
	options      complete.ProcessOptions
	reqID        atomic.Uint64
	fetchPool    *pool.Pool
	stdlibIndex  *index.Index
}

func defaultServeConfig() serveConfig {
	return serveConfig{
		Index:              true,
		MaxItems:           30,
		MaxFromSamePackage: 4,
		WorkspaceTimeout:   15,
		WorkspaceSymbols:   true,
		StdlibSymbols:      true,
		ExcludeImported:    true,
		ExcludeVendored:    false,
		ExcludeInternal:    true,
		ExcludeTestFiles:   true,
	}
}

func runServe(ctx context.Context, stdout io.WriteCloser, args []string) error {
	cfg := defaultServeConfig()
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&cfg.Index, "index", cfg.Index, "crawl and persist default symbols into database")
	fs.StringVar(&cfg.IndexFilePath, "index-file-path", cfg.IndexFilePath, "path to persistent stdlib symbol index file")
	fs.IntVar(&cfg.MaxItems, "max-items", cfg.MaxItems, "maximum completion items returned")
	fs.IntVar(&cfg.MaxFromSamePackage, "max-from-same-package", cfg.MaxFromSamePackage, "maximum completion items from the same package per query (0 = unlimited)")
	fs.IntVar(&cfg.WorkspaceTimeout, "workspace-timeout", cfg.WorkspaceTimeout, "workspace query timeout in seconds")
	fs.BoolVar(&cfg.WorkspaceSymbols, "workspace-symbols", cfg.WorkspaceSymbols, "include workspace symbols from gopls")
	fs.BoolVar(&cfg.StdlibSymbols, "stdlib-symbols", cfg.StdlibSymbols, "include indexed stdlib symbols")
	fs.BoolVar(&cfg.ExcludeImported, "exclude-imported", cfg.ExcludeImported, "exclude imported packages")
	fs.BoolVar(&cfg.ExcludeVendored, "exclude-vendored", cfg.ExcludeVendored, "exclude vendored packages")
	fs.BoolVar(&cfg.ExcludeInternal, "exclude-internal", cfg.ExcludeInternal, "exclude internal packages per Go's rules")
	fs.BoolVar(&cfg.ExcludeTestFiles, "exclude-test-files", cfg.ExcludeTestFiles, "exclude symbols from *_test.go files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("serve: unexpected args: %v", fs.Args())
	}

	setupSessionLog()

	workers := runtime.GOMAXPROCS(0)
	fetchPool := pool.New(ctx, workers)

	log.Printf("serve: index=%v indexDBPath=%q maxItems=%d maxFromSamePackage=%d workspaceTimeout=%ds workspaceSymbols=%v stdlibSymbols=%v workers=%d excludeImported=%v excludeVendored=%v excludeInternal=%v excludeTestFiles=%v",
		cfg.Index, cfg.IndexFilePath,
		cfg.MaxItems, cfg.MaxFromSamePackage, cfg.WorkspaceTimeout, cfg.WorkspaceSymbols, cfg.StdlibSymbols, workers,
		cfg.ExcludeImported, cfg.ExcludeVendored, cfg.ExcludeInternal, cfg.ExcludeTestFiles)

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getcwd: %w", err)
	}
	if cfg.Index && cfg.IndexFilePath == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return fmt.Errorf("user cache dir: %w", err)
		}
		cfg.IndexFilePath = cacheDir + "/go_deep/go_deep.gob"
	}

	endpoint, err := rpc.NewEndpoint(os.Stdin, stdout, stdout, rpc.WithLogf(log.Printf))
	if err != nil {
		return err
	}

	var goplsManager *gopls.Manager
	if cfg.WorkspaceSymbols {
		mgr, err := gopls.NewManager(ctx, cwd)
		if err != nil {
			return fmt.Errorf("workspace client: %w", err)
		}
		goplsManager = mgr
	}

	var stdlibIndex *index.Index
	if cfg.Index && cfg.StdlibSymbols {
		idx, err := index.NewIndex(ctx, index.IndexConfig{Enabled: true, Path: cfg.IndexFilePath})
		if err != nil {
			return fmt.Errorf("index: %w", err)
		}
		stdlibIndex = idx
	}

	opts := complete.ProcessOptions{
		MaxItems:           cfg.MaxItems,
		MaxFromSamePackage: cfg.MaxFromSamePackage,
		WorkspaceSymbols:   cfg.WorkspaceSymbols,
		StdlibSymbols:      cfg.StdlibSymbols,
		ExcludeImported:    cfg.ExcludeImported,
		ExcludeVendored:    cfg.ExcludeVendored,
		ExcludeInternal:    cfg.ExcludeInternal,
		ExcludeTestFiles:   cfg.ExcludeTestFiles,
	}

	h := &serveHandler{
		ctx:          ctx,
		cfg:          cfg,
		goplsManager: goplsManager,
		options:      opts,
		fetchPool:    fetchPool,
		stdlibIndex:  stdlibIndex,
	}

	return h.serve(endpoint)
}

func (handler *serveHandler) serve(endpoint *rpc.Endpoint) error {
	if err := endpoint.Register("symbols", func(e *rpc.Endpoint, req complete.Request) {
		handler.handleSymbols(e, req)
	}, endpoint); err != nil {
		return err
	}
	if err := endpoint.Register("status", func(e *rpc.Endpoint) (statusPayload, error) {
		return handler.handleStatus(), nil
	}, endpoint); err != nil {
		return err
	}
	log.Printf("rpc server ready, awaiting requests")
	return endpoint.Serve()
}

func (handler *serveHandler) handleSymbols(endpoint *rpc.Endpoint, req complete.Request) {
	id := handler.reqID.Add(1)
	log.Printf("[%d] symbols: prefix=%q file=%s cwd=%s",
		id, req.Prefix, req.Filepath, req.CWD)

	effectiveOpts := handler.options
	if req.Options != nil {
		effectiveOpts = *req.Options
	}

	buildReq := complete.Request{
		RequestID:     req.RequestID,
		Prefix:        req.Prefix,
		Filepath:      req.Filepath,
		CWD:           req.CWD,
		ImportedPaths: req.ImportedPaths,
		Options:       &effectiveOpts,
	}

	workspaceSymbolsEnabled := effectiveOpts.WorkspaceSymbols && handler.goplsManager != nil

	var seenHashes map[string]struct{}
	if effectiveOpts.StdlibSymbols && handler.stdlibIndex != nil {
		if stdlibSymbols := handler.stdlibIndex.Symbols(); len(stdlibSymbols) > 0 {
			if workspaceSymbolsEnabled {
				seenHashes = make(map[string]struct{})
			}
			if items := complete.Build(buildReq, seenHashes, stdlibSymbols); len(items) > 0 {
				log.Printf("[%d] stdlib: %d items", id, len(items))
				handler.sendSymbols(id, endpoint, req.RequestID, items, !workspaceSymbolsEnabled)
			}
		}
	}

	if !workspaceSymbolsEnabled {
		if effectiveOpts.StdlibSymbols && handler.stdlibIndex != nil {
			return
		}
		handler.sendSymbols(id, endpoint, req.RequestID, nil, true)
		return
	}

	handler.fetchPool.Submit(func() {
		fetchCtx, cancel := context.WithTimeout(handler.ctx, time.Duration(handler.cfg.WorkspaceTimeout)*time.Second)
		defer cancel()

		rawWs, err := handler.goplsManager.WorkspaceSymbol(fetchCtx, req.CWD, req.Prefix)
		if err != nil {
			log.Printf("[%d] workspace symbols: %v", id, err)
			handler.sendSymbols(id, endpoint, req.RequestID, nil, true)
			return
		}
		wsSymbols := make([]*symbol.Symbol, 0, len(rawWs))
		for _, raw := range rawWs {
			if sym, ok := gopls.ConvertLSPSymbolToSymbol(raw, req.CWD); ok {
				wsSymbols = append(wsSymbols, sym)
			}
		}
		items := complete.Build(buildReq, seenHashes, wsSymbols)
		log.Printf("[%d] workspace: %d items", id, len(items))
		handler.sendSymbols(id, endpoint, req.RequestID, items, true)
	})
}

func (handler *serveHandler) handleStatus() statusPayload {
	p := statusPayload{}
	if handler.stdlibIndex != nil {
		p.IndexReady = handler.stdlibIndex.Ready()
		p.IndexBuilding = handler.stdlibIndex.Building()
		p.IndexSymbols = handler.stdlibIndex.SymbolCount()
	}
	return p
}

func (handler *serveHandler) sendSymbols(id uint64, e *rpc.Endpoint, requestID uint64, items []complete.CompletionItem, final bool) {
	reply := replyPayload{RequestID: requestID, Items: items, Final: final}
	if err := e.Call("nvim_call_function", nil, "luaeval", []any{"require('go_deep.client')._dispatch(_A[1])", []any{reply}}); err != nil {
		log.Printf("[%d] dispatch call failed: %v", id, err)
	}
}
