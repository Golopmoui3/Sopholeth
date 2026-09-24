package discovery

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"sopholeth/internal/trust/bootstrap"
)

const RefreshInterval = time.Hour
const RetryMin = 30 * time.Second
const RetryMax = 10 * time.Minute

type source interface {
	Current(context.Context) (bootstrap.View, error)
	Refresh(context.Context) (bootstrap.View, error)
}

// Session holds a verified view for runtime consumers. Current never waits on
// a fetch or disk lock, and always checks expiry, even during a stalled refresh.
// A durable root/targets transition immediately invalidates the retained view.
type Session struct {
	source  source
	mu      sync.RWMutex
	view    bootstrap.View
	lastErr error
	now     func() time.Time
	after   func(time.Duration) <-chan time.Time
}

func Open(ctx context.Context, b bootstrap.Bundle, stateDir string, hc *http.Client) (*Session, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(stateDir), 0700); err != nil {
		return nil, err
	}
	s := &Session{now: time.Now, after: time.After}
	c, err := bootstrap.New(ctx, bootstrap.Config{Bundle: b, StateDir: stateDir, HTTPClient: hc,
		OnInvalidate: func() { s.set(bootstrap.View{}) },
	})
	if err != nil {
		return nil, err
	}
	s.source = c
	view, _ := c.Current(ctx) // An absent/expired view can be recovered by Refresh.
	s.set(view)
	return s, nil
}

func (s *Session) set(v bootstrap.View) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.view = v
}

func (s *Session) Current() (bootstrap.View, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.view.Manifest.Roots) == 0 || !s.now().Before(s.view.Expires) {
		return bootstrap.View{}, bootstrap.ErrNoManifest
	}
	v := s.view
	v.Manifest.Roots = slices.Clone(v.Manifest.Roots)
	return v, nil
}

// Refresh retains only a still-valid durable view on failure, never an unchecked
// in-memory list. Its error reports the failed fetch even if Current is usable.
func (s *Session) Refresh(ctx context.Context) error {
	v, err := s.source.Refresh(ctx)
	if err != nil {
		// Refresh may have used up its request deadline. Re-read committed
		// state with a separate, short budget, without repeating any download.
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		var currentErr error
		v, currentErr = s.source.Current(readCtx)
		cancel()
		if currentErr != nil {
			v = bootstrap.View{}
			err = errors.Join(err, currentErr)
		}
	}
	s.mu.Lock()
	s.view, s.lastErr = v, err
	s.mu.Unlock()
	return err
}

// Run refreshes hourly (so daily publications are picked up promptly), sooner
// near expiry. Failures wait only the retry backoff, not another normal period.
// Expiry is enforced independently by Current, not by this timer.
func (s *Session) Run(ctx context.Context, onResult func(error)) {
	backoff := RetryMin
	for {
		s.mu.RLock()
		failed := s.lastErr != nil
		s.mu.RUnlock()
		delay := RefreshInterval
		if failed {
			delay = backoff
			backoff = min(backoff*2, RetryMax)
		} else {
			backoff = RetryMin
			if v, err := s.Current(); err == nil {
				delay = min(delay, max(RetryMin, v.Expires.Sub(s.now())/2))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-s.after(delay):
		}
		err := s.Refresh(ctx)
		if onResult != nil {
			onResult(err)
		}
	}
}

func (s *Session) Seeds() []string {
	v, err := s.Current()
	if err != nil {
		return nil
	}
	seeds := make([]string, 0, len(v.Manifest.Roots))
	for _, root := range v.Manifest.Roots {
		seeds = append(seeds, root.Origin)
	}
	return seeds
}

func (s *Session) IsRoot(id, origin, enclave string) bool {
	v, err := s.Current()
	if err != nil || v.Manifest.Enclave != enclave {
		return false
	}
	for _, root := range v.Manifest.Roots {
		if root.ID == id && root.Origin == origin {
			return true
		}
	}
	return false
}
