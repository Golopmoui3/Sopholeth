package discovery

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"sopholeth/internal/trust/bootstrap"
)

type fakeSource struct {
	refresh func(context.Context) (bootstrap.View, error)
	current func(context.Context) (bootstrap.View, error)
}

func (f fakeSource) Refresh(ctx context.Context) (bootstrap.View, error) { return f.refresh(ctx) }
func (f fakeSource) Current(ctx context.Context) (bootstrap.View, error) { return f.current(ctx) }

func TestRuntimeExpiryAndRetry(t *testing.T) {
	base := time.Now()
	var seconds atomic.Int64
	now := func() time.Time { return base.Add(time.Duration(seconds.Load()) * time.Second) }
	v := bootstrap.View{Expires: base.Add(time.Second), Manifest: bootstrap.Manifest{Enclave: "default", Roots: []bootstrap.Root{{ID: "one", Origin: "https://one.invalid"}}}}
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	s := &Session{view: v, now: now}
	s.source = fakeSource{refresh: func(context.Context) (bootstrap.View, error) {
		close(started)
		<-release
		return bootstrap.View{}, errors.New("offline")
	}, current: func(context.Context) (bootstrap.View, error) { return bootstrap.View{}, bootstrap.ErrNoManifest }}
	go func() { done <- s.Refresh(context.Background()) }()
	<-started
	seconds.Store(1) // Equality is expired, while the refresh is still blocked.
	if _, err := s.Current(); err == nil || s.IsRoot("one", "https://one.invalid", "default") || len(s.Seeds()) != 0 {
		t.Fatal("stalled refresh preserved expired role/seeds")
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("lost refresh error")
	}

	// A failed refresh waits only the backoff, without another hourly delay.
	delays, ticks := make(chan time.Duration, 1), make(chan time.Time)
	s.after = func(d time.Duration) <-chan time.Time { delays <- d; return ticks }
	s.source = fakeSource{refresh: func(context.Context) (bootstrap.View, error) { return bootstrap.View{}, errors.New("offline") }, current: func(context.Context) (bootstrap.View, error) { return bootstrap.View{}, bootstrap.ErrNoManifest }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{})
	go func() { s.Run(ctx, nil); close(finished) }()
	for _, want := range []time.Duration{RetryMin, 2 * RetryMin, 4 * RetryMin} {
		select {
		case got := <-delays:
			if got != want {
				t.Fatalf("retry %s, want %s", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("no retry scheduled")
		}
		if want != 4*RetryMin {
			ticks <- now()
		}
	}
	cancel()
	<-finished

	v.Expires = base.Add(time.Hour)
	s.source = fakeSource{refresh: func(context.Context) (bootstrap.View, error) { return v, nil }}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s.IsRoot("one", "https://one.invalid", "default") || len(s.Seeds()) != 1 {
		t.Fatal("valid renewal did not restore role/seeds")
	}
	if s.IsRoot("other", "https://one.invalid", "default") || s.IsRoot("one", "http://one.invalid", "default") {
		t.Fatal("root recognized without matching signed ID and origin")
	}
}
