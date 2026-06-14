package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neovim/go-client/msgpack/rpc"
	"github.com/samiulsami/go-deep.nvim/go/complete"
	"github.com/samiulsami/go-deep.nvim/go/gopls"
	"github.com/samiulsami/go-deep.nvim/go/index"
	"github.com/samiulsami/go-deep.nvim/go/pool"
)

type serveConfig struct {
	Cache              bool
	Index              bool
	IndexDBPath        string
	MinPrefixLength    int
	MaxItems           int
	MaxFromSamePackage int
	WorkspaceTimeout   int
	ExcludeImported    bool
	ExcludeVendored    bool
	ExcludeInternal    bool
	ExcludeTestFiles   bool
}

type serveRequest struct {
	RequestID       uint64                   `msgpack:"request_id"`
	Prefix          string                   `msgpack:"prefix"`
	Filepath        string                   `msgpack:"filepath"`
	CWD             string                   `msgpack:"cwd"`
	ImportedPaths   map[string]string        `msgpack:"imported_paths"`
	WarmOnly        bool                     `msgpack:"warm_only"`
	MinPrefixLength *int                     `msgpack:"min_prefix_length"`
	Options         *complete.ProcessOptions `msgpack:"options"`
}

type replyPayload struct {
	RequestID uint64                    `msgpack:"request_id"`
	Items     []complete.CompletionItem `msgpack:"items"`
}

type completionFetch struct {
	id        uint64
	requestID uint64
	e         *rpc.Endpoint
	req       serveRequest
	cReq      complete.Request
	comp      *complete.DefaultCompleter
}

type serveHandler struct {
	ctx       context.Context
	cfg       serveConfig
	provider  *complete.Provider
	options   complete.ProcessOptions
	reqID     atomic.Uint64
	fetchPool *pool.Pool

	projectSymbolCache complete.SymbolStore
	stdlibCache        complete.SymbolMatcher

	workspaceMu             sync.Mutex
	workspaceCWD            string
	clearProjectSymbolCache func()
}

func defaultServeConfig() serveConfig {
	return serveConfig{
		Cache:              true,
		Index:              true,
		IndexDBPath:        "",
		MinPrefixLength:    3,
		MaxItems:           30,
		MaxFromSamePackage: 4,
		WorkspaceTimeout:   15,
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
	fs.BoolVar(&cfg.Cache, "cache", cfg.Cache, "use in-memory cache for completion")
	fs.BoolVar(&cfg.Index, "index", cfg.Index, "crawl and persist default symbols into database")
	fs.StringVar(&cfg.IndexDBPath, "index-db-path", cfg.IndexDBPath, "path to persistent stdlib symbol database")
	fs.IntVar(&cfg.MinPrefixLength, "min-prefix-length", cfg.MinPrefixLength, "minimum prefix length before querying")
	fs.IntVar(&cfg.MaxItems, "max-items", cfg.MaxItems, "maximum completion items returned")
	fs.IntVar(&cfg.MaxFromSamePackage, "max-from-same-package", cfg.MaxFromSamePackage, "maximum completion items from the same package per query (0 = unlimited)")
	fs.IntVar(&cfg.WorkspaceTimeout, "workspace-timeout", cfg.WorkspaceTimeout, "workspace query timeout in seconds")
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

	log.Printf("serve: cache=%v index=%v indexDBPath=%q minPrefix=%d maxItems=%d maxFromSamePackage=%d workspaceTimeout=%ds workers=%d excludeImported=%v excludeVendored=%v excludeInternal=%v excludeTestFiles=%v",
		cfg.Cache, cfg.Index, cfg.IndexDBPath,
		cfg.MinPrefixLength, cfg.MaxItems, cfg.MaxFromSamePackage, cfg.WorkspaceTimeout, workers,
		cfg.ExcludeImported, cfg.ExcludeVendored, cfg.ExcludeInternal, cfg.ExcludeTestFiles)

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getcwd: %w", err)
	}
	if cfg.Index && cfg.IndexDBPath == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return fmt.Errorf("user cache dir: %w", err)
		}
		cfg.IndexDBPath = cacheDir + "/go_deep/go_deep.gob"
	}

	e, err := rpc.NewEndpoint(os.Stdin, stdout, stdout, rpc.WithLogf(log.Printf))
	if err != nil {
		return err
	}

	wsClient, err := gopls.NewGoplsManager(ctx, cwd)
	if err != nil {
		return fmt.Errorf("workspace client: %w", err)
	}

	var projectSymbolCache complete.SymbolStore
	var clearProjectSymbolCache func()
	if cfg.Cache {
		cache := complete.NewProjectSymbolCache(0)
		projectSymbolCache = cache
		clearProjectSymbolCache = cache.Clear
	}

	var stdlibCache complete.SymbolMatcher
	if cfg.Index {
		idx, err := index.NewIndex(ctx, index.IndexConfig{Enabled: true, Path: cfg.IndexDBPath, Source: index.NewGoStdlibSource()})
		if err != nil {
			return fmt.Errorf("index: %w", err)
		}
		stdlibCache = idx
	}

	opts := complete.ProcessOptions{
		MaxItems:           cfg.MaxItems,
		MaxFromSamePackage: cfg.MaxFromSamePackage,
		ExcludeImported:    cfg.ExcludeImported,
		ExcludeVendored:    cfg.ExcludeVendored,
		ExcludeInternal:    cfg.ExcludeInternal,
		ExcludeTestFiles:   cfg.ExcludeTestFiles,
	}

	goProvider, err := complete.NewProvider(wsClient)
	if err != nil {
		return err
	}

	comp := complete.NewDefaultWithProvider(projectSymbolCache, stdlibCache, goProvider)

	h := &serveHandler{
		ctx:                     ctx,
		cfg:                     cfg,
		provider:                goProvider,
		options:                 opts,
		fetchPool:               fetchPool,
		projectSymbolCache:      projectSymbolCache,
		stdlibCache:             stdlibCache,
		workspaceCWD:            cwd,
		clearProjectSymbolCache: clearProjectSymbolCache,
	}
	serveErr := h.serve(e, comp)

	if closeErr := wsClient.Close(); closeErr != nil {
		log.Printf("workspace client close error: %v", closeErr)
	}

	return serveErr
}

func (h *serveHandler) serve(e *rpc.Endpoint, comp *complete.DefaultCompleter) error {
	if err := e.Register("symbols", func(e *rpc.Endpoint, req serveRequest) {
		go h.handleSymbols(e, req, comp)
	}, e); err != nil {
		return err
	}
	log.Printf("rpc server ready, awaiting requests")
	return e.Serve()
}

func (h *serveHandler) handleSymbols(e *rpc.Endpoint, req serveRequest, comp *complete.DefaultCompleter) {
	id := h.reqID.Add(1)
	h.maybeSwitchWorkspace(req.CWD)
	log.Printf("[%d] symbols: prefix=%q file=%s cwd=%s warmOnly=%v",
		id, req.Prefix, req.Filepath, req.CWD, req.WarmOnly)

	minPrefix := h.cfg.MinPrefixLength
	if req.MinPrefixLength != nil {
		minPrefix = *req.MinPrefixLength
	}
	if len(req.Prefix) < minPrefix {
		return
	}

	effectiveOpts := h.options
	if req.Options != nil {
		effectiveOpts = *req.Options
	}

	cReq := complete.Request{
		Prefix:          req.Prefix,
		Filepath:        req.Filepath,
		CWD:             req.CWD,
		ImportedPaths:   req.ImportedPaths,
		MinPrefixLength: minPrefix,
		Options:         effectiveOpts,
	}

	fetch := completionFetch{id: id, requestID: req.RequestID, e: e, req: req, cReq: cReq, comp: comp}

	if req.WarmOnly {
		h.fetchPool.Submit(func() { h.handleCompletionFetch(h.ctx, fetch) })
		return
	}

	h.fetchPool.Submit(func() { h.handleCompletionFetch(h.ctx, fetch) })
}

func (h *serveHandler) handleCompletionFetch(ctx context.Context, job completionFetch) {
	fetchCtx, cancel := context.WithTimeout(ctx, time.Duration(h.cfg.WorkspaceTimeout)*time.Second)
	defer cancel()
	if job.req.WarmOnly {
		if err := job.comp.WarmupCache(fetchCtx, job.cReq); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			log.Printf("[%d] workspace fetch error: %v", job.id, err)
		}
		return
	}

	result, err := job.comp.FetchAndComplete(fetchCtx, job.cReq)
	if err != nil {
		log.Printf("[%d] fetch and complete failed: %v", job.id, err)
		return
	}
	if len(result.Items) > 0 {
		log.Printf("[%d] fetched: %d items", job.id, len(result.Items))
		h.sendSymbols(job.id, job.e, job.requestID, result.Items)
	}
}

func (h *serveHandler) maybeSwitchWorkspace(cwd string) {
	h.workspaceMu.Lock()
	defer h.workspaceMu.Unlock()
	if cwd == "" || cwd == h.workspaceCWD {
		return
	}
	h.workspaceCWD = cwd
	if h.clearProjectSymbolCache != nil {
		h.clearProjectSymbolCache()
	}
}

func (h *serveHandler) sendSymbols(id uint64, e *rpc.Endpoint, requestID uint64, items []complete.CompletionItem) {
	reply := replyPayload{RequestID: requestID, Items: items}
	if err := e.Call("nvim_call_function", nil, "luaeval", []any{"require('go_deep.client')._dispatch(_A[1])", []any{reply}}); err != nil {
		log.Printf("[%d] dispatch call failed: %v", id, err)
	}
}
