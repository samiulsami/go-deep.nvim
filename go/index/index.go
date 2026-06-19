package index

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
)

const (
	schemaVersion             = "v0.0.4"
	statusReasonExisting      = "existing"
	statusReasonMissing       = "missing_or_empty"
	statusReasonSchemaChanged = "schema_changed"
	statusReasonGoEnvChanged  = "go_env_changed"
	statusReasonStaleCheck    = "stale_check_failed"
)

type IndexConfig struct {
	Enabled bool
	Path    string
}

type CacheFingerprint struct {
	Language string
	Provider string
	Tool     string
	Values   map[string]string
}

type CacheFile struct {
	Meta    CacheMeta
	Symbols []*symbol.Symbol
}

type CacheMeta struct {
	SchemaVersion string
	Fingerprint   CacheFingerprint
	IndexedAt     time.Time
	SymbolCount   int
}

type goEnv struct {
	GoVersion  string `json:"GOVERSION"`
	GOROOT     string `json:"GOROOT"`
	GOOS       string `json:"GOOS"`
	GOARCH     string `json:"GOARCH"`
	CGOEnabled string `json:"CGO_ENABLED"`
}

type Index struct {
	cfg      IndexConfig
	symbols  []*symbol.Symbol
	ready    atomic.Bool
	building atomic.Bool
	mu       sync.RWMutex
}

func NewIndex(ctx context.Context, cfg IndexConfig) (*Index, error) {
	idx := &Index{cfg: cfg}
	if !cfg.Enabled || cfg.Path == "" {
		return idx, nil
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return nil, err
	}

	if hasNonEmptyFile(cfg.Path) {
		cache, err := loadCache(cfg.Path)
		if err != nil {
			log.Printf("index: cache load failed, rebuilding: %v", err)
			idx.startBuild(ctx, statusReasonStaleCheck, true)
			return idx, nil
		}
		if err := validateCache(cache); err != nil {
			log.Printf("index: cache validation failed, rebuilding: %v", err)
			idx.startBuild(ctx, statusReasonStaleCheck, true)
			return idx, nil
		}
		current, err := stdlibFingerprint(ctx)
		if err != nil {
			log.Printf("index: fingerprint check failed, rebuilding: %v", err)
			idx.startBuild(ctx, statusReasonStaleCheck, true)
			return idx, nil
		}
		stale, reason := isCacheStale(cache.Meta, current)
		if stale {
			idx.startBuild(ctx, reason, true)
		} else {
			idx.loadFromCache(cache)
			log.Printf("index: stdlib index ready (%d symbols, %s)", len(cache.Symbols), cache.Meta.IndexedAt.Format(time.RFC3339))
		}
	} else {
		idx.startBuild(ctx, statusReasonMissing, false)
	}

	return idx, nil
}

func (idx *Index) Symbols() []*symbol.Symbol {
	if !idx.ready.Load() {
		return nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.symbols
}

func (idx *Index) loadFromCache(cache CacheFile) {
	idx.mu.Lock()
	idx.symbols = make([]*symbol.Symbol, len(cache.Symbols))
	copy(idx.symbols, cache.Symbols)
	idx.mu.Unlock()
	idx.ready.Store(true)
}

func (idx *Index) startBuild(ctx context.Context, reason string, rebuilding bool) {
	if !idx.building.CompareAndSwap(false, true) {
		return
	}
	idx.ready.Store(false)
	action := "building"
	if rebuilding {
		action = "rebuilding"
	}
	log.Printf("index: %s stdlib index at %q (reason=%s)", action, idx.cfg.Path, reason)
	go func() {
		defer idx.building.Store(false)
		startedAt := time.Now()
		cache, err := idx.build(ctx)
		if err != nil {
			log.Printf("index: %s stdlib index failed after %s: %v", action, time.Since(startedAt).Round(time.Millisecond), err)
			return
		}
		if err := saveCache(idx.cfg.Path, cache); err != nil {
			log.Printf("index: %s stdlib index save failed after %s: %v", action, time.Since(startedAt).Round(time.Millisecond), err)
			return
		}
		idx.loadFromCache(cache)
		log.Printf("index: stdlib index ready (%d symbols, %s)", len(cache.Symbols), time.Since(startedAt).Round(time.Millisecond))
	}()
}

func (idx *Index) build(ctx context.Context) (CacheFile, error) {
	fingerprint, err := stdlibFingerprint(ctx)
	if err != nil {
		return CacheFile{}, err
	}
	symbols, err := crawlStdlib(ctx)
	if err != nil {
		return CacheFile{}, fmt.Errorf("go stdlib source: %w", err)
	}
	now := time.Now().UTC()
	return CacheFile{
		Meta: CacheMeta{
			SchemaVersion: schemaVersion,
			Fingerprint:   fingerprint,
			IndexedAt:     now,
			SymbolCount:   len(symbols),
		},
		Symbols: symbols,
	}, nil
}

func stdlibFingerprint(ctx context.Context) (CacheFingerprint, error) {
	env, err := currentGoEnv(ctx)
	if err != nil {
		return CacheFingerprint{}, err
	}
	return CacheFingerprint{
		Language: "go",
		Provider: "go-stdlib",
		Tool:     "go",
		Values: map[string]string{
			"GOVERSION":   env.GoVersion,
			"GOROOT":      env.GOROOT,
			"GOOS":        env.GOOS,
			"GOARCH":      env.GOARCH,
			"CGO_ENABLED": env.CGOEnabled,
		},
	}, nil
}

func currentGoEnv(ctx context.Context) (goEnv, error) {
	cmd := exec.CommandContext(ctx, "go", "env", "-json", "GOVERSION", "GOROOT", "GOOS", "GOARCH", "CGO_ENABLED")
	out, err := cmd.Output()
	if err != nil {
		return goEnv{}, fmt.Errorf("go env: %w", err)
	}
	var env goEnv
	if err := json.Unmarshal(out, &env); err != nil {
		return goEnv{}, err
	}
	return env, nil
}

func saveCache(path string, cache CacheFile) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create cache file: %w", err)
	}
	if err := gob.NewEncoder(f).Encode(cache); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			log.Printf("index: close cache file after encode error: %v", closeErr)
		}
		return fmt.Errorf("encode cache: %w", err)
	}
	return f.Close()
}

func loadCache(path string) (CacheFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return CacheFile{}, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Printf("index: close cache file: %v", closeErr)
		}
	}()
	var cache CacheFile
	if err := gob.NewDecoder(f).Decode(&cache); err != nil {
		return CacheFile{}, fmt.Errorf("decode cache: %w", err)
	}
	return cache, nil
}

func isCacheStale(meta CacheMeta, current CacheFingerprint) (bool, string) {
	if meta.SchemaVersion == "" {
		return true, statusReasonMissing
	}
	if meta.SchemaVersion != schemaVersion {
		return true, statusReasonSchemaChanged
	}
	if meta.Fingerprint.Language == "" || meta.Fingerprint.Provider == "" {
		return true, statusReasonMissing
	}
	if !sameFingerprint(meta.Fingerprint, current) {
		return true, statusReasonGoEnvChanged
	}
	return false, statusReasonExisting
}

func sameFingerprint(a, b CacheFingerprint) bool {
	if a.Language != b.Language || a.Provider != b.Provider || a.Tool != b.Tool {
		return false
	}
	if len(a.Values) != len(b.Values) {
		return false
	}
	for k, av := range a.Values {
		if b.Values[k] != av {
			return false
		}
	}
	return true
}

func validateCache(cache CacheFile) error {
	if cache.Meta.SymbolCount != len(cache.Symbols) {
		return fmt.Errorf("symbol count mismatch: meta=%d actual=%d", cache.Meta.SymbolCount, len(cache.Symbols))
	}
	return nil
}

func hasNonEmptyFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Size() > 0
}

func containsPathComponent(path, component string) bool {
	return path == component ||
		strings.HasPrefix(path, component+"/") ||
		strings.Contains(path, "/"+component+"/") ||
		strings.HasSuffix(path, "/"+component)
}

func init() {
	gob.Register(CacheFingerprint{})
	gob.Register(map[string]string{})
	gob.Register(time.Time{})
}
