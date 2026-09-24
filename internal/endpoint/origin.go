// Package endpoint handles node HTTP origins shared by clients and gossip.
package endpoint

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// Normalize accepts an HTTP(S) origin or a bare address. Only bare addresses
// default to the node's 8080 port; explicit URLs retain HTTP/HTTPS defaults.
func Normalize(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("endpoint must not be empty")
	}
	bare := !strings.Contains(raw, "://")
	if bare {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid endpoint: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Opaque != "" ||
		u.User != nil || (u.Path != "" && u.Path != "/") || u.RawPath != "" || strings.ContainsAny(raw, "?#\\%") {
		return "", errors.New("invalid endpoint: expected an HTTP(S) origin without credentials, path, query, or fragment")
	}
	host := strings.ToLower(u.Hostname())
	if ip, err := netip.ParseAddr(host); err == nil {
		host = ip.String()
	}
	port := u.Port()
	if strings.HasSuffix(u.Host, ":") {
		return "", errors.New("invalid endpoint: empty port")
	}
	if port == "" && bare {
		port = "8080"
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("invalid endpoint: bad port")
		}
		port = strconv.Itoa(n)
		if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
			port = ""
		}
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return u.Scheme + "://" + host, nil
}

// Client preserves certificate and hostname validation and refuses redirects.
// Public callers can supply additional CA roots, never a verification bypass.
func Client(hc *http.Client) (*http.Client, error) {
	c := &http.Client{}
	if hc != nil {
		*c = *hc
	}
	transport := c.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	t, ok := transport.(*http.Transport)
	if !ok || t.DialTLS != nil || t.DialTLSContext != nil ||
		(t.TLSClientConfig != nil && (t.TLSClientConfig.InsecureSkipVerify || t.TLSClientConfig.ServerName != "")) {
		return nil, errors.New("node transport requires certificate and hostname verification")
	}
	c.Transport = t
	c.Jar = nil
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c, nil
}
