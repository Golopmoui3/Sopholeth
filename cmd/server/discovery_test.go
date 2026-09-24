package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sopholeth/internal/cluster"
	"sopholeth/internal/discovery"
	"sopholeth/internal/gossip"
	"sopholeth/internal/testomega"
	"sopholeth/internal/trust/bootstrap"
)

// This replaces the legacy DNS integration test with the actual HTTPS/TUF
// consumer path. One root and one ordinary node suffice to check the wiring;
// three-host operation belongs to the deployment test.
func TestHTTPSDiscoveryNodeJoining(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	newNode := func(id string) (*cluster.ClusterNode, *httptest.Server) {
		cn := cluster.NewClusterNode(id, "127.0.0.1", 9090, 8080, 3, 0, 2*time.Second, "", "default")
		h := &HTTPServer{clusterNode: cn, nodeID: id, network: "public"}
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/bootstrap", h.bootstrapHandler)
		mux.HandleFunc("/v1/gossip/message", h.gossipHandler)
		srv := httptest.NewTLSServer(mux)
		t.Cleanup(srv.Close)
		cn.SetHTTPOrigin(srv.URL) // Advertised TLS port differs from both local fields.
		return cn, srv
	}
	root, rootHTTP := newNode("root-one")
	leaf, leafHTTP := newNode("ordinary-node")
	repo := testomega.New(t, []bootstrap.Root{{ID: "root-one", Origin: rootHTTP.URL}, {ID: "two", Origin: "https://127.0.0.1:1"}, {ID: "three", Origin: "https://127.0.0.1:2"}})
	open := func(cn *cluster.ClusterNode, id, origin string) *discovery.Session {
		s, err := discovery.Open(ctx, repo.Bundle, filepath.Join(t.TempDir(), "trust"), repo.Server.Client())
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Refresh(ctx); err != nil {
			t.Fatal(err)
		}
		if err := configurePublicDiscovery(cn, s, id, origin, "default"); err != nil {
			t.Fatal(err)
		}
		cn.ConfigureBootstrap(repo.Server.Client(), publicSeedCheck(s))
		return s
	}
	rootDiscovery := open(root, "root-one", rootHTTP.URL)
	leafDiscovery := open(leaf, "ordinary-node", leafHTTP.URL)
	if !root.IsRoot() || leaf.IsRoot() {
		t.Fatal("official root role did not match signed ID and origin")
	}
	if err := root.Start(ctx, nil); err != nil {
		t.Fatal(err)
	}
	defer root.Stop()
	if err := leaf.Start(ctx, leafDiscovery.Seeds()); err != nil {
		t.Fatal(err)
	}
	defer leaf.Stop()
	if len(leaf.Topology()) != 1 || len(root.Topology()) != 1 {
		t.Fatalf("join failed: %v / %v", leaf.Topology(), root.Topology())
	}
	if leaf.Topology()[0].HTTPOrigin != rootHTTP.URL || root.Topology()[0].HTTPOrigin != leafHTTP.URL {
		t.Fatal("lost advertised HTTPS origins")
	}
	if err := root.Put(ctx, "test", []byte("anonymous"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if data, ok := leaf.Get("test"); !ok || string(data) != "anonymous" {
		t.Fatalf("HTTPS gossip did not replicate: %q", data)
	}
	resp, err := leafHTTP.Client().Post(leafHTTP.URL+"/v1/bootstrap", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("ordinary node claimed root role: %d", resp.StatusCode)
	}
	check := publicSeedCheck(rootDiscovery)
	if err := check(rootHTTP.URL, []*gossip.Node{{ID: "root-one", HTTPOrigin: "http://substitute.invalid", Enclave: "default"}}); err == nil {
		t.Fatal("accepted substituted/downgraded root advertisement")
	}
	if err := check(rootHTTP.URL, []*gossip.Node{{ID: "wrong-root", HTTPOrigin: rootHTTP.URL, Enclave: "default"}}); err == nil {
		t.Fatal("accepted mismatched root ID")
	}
}
