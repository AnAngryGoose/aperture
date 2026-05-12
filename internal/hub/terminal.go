package hub

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/aperture/aperture/internal/dockerctl"
)

// localTerminalProvider implements TerminalProvider using a directly-connected
// Docker client. Used when the hub manages containers on its own host.
type localTerminalProvider struct {
	docker  *dockerctl.Client
	mu      sync.Mutex
	sessions map[string]*localTermSession
	counter  atomic.Int64
}

type localTermSession struct {
	stdin   io.WriteCloser
	resize  func(cols, rows uint) error
	closeFn func()
}

// NewLocalTerminalProvider returns a TerminalProvider backed by a direct Docker
// socket connection. Use this for the hub's own host.
func NewLocalTerminalProvider(dc *dockerctl.Client) TerminalProvider {
	return &localTerminalProvider{
		docker:   dc,
		sessions: make(map[string]*localTermSession),
	}
}

func (p *localTerminalProvider) StartTerminal(ctx context.Context, cid, cmd string) (string, <-chan []byte, error) {
	stdin, outCh, resizeFn, closeFn, err := p.docker.StartTerminal(ctx, cid, cmd)
	if err != nil {
		return "", nil, err
	}
	reqID := fmt.Sprintf("local_%d", p.counter.Add(1))
	p.mu.Lock()
	p.sessions[reqID] = &localTermSession{stdin: stdin, resize: resizeFn, closeFn: closeFn}
	p.mu.Unlock()
	return reqID, outCh, nil
}

func (p *localTerminalProvider) SendTerminalData(_ context.Context, reqID string, data []byte) error {
	p.mu.Lock()
	sess, ok := p.sessions[reqID]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("terminal session not found: %s", reqID)
	}
	_, err := sess.stdin.Write(data)
	return err
}

func (p *localTerminalProvider) ResizeTerminal(_ context.Context, reqID string, cols, rows uint) error {
	p.mu.Lock()
	sess, ok := p.sessions[reqID]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("terminal session not found: %s", reqID)
	}
	return sess.resize(cols, rows)
}

func (p *localTerminalProvider) CloseTerminal(_ context.Context, reqID string) error {
	p.mu.Lock()
	sess, ok := p.sessions[reqID]
	if ok {
		delete(p.sessions, reqID)
	}
	p.mu.Unlock()
	if !ok {
		return nil
	}
	sess.closeFn()
	return nil
}

// compile-time check
var _ TerminalProvider = (*localTerminalProvider)(nil)
