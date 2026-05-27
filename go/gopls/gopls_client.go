package gopls

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
	"go.lsp.dev/jsonrpc2"
)

type goplsConn struct {
	conn jsonrpc2.Conn
}

func newGoplsConn(rwc io.ReadWriteCloser) *goplsConn {
	stream := jsonrpc2.NewStream(rwc)
	conn := jsonrpc2.NewConn(stream)
	conn.Go(context.Background(), jsonrpc2.MethodNotFoundHandler)
	return &goplsConn{conn: conn}
}

func (c *goplsConn) Call(ctx context.Context, method string, params, result any) error {
	_, err := c.conn.Call(ctx, method, params, result)
	return err
}

func (c *goplsConn) Notify(ctx context.Context, method string, params any) error {
	return c.conn.Notify(ctx, method, params)
}

func (c *goplsConn) Close() error {
	return c.conn.Close()
}

const (
	clientInitTimeout     = 30 * time.Second
	clientShutdownTimeout = 5 * time.Second
	clientKillTimeout     = 5 * time.Second
)

type FileLocation struct {
	URI   string       `json:"uri"`
	Range symbol.Range `json:"range"`
}

type LspSymbol struct {
	Name          string       `json:"name"`
	ContainerName string       `json:"containerName"`
	Kind          symbol.Kind  `json:"kind"`
	Location      FileLocation `json:"location"`
}

type GoplsClient struct {
	cmd  *exec.Cmd
	conn *goplsConn
}

func NewGoplsClient(ctx context.Context, cwd string) (*GoplsClient, error) {
	ctx, cancel := context.WithTimeout(ctx, clientInitTimeout)
	defer cancel()

	cmd := exec.Command("gopls", "-remote=auto")
	cmd.Dir = cwd

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("gopls stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("gopls stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start gopls: %w", err)
	}

	conn := newGoplsConn(&stdioRWC{stdin: stdin, stdout: stdout})

	var initResult any
	if err := conn.Call(ctx, "initialize", map[string]any{
		"processId": nil,
		"rootUri":   "file://" + cwd,
		"capabilities": map[string]any{
			"workspace": map[string]any{"symbol": map[string]any{}},
		},
		"workspaceFolders": []map[string]any{
			{"uri": "file://" + cwd, "name": "workspace"},
		},
	}, &initResult); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("gopls close after init failure: %v", closeErr)
		}
		if waitErr := cmd.Wait(); waitErr != nil {
			log.Printf("gopls wait after init failure: %v", waitErr)
		}
		return nil, fmt.Errorf("gopls initialize: %w", err)
	}
	if err := conn.Notify(ctx, "initialized", map[string]any{}); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("gopls close after initialized failure: %v", closeErr)
		}
		if waitErr := cmd.Wait(); waitErr != nil {
			log.Printf("gopls wait after initialized failure: %v", waitErr)
		}
		return nil, fmt.Errorf("gopls initialized: %w", err)
	}

	return &GoplsClient{cmd: cmd, conn: conn}, nil
}

func (c *GoplsClient) WorkspaceSymbol(ctx context.Context, query string) ([]*LspSymbol, error) {
	var rawSymbols []*LspSymbol
	if err := c.conn.Call(ctx, "workspace/symbol", map[string]any{"query": query}, &rawSymbols); err != nil {
		return nil, fmt.Errorf("workspace/symbol: %w", err)
	}
	return rawSymbols, nil
}

func (c *GoplsClient) Close() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), clientShutdownTimeout)
	defer cancel()
	if err := c.conn.Call(shutdownCtx, "shutdown", nil, nil); err != nil {
		log.Printf("gopls shutdown error: %v", err)
	}
	if err := c.conn.Notify(context.Background(), "exit", nil); err != nil {
		log.Printf("gopls exit notify error: %v", err)
	}
	if err := c.conn.Close(); err != nil {
		log.Printf("gopls connection close error: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case <-time.After(clientKillTimeout):
		if err := c.cmd.Process.Kill(); err != nil {
			log.Printf("gopls kill error: %v", err)
		}
		<-done
		return fmt.Errorf("gopls did not exit cleanly after %s, killed", clientKillTimeout)
	case err := <-done:
		return err
	}
}

type stdioRWC struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (s *stdioRWC) Read(p []byte) (int, error)  { return s.stdout.Read(p) }
func (s *stdioRWC) Write(p []byte) (int, error) { return s.stdin.Write(p) }
func (s *stdioRWC) Close() error {
	err := s.stdin.Close()
	if err != nil {
		log.Printf("failed to close stdin: %v", err)
	}
	return s.stdout.Close()
}

type GoplsManager struct {
	ctx    context.Context
	mu     sync.RWMutex
	cwd    string
	client *GoplsClient
}

func NewGoplsManager(ctx context.Context, cwd string) (*GoplsManager, error) {
	client, err := NewGoplsClient(ctx, cwd)
	if err != nil {
		return nil, err
	}
	return &GoplsManager{ctx: ctx, cwd: cwd, client: client}, nil
}

func (m *GoplsManager) WorkspaceSymbol(ctx context.Context, cwd string, query string) ([]*LspSymbol, error) {
	client, err := m.clientForWorkspace(cwd)
	if err != nil {
		return nil, err
	}
	return client.WorkspaceSymbol(ctx, query)
}

func (m *GoplsManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client == nil {
		return nil
	}
	err := m.client.Close()
	m.client = nil
	return err
}

func (m *GoplsManager) clientForWorkspace(cwd string) (*GoplsClient, error) {
	m.mu.RLock()
	if m.client != nil && m.cwd == cwd {
		client := m.client
		m.mu.RUnlock()
		return client, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client != nil && m.cwd == cwd {
		return m.client, nil
	}

	client, err := NewGoplsClient(m.ctx, cwd)
	if err != nil {
		log.Printf("failed to create gopls client for %s: %v", cwd, err)
		return nil, err
	}
	old := m.client
	oldCWD := m.cwd
	m.client = client
	m.cwd = cwd
	if old != nil {
		err = old.Close()
		log.Printf("closed old gopls client for %s: %v", oldCWD, err)
	}
	return client, nil
}
