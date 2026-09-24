// Package testomega provides a small disposable HTTPS repository for consumer
// integration tests. It uses the operator API; it does not repeat custody tests.
package testomega

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"sopholeth/internal/omega"
	"sopholeth/internal/trust/bootstrap"
)

type Repository struct {
	Server   *httptest.Server
	Bundle   bootstrap.Bundle
	Offline  atomic.Bool
	FailPath atomic.Value // string; fail one download after verified progress
	opts     omega.PublishOptions
}

func New(t *testing.T, roots []bootstrap.Root) *Repository {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("disposable operator fixture requires Linux")
	}
	base := t.TempDir()
	r := &Repository{opts: omega.PublishOptions{Home: filepath.Join(base, "authority"), Directory: filepath.Join(base, "public"), Network: "consumer-test", Disposable: true}}
	r.FailPath.Store("")
	files := http.StripPrefix("/omega/", http.FileServer(http.Dir(r.opts.Directory)))
	r.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if r.Offline.Load() || r.FailPath.Load().(string) == req.URL.Path {
			http.Error(w, "temporarily unavailable", 503)
			return
		}
		files.ServeHTTP(w, req)
	}))
	t.Cleanup(r.Server.Close)
	r.opts.HTTPClient = r.Server.Client()
	_, err := omega.Init(context.Background(), omega.InitOptions{Home: r.opts.Home, Network: r.opts.Network, Repository: r.Server.URL + "/omega", Disposable: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(r.opts.Home, r.opts.Network, "bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	r.Bundle, err = bootstrap.ParseBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	r.Publish(t, roots)
	return r
}

func (r *Repository) Publish(t *testing.T, roots []bootstrap.Root) {
	t.Helper()
	r.opts.Version++
	data, err := json.Marshal(bootstrap.Manifest{Schema: 1, Network: r.opts.Network, Enclave: "default", Roots: roots})
	if err != nil {
		t.Fatal(err)
	}
	r.opts.Manifest = data
	if _, err := omega.Publish(context.Background(), r.opts); err != nil {
		t.Fatal(err)
	}
}
