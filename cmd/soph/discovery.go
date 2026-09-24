package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"time"

	"sopholeth/internal/client"
	"sopholeth/internal/discovery"
	"sopholeth/internal/endpoint"
	"sopholeth/internal/trust/bootstrap"
)

func (a *app) discover(ctx context.Context, refresh bool) (discovery.Identity, bootstrap.View, error) {
	if a.publicDiscovery != nil {
		return a.publicDiscovery(ctx, refresh)
	}
	b, err := discovery.PublicBundle()
	if err != nil {
		return discovery.Identity{}, bootstrap.View{}, fmt.Errorf("public discovery unavailable (use 'soph join <node>' for an explicit connection): %w", err)
	}
	return a.discoverBundle(ctx, b, refresh)
}

func (a *app) discoverBundle(ctx context.Context, b bootstrap.Bundle, refresh bool) (discovery.Identity, bootstrap.View, error) {
	ctx, cancel := a.requestContext(ctx)
	defer cancel()
	id := discovery.BundleIdentity(b)
	// Missing rollback history is not an invitation to trust the initial root
	// again for a saved public connection. Explicit reset remains separate work.
	cfg, err := a.loadConfig()
	if err != nil {
		return id, bootstrap.View{}, err
	}
	if _, err := os.Lstat(a.configPath + ".trust"); os.IsNotExist(err) {
		for _, n := range cfg.Networks {
			if n.Discovery == discoveryHTTPS {
				return id, bootstrap.View{}, fmt.Errorf("public trust state is missing; restore %s.trust from backup, or deliberately adopt a reset network", a.configPath)
			}
		}
	}
	s, err := discovery.Open(ctx, b, a.configPath+".trust", a.newHTTPClient())
	if err != nil {
		return id, bootstrap.View{}, err
	}
	if !refresh {
		if v, err := s.Current(); err == nil {
			return id, v, nil
		}
	}
	refreshErr := s.Refresh(ctx)
	if ctx.Err() == context.Canceled {
		return id, bootstrap.View{}, ctx.Err()
	}
	v, err := s.Current()
	if err != nil {
		return id, bootstrap.View{}, fmt.Errorf("public discovery unavailable: %w (refresh: %v); retry when valid metadata is reachable", err, refreshErr)
	}
	if refreshErr != nil {
		fmt.Fprintf(a.stderr, "discovery refresh failed; using valid cached metadata until %s: %v\n", v.Expires.UTC().Format(time.RFC3339), refreshErr)
	}
	return id, v, nil
}

// publicNetwork selects an authenticated root before a command is sent. It
// never submits/retries a data operation, registers a client, or starts a node.
func (a *app) publicNetwork(ctx context.Context, previous *Network, force bool) (Network, error) {
	refresh := force || previous == nil || time.Since(previous.DiscoveryChecked) >= discovery.RefreshInterval
	id, view, err := a.discover(ctx, refresh)
	if err != nil {
		return Network{}, err
	}
	if previous != nil && (previous.Authority != id || previous.Enclave != view.Manifest.Enclave) {
		return Network{}, fmt.Errorf("saved public profile belongs to a different authority or enclave; refusing to switch networks")
	}
	hc, err := endpoint.Client(a.newHTTPClient())
	if err != nil {
		return Network{}, err
	}
	roots := slices.Clone(view.Manifest.Roots)
	if previous != nil {
		// Prefer the selected endpoint while it is still an official root.
		for i, root := range roots {
			if root.Origin == previous.Endpoint {
				roots[0], roots[i] = roots[i], roots[0]
				break
			}
		}
	}
	var lastErr error
	for _, root := range roots {
		if ctx.Err() != nil {
			return Network{}, ctx.Err()
		}
		if !time.Now().Before(view.Expires) {
			return Network{}, fmt.Errorf("public discovery expired before connection; retry discovery")
		}
		rctx, cancel := a.requestContext(ctx)
		health, err := client.New(root.Origin, hc).Health(rctx)
		cancel()
		if err == nil && (health.Status != "healthy" || health.NodeID != root.ID || health.Network != modePublic || health.Enclave != view.Manifest.Enclave) {
			err = fmt.Errorf("root health disagrees with signed node ID, public mode, or enclave")
		}
		if err != nil {
			lastErr = err
			fmt.Fprintf(a.stderr, "root %s failed health check: %v\n", root.Origin, err)
			continue
		}
		if !time.Now().Before(view.Expires) {
			return Network{}, fmt.Errorf("public discovery expired during connection; retry discovery")
		}
		n := Network{Endpoint: root.Origin, Mode: modePublic, Discovery: discoveryHTTPS, Authority: id,
			RootsExpire: view.Expires.Unix(), NodeID: health.NodeID, NodeNetwork: health.Network, Enclave: health.Enclave, JoinedAt: time.Now().UTC()}
		for _, r := range view.Manifest.Roots {
			n.Roots = append(n.Roots, r.Origin)
		}
		if previous != nil {
			n.JoinedAt, n.DiscoveryChecked = previous.JoinedAt, previous.DiscoveryChecked
		}
		if refresh || n.DiscoveryChecked.IsZero() {
			n.DiscoveryChecked = time.Now().UTC()
		}
		return n, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no authenticated roots")
	}
	return Network{}, fmt.Errorf("no public root passed health checks: %w", lastErr)
}

// verifiedHTTPClient is used for a saved public data connection as well as its
// health probe. The standard client also refuses redirects for private calls.
func (a *app) verifiedHTTPClient() (*http.Client, error) { return endpoint.Client(a.newHTTPClient()) }
