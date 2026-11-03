package clientip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func req(remote string, xff ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	for _, v := range xff {
		r.Header.Add("X-Forwarded-For", v)
	}
	return r
}

func TestResolve(t *testing.T) {
	def, err := NewResolver(nil)
	if err != nil {
		t.Fatalf("default resolver: %v", err)
	}
	cases := []struct {
		name string
		r    *http.Request
		want string
	}{
		{"untrusted peer ignores spoofed XFF", req("203.0.113.9:4000", "1.2.3.4"), "203.0.113.9"},
		{"no XFF uses peer", req("10.0.0.5:4000"), "10.0.0.5"},
		{"trusted peer, single hop", req("10.0.0.5:4000", "198.51.100.7"), "198.51.100.7"},
		{"rightmost untrusted hop wins over spoofed leftmost",
			req("172.18.0.3:4000", "6.6.6.6, 198.51.100.7"), "198.51.100.7"},
		{"skips trusted proxy hops from the right",
			req("127.0.0.1:4000", "198.51.100.7, 10.0.0.2, 172.18.0.4"), "198.51.100.7"},
		{"multiple XFF headers are one chain",
			req("10.0.0.5:4000", "6.6.6.6", "198.51.100.7"), "198.51.100.7"},
		{"all hops trusted returns leftmost", req("10.0.0.5:4000", "192.168.1.20, 10.0.0.2"), "192.168.1.20"},
		{"garbage hops skipped", req("10.0.0.5:4000", "not-an-ip, , 198.51.100.7"), "198.51.100.7"},
		{"hop with port", req("10.0.0.5:4000", "198.51.100.7:5555"), "198.51.100.7"},
		{"ipv6 hop", req("[::1]:4000", "2001:db8::1"), "2001:db8::1"},
		{"bracketed ipv6 hop", req("[::1]:4000", "[2001:db8::2]"), "2001:db8::2"},
		{"only garbage falls back to peer", req("10.0.0.5:4000", "nope"), "10.0.0.5"},
		{"ipv4-mapped peer is trusted", req("[::ffff:10.0.0.5]:4000", "198.51.100.7"), "198.51.100.7"},
		{"ipv6 zone stripped", req("10.0.0.5:4000", "2001:db8::9%eth0"), "2001:db8::9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := def.Resolve(tc.r); got != tc.want {
				t.Errorf("Resolve = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNoneTrustsNothing(t *testing.T) {
	r, err := NewResolver([]string{"none"})
	if err != nil {
		t.Fatalf("none: %v", err)
	}
	if got := r.Resolve(req("127.0.0.1:1", "198.51.100.7")); got != "127.0.0.1" {
		t.Errorf("Resolve = %q, want peer when nothing is trusted", got)
	}
}

func TestCustomTrustedList(t *testing.T) {
	r, err := NewResolver([]string{"203.0.113.10", "198.18.0.0/15"})
	if err != nil {
		t.Fatalf("custom: %v", err)
	}
	// Loopback is NOT trusted in a custom list.
	if got := r.Resolve(req("127.0.0.1:1", "198.51.100.7")); got != "127.0.0.1" {
		t.Errorf("loopback peer: got %q", got)
	}
	if got := r.Resolve(req("203.0.113.10:1", "198.51.100.7, 198.18.0.9")); got != "198.51.100.7" {
		t.Errorf("custom chain: got %q", got)
	}
}

func TestNewResolverRejectsInvalid(t *testing.T) {
	if _, err := NewResolver([]string{"10.0.0.0/99"}); err == nil {
		t.Error("expected error for bad prefix")
	}
	if _, err := NewResolver([]string{"not-an-ip"}); err == nil {
		t.Error("expected error for bad address")
	}
}

func TestMiddlewareAndFrom(t *testing.T) {
	res, _ := NewResolver(nil)
	var got string
	h := Middleware(res)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = From(r)
	}))
	h.ServeHTTP(httptest.NewRecorder(), req("10.0.0.5:4000", "6.6.6.6, 198.51.100.7"))
	if got != "198.51.100.7" {
		t.Errorf("From via middleware = %q", got)
	}

	// Without the middleware, From never trusts headers.
	if g := From(req("10.0.0.5:4000", "198.51.100.7")); g != "10.0.0.5" {
		t.Errorf("From without middleware = %q, want peer", g)
	}
}
