package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sopholeth/internal/client/clienttest"
	"sopholeth/internal/discovery"
	"sopholeth/internal/testomega"
	"sopholeth/internal/trust/bootstrap"
)

func TestPublicJoinAndProfileRefresh(t *testing.T) {
	// One operator fixture exercises the actual publication -> TUF -> CLI path.
	// The second node lets membership updates move a saved profile before PUT.
	first := clienttest.New()
	first.Network = "public"
	second := clienttest.New()
	second.Network, second.NodeID = "public", "second"
	var puts atomic.Int32
	firstServer := httptest.NewTLSServer(first.Handler())
	defer firstServer.Close()
	secondServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts.Add(1)
		}
		second.Handler().ServeHTTP(w, r)
	}))
	defer secondServer.Close()
	bad := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	defer bad.Close()
	roots := []bootstrap.Root{{ID: "bad", Origin: bad.URL}, {ID: first.NodeID, Origin: firstServer.URL}, {ID: "unused", Origin: "https://127.0.0.1:1"}}
	repo := testomega.New(t, roots)
	ta := newTestApp(t)
	ta.app.newHTTPClient = func() *http.Client { return repo.Server.Client() }
	ta.app.publicDiscovery = func(ctx context.Context, refresh bool) (discovery.Identity, bootstrap.View, error) {
		return ta.app.discoverBundle(ctx, repo.Bundle, refresh)
	}
	out, stderr := ta.mustRun(t, "", "join")
	if !strings.Contains(out, "public, https") || !strings.Contains(stderr, "failed health check") {
		t.Fatalf("join: %s / %s", out, stderr)
	}
	cfg := loadOrEmpty(ta.configPath)
	n := cfg.Networks["public"]
	if n.Endpoint != firstServer.URL || n.Authority.Fingerprint != repo.Bundle.Fingerprint() || cfg.Current != "public" {
		t.Fatalf("saved: %+v", cfg)
	}
	ta.mustRun(t, "initial", "put", "k")
	if value, _ := first.Value("k"); string(value) != "initial" {
		t.Fatal("write did not reach selected root")
	}

	// Make the saved scheduling hint old; the verified metadata remains the
	// authority. Removing a root must select a new one without manual rejoin.
	roots[1] = bootstrap.Root{ID: second.NodeID, Origin: secondServer.URL}
	repo.Publish(t, roots)
	cfg = loadOrEmpty(ta.configPath)
	n = cfg.Networks["public"]
	n.DiscoveryChecked = time.Time{}
	cfg.Networks["public"] = n
	if err := saveConfig(ta.configPath, cfg); err != nil {
		t.Fatal(err)
	}
	ta.mustRun(t, "updated", "put", "k")
	if puts.Load() != 1 {
		t.Fatalf("PUT submitted %d times", puts.Load())
	}
	if value, _ := second.Value("k"); string(value) != "updated" {
		t.Fatal("profile did not move to replacement root")
	}

	// A repository outage may use a valid durable view, including in a new
	// process. Saved endpoints/expiry are informational, not trusted caches.
	repo.Offline.Store(true)
	cfg = loadOrEmpty(ta.configPath)
	n = cfg.Networks["public"]
	n.DiscoveryChecked = time.Time{}
	n.Endpoint, n.RootsExpire = "http://untrusted.invalid", 1
	cfg.Networks["public"] = n
	if err := saveConfig(ta.configPath, cfg); err != nil {
		t.Fatal(err)
	}
	out, _ = ta.mustRun(t, "", "get", "k")
	if out != "updated" {
		t.Fatal(out)
	}

	// Never adopt a different authority for an existing profile, and never
	// downgrade after corrupt durable state, even with healthy node URLs.
	cfg = loadOrEmpty(ta.configPath)
	n = cfg.Networks["public"]
	n.Authority.Network = "another-network"
	cfg.Networks["public"] = n
	if err := saveConfig(ta.configPath, cfg); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(ta.configPath)
	code, _, stderr := ta.run("rejected", "put", "k")
	after, _ := os.ReadFile(ta.configPath)
	if code == exitOK || !strings.Contains(stderr, "refusing to switch") || !bytes.Equal(before, after) || puts.Load() != 1 {
		t.Fatalf("authority switch: %d %s", code, stderr)
	}
	// A legacy DNS profile cannot turn into a newly trusted network implicitly.
	n.Discovery = discoverySignedList
	cfg.Networks["public"] = n
	if err := saveConfig(ta.configPath, cfg); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = ta.run("", "get", "k")
	if code != exitUsage || !strings.Contains(stderr, "legacy public profile") {
		t.Fatalf("legacy profile: %d %s", code, stderr)
	}
	n.Discovery = discoveryHTTPS
	n.Authority = discovery.BundleIdentity(repo.Bundle)
	cfg.Networks["public"] = n
	if err := saveConfig(ta.configPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(ta.configPath+".trust", ta.configPath+".saved-trust"); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = ta.run("", "get", "k")
	if code == exitOK || !strings.Contains(stderr, "trust state is missing") {
		t.Fatalf("missing state: %d %s", code, stderr)
	}

}

func TestPublicJoinWithoutConfiguredAuthority(t *testing.T) {
	if _, err := discovery.PublicBundle(); !errors.Is(err, discovery.ErrUnconfigured) {
		t.Skip("configured build")
	}
	ta := newTestApp(t)
	ta.app.publicDiscovery = nil
	code, _, stderr := ta.run("", "join")
	if code != exitError || !strings.Contains(stderr, "trust bundle is not configured") || !strings.Contains(stderr, "soph join <node>") {
		t.Fatalf("%d %s", code, stderr)
	}
	if _, err := os.Stat(ta.configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created profile: %v", err)
	}
}
