// Package clientip resolves the real client address of an HTTP request.
//
// X-Forwarded-For is client-controlled: anything a browser sends is copied
// into the header by every proxy on the way. It is only meaningful when the
// direct TCP peer is a proxy we trust, and even then only the hops appended by
// trusted proxies can be believed. Resolver therefore:
//
//   - ignores X-Forwarded-For entirely when the direct peer is not trusted;
//   - otherwise walks the chain right-to-left, skipping trusted proxies, and
//     returns the first untrusted hop (the address the outermost trusted proxy
//     saw);
//   - falls back to the leftmost hop when every hop is trusted, and to the
//     peer address when the header is absent or unusable.
//
// The resolved address is computed once per request by Middleware and read
// with From, so handlers never parse forwarding headers themselves.
package clientip

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// DefaultTrusted is used when no trusted proxies are configured: loopback and
// private ranges, which covers a BFF and edge proxy on the same host or
// container network.
var DefaultTrusted = []string{
	"127.0.0.0/8",
	"::1/128",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7",
}

// Resolver decides which forwarding hops to believe.
type Resolver struct {
	trusted []netip.Prefix
}

// NewResolver builds a Resolver from CIDRs or bare IPs. An empty list means
// DefaultTrusted; the single entry "none" trusts no proxy at all (forwarding
// headers are then always ignored).
func NewResolver(entries []string) (*Resolver, error) {
	if len(entries) == 0 {
		entries = DefaultTrusted
	}
	if len(entries) == 1 && strings.EqualFold(strings.TrimSpace(entries[0]), "none") {
		return &Resolver{}, nil
	}
	r := &Resolver{trusted: make([]netip.Prefix, 0, len(entries))}
	for _, raw := range entries {
		e := strings.TrimSpace(raw)
		if e == "" {
			continue
		}
		p, err := parsePrefix(e)
		if err != nil {
			return nil, fmt.Errorf("clientip: invalid trusted proxy %q: %w", e, err)
		}
		r.trusted = append(r.trusted, p)
	}
	return r, nil
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		return p.Masked(), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}

func (r *Resolver) isTrusted(a netip.Addr) bool {
	a = a.Unmap().WithZone("")
	for _, p := range r.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Resolve returns the client address for req as a string.
func (r *Resolver) Resolve(req *http.Request) string {
	peerStr := peerHost(req.RemoteAddr)
	peer, err := netip.ParseAddr(peerStr)
	if err != nil || r == nil || !r.isTrusted(peer) {
		return peerStr
	}

	var hops []netip.Addr
	for _, v := range req.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(v, ",") {
			if a, ok := parseHop(part); ok {
				hops = append(hops, a)
			}
		}
	}
	if len(hops) == 0 {
		return peerStr
	}
	for i := len(hops) - 1; i >= 0; i-- {
		if !r.isTrusted(hops[i]) {
			return hops[i].String()
		}
	}
	// Every hop is a trusted (internal) address: the leftmost is the origin.
	return hops[0].String()
}

// parseHop parses one X-Forwarded-For entry, tolerating an optional port and
// IPv6 brackets.
func parseHop(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, false
	}
	// Zones (fe80::1%eth0) are dropped so they never reach session/audit rows.
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Unmap().WithZone(""), true
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap().WithZone(""), true
	}
	if a, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
		return a.Unmap().WithZone(""), true
	}
	return netip.Addr{}, false
}

func peerHost(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

type ctxKey struct{}

// Middleware resolves the client address once and stores it on the request
// context for From.
func Middleware(r *Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip := r.Resolve(req)
			next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), ctxKey{}, ip)))
		})
	}
}

// From returns the client address resolved by Middleware. Without the
// middleware it falls back to the direct peer and never trusts headers.
func From(req *http.Request) string {
	if req == nil {
		return ""
	}
	if ip, ok := req.Context().Value(ctxKey{}).(string); ok && ip != "" {
		return ip
	}
	return peerHost(req.RemoteAddr)
}
