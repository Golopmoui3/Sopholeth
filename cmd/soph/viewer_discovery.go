package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"sopholeth/internal/discovery"
)

// viewerLease pins one process to the profile selected at startup. A refresh
// or expiry cancels existing streams; reconnect gets a new node-local snapshot.
type viewerLease struct {
	mu     sync.Mutex
	target *url.URL
	ctx    context.Context
	cancel context.CancelFunc
}

func (v *viewerLease) clear() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cancel != nil {
		v.cancel()
	}
	v.target = nil
}

func (v *viewerLease) set(ctx context.Context, n Network) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cancel != nil {
		v.cancel()
	}
	v.target, _ = url.Parse(n.Endpoint)
	if n.Discovery == discoveryHTTPS {
		v.ctx, v.cancel = context.WithDeadline(ctx, time.Unix(n.RootsExpire, 0))
	} else {
		v.ctx, v.cancel = context.WithCancel(ctx)
	}
}

func (v *viewerLease) current() (*url.URL, context.Context, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.target == nil || v.ctx.Err() != nil {
		return nil, nil, errors.New("public discovery unavailable; waiting for verified refresh")
	}
	return v.target, v.ctx, nil
}

func (a *app) refreshViewer(ctx context.Context, lease *viewerLease, selected Network) {
	delay := min(discovery.RefreshInterval, max(time.Second, time.Until(time.Unix(selected.RootsExpire, 0))/2))
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			lease.clear()
			return
		case <-timer.C:
		}
		// Withdraw while checking so newly accepted authority changes cannot
		// leave an old stream authorized during a later stalled download.
		lease.clear()
		next, err := a.publicNetwork(ctx, &selected, true)
		if err != nil {
			fmt.Fprintf(a.stderr, "viewer discovery: %v\n", err)
			delay = discovery.RetryMin
			continue
		}
		selected = next
		lease.set(ctx, next)
		delay = min(discovery.RefreshInterval, max(time.Second, time.Until(time.Unix(next.RootsExpire, 0))/2))
	}
}
