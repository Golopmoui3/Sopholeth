// Package discovery connects the durable omega trust client to applications.
package discovery

import (
	"bytes"
	_ "embed"
	"errors"

	"sopholeth/internal/trust/bootstrap"
)

// Replace only with the independently verified PUBLIC bundle when adopting an
// authority for a release. Empty development builds cannot discover a network.
//
//go:embed bundle.json
var bundled []byte

var ErrUnconfigured = errors.New("omega trust bundle is not configured; use a build for the intended public network")

func PublicBundle() (bootstrap.Bundle, error) {
	if bytes.Equal(bytes.TrimSpace(bundled), []byte("{}")) {
		return bootstrap.Bundle{}, ErrUnconfigured
	}
	return bootstrap.ParseBundle(bundled)
}

// Identity binds a saved client profile to its initial authority, including
// repository and network name. A release must never silently switch these.
type Identity struct {
	Network     string `json:"network"`
	Repository  string `json:"repository"`
	Fingerprint string `json:"fingerprint"`
}

func BundleIdentity(b bootstrap.Bundle) Identity {
	repository, _ := bootstrap.ValidateLocation(b.Network, b.Repository)
	return Identity{Network: b.Network, Repository: repository, Fingerprint: b.Fingerprint()}
}
