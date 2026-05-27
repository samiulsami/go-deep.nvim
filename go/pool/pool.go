package pool

import "context"

type Pool struct {
	ctx  context.Context
	jobs chan func()
}

func New(ctx context.Context, workers int) *Pool {
	if workers < 1 {
		workers = 1
	}
	p := &Pool{ctx: ctx, jobs: make(chan func(), min(50, workers*2))}
	for range workers {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case fn := <-p.jobs:
					fn()
				}
			}
		}()
	}
	return p
}

func (p *Pool) Submit(fn func()) {
	select {
	case <-p.ctx.Done():
		return
	case p.jobs <- fn:
		return
	}
}
