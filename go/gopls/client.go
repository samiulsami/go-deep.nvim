package gopls

import (
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"go.lsp.dev/jsonrpc2"

	"github.com/samiulsami/go-deep.nvim/go/symbol"
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

const (
	clientInitTimeout = 30 * time.Second
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

type Client struct {
	cmd  *exec.Cmd
	conn *goplsConn
}

func NewClient(ctx context.Context, cwd string) (*Client, error) {
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
	if _, err := conn.conn.Call(ctx, "initialize", map[string]any{
		"processId": nil,
		"rootUri":   "file://" + cwd,
		"capabilities": map[string]any{
			"workspace": map[string]any{"symbol": map[string]any{}},
		},
		"workspaceFolders": []map[string]any{
			{"uri": "file://" + cwd, "name": "workspace"},
		},
	}, &initResult); err != nil {
		if closeErr := conn.conn.Close(); closeErr != nil {
			log.Printf("gopls close after init failure: %v", closeErr)
		}
		if waitErr := cmd.Wait(); waitErr != nil {
			log.Printf("gopls wait after init failure: %v", waitErr)
		}
		return nil, fmt.Errorf("gopls initialize: %w", err)
	}
	if err := conn.conn.Notify(ctx, "initialized", map[string]any{}); err != nil {
		if closeErr := conn.conn.Close(); closeErr != nil {
			log.Printf("gopls close after initialized failure: %v", closeErr)
		}
		if waitErr := cmd.Wait(); waitErr != nil {
			log.Printf("gopls wait after initialized failure: %v", waitErr)
		}
		return nil, fmt.Errorf("gopls initialized: %w", err)
	}

	return &Client{cmd: cmd, conn: conn}, nil
}

func (c *Client) WorkspaceSymbol(ctx context.Context, query string) ([]*LspSymbol, error) {
	var rawSymbols []*LspSymbol
	if _, err := c.conn.conn.Call(ctx, "workspace/symbol", map[string]any{"query": query}, &rawSymbols); err != nil {
		return nil, fmt.Errorf("workspace/symbol: %w", err)
	}
	return rawSymbols, nil
}

func (c *Client) kill() {
	if err := c.conn.conn.Close(); err != nil {
		log.Printf("failed to close gopls connection: %v", err)
	}
	if err := c.cmd.Process.Kill(); err != nil {
		log.Printf("failed to kill gopls process: %v", err)
	}
	if err := c.cmd.Wait(); err != nil {
		log.Printf("failed to wait for gopls process: %v", err)
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

type Manager struct {
	ctx    context.Context
	mu     sync.RWMutex
	cwd    string
	client *Client
}

func NewManager(ctx context.Context, cwd string) (*Manager, error) {
	client, err := NewClient(ctx, cwd)
	if err != nil {
		return nil, err
	}
	return &Manager{ctx: ctx, cwd: cwd, client: client}, nil
}

func (m *Manager) WorkspaceSymbol(ctx context.Context, cwd string, query string) ([]*LspSymbol, error) {
	client, err := m.clientForWorkspace(cwd)
	if err != nil {
		return nil, err
	}
	return client.WorkspaceSymbol(ctx, query)
}

func (m *Manager) Kill() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client == nil {
		return
	}
	m.client.kill()
	m.client = nil
}

func (m *Manager) clientForWorkspace(cwd string) (*Client, error) {
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

	client, err := NewClient(m.ctx, cwd)
	if err != nil {
		log.Printf("failed to create gopls client for %s: %v", cwd, err)
		return nil, err
	}
	old := m.client
	oldCWD := m.cwd
	m.client = client
	m.cwd = cwd
	if old != nil {
		old.kill()
		log.Printf("closed old gopls client for %s", oldCWD)
	}
	return client, nil
}
