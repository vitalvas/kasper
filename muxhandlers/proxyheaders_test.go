package muxhandlers

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.vitalvas.com/kasper/mux"
)

func TestProxyHeadersMiddleware(t *testing.T) {
	t.Run("config validation", func(t *testing.T) {
		tests := []struct {
			name    string
			config  ProxyHeadersConfig
			wantErr error
		}{
			{"invalid IP entry", ProxyHeadersConfig{TrustedProxies: []string{"not-an-ip"}}, ErrInvalidProxy},
			{"invalid CIDR entry", ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.0/99"}}, ErrInvalidProxy},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := ProxyHeadersMiddleware(tt.config)
				assert.ErrorIs(t, err, tt.wantErr)
			})
		}

		t.Run("valid IPs and CIDRs accepted", func(t *testing.T) {
			_, err := ProxyHeadersMiddleware(ProxyHeadersConfig{
				TrustedProxies: []string{"10.0.0.1", "192.168.0.0/16", "::1", "fd00::/8"},
			})
			assert.NoError(t, err)
		})
	})

	type middlewareTest struct {
		name        string
		config      ProxyHeadersConfig
		remoteAddr  string
		initialHost string
		headers     map[string]string
		wantAddr    string
		wantScheme  string
		wantHost    string
		wantBy      string
	}

	tests := []middlewareTest{
		// Default trusted proxies
		{
			name:       "empty config trusts private peer",
			config:     ProxyHeadersConfig{},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			wantAddr:   "203.0.113.50",
		},
		{
			name:       "empty config rejects public peer",
			config:     ProxyHeadersConfig{},
			remoteAddr: "203.0.113.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.10"},
			wantAddr:   "203.0.113.1:8080",
		},

		// Trust verification
		{
			name:       "untrusted peer passes through unchanged",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "192.168.1.100:12345",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50", "X-Forwarded-Proto": "https"},
			wantAddr:   "192.168.1.100:12345",
		},
		{
			name:       "trusted peer by exact IP",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			wantAddr:   "203.0.113.50",
		},
		{
			name:       "trusted peer by CIDR",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.0/8"}},
			remoteAddr: "10.99.88.77:1234",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			wantAddr:   "203.0.113.50",
		},

		// X-Forwarded-For
		{
			name:       "XFF single IP",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.10"},
			wantAddr:   "198.51.100.10",
		},
		{
			name:       "XFF chain uses first untrusted address from right",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50, 198.51.100.10, 10.0.0.1"},
			wantAddr:   "198.51.100.10",
		},
		{
			name:       "XFF ignores attacker supplied leftmost address",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.0/8"}},
			remoteAddr: "10.0.0.2:8080",
			headers:    map[string]string{"X-Forwarded-For": "192.0.2.123, 198.51.100.10, 10.0.0.1"},
			wantAddr:   "198.51.100.10",
		},
		{
			name:       "XFF invalid IP ignored",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "not-valid"},
			wantAddr:   "10.0.0.1:8080",
		},
		{
			name:       "XFF with IPv6 address",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "2001:db8::1"},
			wantAddr:   "2001:db8::1",
		},

		// X-Real-IP
		{
			name:       "X-Real-IP sets RemoteAddr when no XFF",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Real-IP": "203.0.113.50"},
			wantAddr:   "203.0.113.50",
		},
		{
			name:       "XFF takes priority over X-Real-IP",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.10", "X-Real-IP": "203.0.113.50"},
			wantAddr:   "198.51.100.10",
		},
		{
			name:       "X-Real-IP invalid ignored",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Real-IP": "not-valid"},
			wantAddr:   "10.0.0.1:8080",
		},
		{
			name:       "X-Real-IP with IPv6",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Real-IP": "2001:db8::1"},
			wantAddr:   "2001:db8::1",
		},

		// X-Forwarded-Proto
		{
			name:       "XFP http",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Proto": "http"},
			wantAddr:   "10.0.0.1:8080",
			wantScheme: "http",
		},
		{
			name:       "XFP https",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Proto": "https"},
			wantAddr:   "10.0.0.1:8080",
			wantScheme: "https",
		},
		{
			name:       "XFP invalid value ignored",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Proto": "ftp"},
			wantAddr:   "10.0.0.1:8080",
		},
		{
			name:       "XFP case insensitive",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Proto": "HTTPS"},
			wantAddr:   "10.0.0.1:8080",
			wantScheme: "https",
		},

		// X-Forwarded-Scheme
		{
			name:       "XFS sets Scheme when no XFP",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Scheme": "https"},
			wantAddr:   "10.0.0.1:8080",
			wantScheme: "https",
		},
		{
			name:       "XFP takes priority over XFS",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Proto": "http", "X-Forwarded-Scheme": "https"},
			wantAddr:   "10.0.0.1:8080",
			wantScheme: "http",
		},
		{
			name:       "XFS invalid value ignored",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Scheme": "ftp"},
			wantAddr:   "10.0.0.1:8080",
		},
		{
			name:       "XFS case insensitive",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Scheme": "HTTPS"},
			wantAddr:   "10.0.0.1:8080",
			wantScheme: "https",
		},

		// X-Forwarded-Host
		{
			name:       "XFH sets Host",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Host": "example.com"},
			wantAddr:   "10.0.0.1:8080",
			wantHost:   "example.com",
		},
		{
			name:       "XFH with port",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Host": "example.com:443"},
			wantAddr:   "10.0.0.1:8080",
			wantHost:   "example.com:443",
		},
		{
			name:        "XFH absent leaves Host unchanged",
			config:      ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr:  "10.0.0.1:8080",
			initialHost: "original.com",
			wantAddr:    "10.0.0.1:8080",
			wantHost:    "original.com",
		},
		{
			name:        "untrusted peer ignores XFH",
			config:      ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr:  "192.168.1.100:12345",
			initialHost: "original.com",
			headers:     map[string]string{"X-Forwarded-Host": "spoofed.com"},
			wantAddr:    "192.168.1.100:12345",
			wantHost:    "original.com",
		},

		// RFC 7239 Forwarded header
		{
			name:       "Forwarded ignored when disabled",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"Forwarded": "for=203.0.113.50;proto=https;host=example.com"},
			wantAddr:   "10.0.0.1:8080",
		},
		{
			name:       "Forwarded for= as fallback",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}, EnableForwarded: true},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"Forwarded": "for=203.0.113.50"},
			wantAddr:   "203.0.113.50",
		},
		{
			name:       "Forwarded proto= as fallback",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}, EnableForwarded: true},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"Forwarded": "for=203.0.113.50;proto=https"},
			wantAddr:   "203.0.113.50",
			wantScheme: "https",
		},
		{
			name:       "Forwarded host= as fallback",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}, EnableForwarded: true},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"Forwarded": "for=203.0.113.50;host=example.com"},
			wantAddr:   "203.0.113.50",
			wantHost:   "example.com",
		},
		{
			name:       "XFF takes priority over Forwarded for=",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}, EnableForwarded: true},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.10", "Forwarded": "for=203.0.113.50"},
			wantAddr:   "198.51.100.10",
		},
		{
			name:       "XFP takes priority over Forwarded proto=",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}, EnableForwarded: true},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Proto": "http", "Forwarded": "proto=https"},
			wantAddr:   "10.0.0.1:8080",
			wantScheme: "http",
		},
		{
			name:       "XFH takes priority over Forwarded host=",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}, EnableForwarded: true},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-Host": "primary.com", "Forwarded": "host=fallback.com"},
			wantAddr:   "10.0.0.1:8080",
			wantHost:   "primary.com",
		},
		{
			name:       "Forwarded all directives",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}, EnableForwarded: true},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"Forwarded": "for=192.0.2.60;proto=https;by=203.0.113.43;host=example.com"},
			wantAddr:   "192.0.2.60",
			wantScheme: "https",
			wantHost:   "example.com",
			wantBy:     "203.0.113.43",
		},
		{
			name:       "Forwarded by= sets X-Forwarded-By",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}, EnableForwarded: true},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"Forwarded": "for=192.0.2.60;by=203.0.113.43"},
			wantAddr:   "192.0.2.60",
			wantBy:     "203.0.113.43",
		},
		{
			name:       "Forwarded by= absent no X-Forwarded-By",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}, EnableForwarded: true},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"Forwarded": "for=192.0.2.60;proto=https"},
			wantAddr:   "192.0.2.60",
			wantScheme: "https",
		},
		{
			name:       "Forwarded by= obfuscated identifier",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}, EnableForwarded: true},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"Forwarded": "for=192.0.2.60;by=_hidden"},
			wantAddr:   "192.0.2.60",
			wantBy:     "_hidden",
		},
		{
			name:       "Forwarded by= not set when disabled",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"Forwarded": "for=192.0.2.60;by=203.0.113.43"},
			wantAddr:   "10.0.0.1:8080",
		},
		{
			name:       "Forwarded IPv6 for= value",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}, EnableForwarded: true},
			remoteAddr: "10.0.0.1:8080",
			headers:    map[string]string{"Forwarded": `for="[2001:db8::1]"`},
			wantAddr:   "2001:db8::1",
		},
		{
			name:       "Forwarded chain ignores attacker supplied leftmost element",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.0/8"}, EnableForwarded: true},
			remoteAddr: "10.0.0.2:8080",
			headers:    map[string]string{"Forwarded": "for=192.0.2.123;proto=http;host=evil.example, for=198.51.100.10;proto=https;host=good.example, for=10.0.0.1"},
			wantAddr:   "198.51.100.10",
			wantScheme: "https",
			wantHost:   "good.example",
		},
		{
			name:       "Forwarded malformed chain is rejected",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.0/8"}, EnableForwarded: true},
			remoteAddr: "10.0.0.2:8080",
			headers:    map[string]string{"Forwarded": "for=_hidden, for=198.51.100.10, for=10.0.0.1"},
			wantAddr:   "10.0.0.2:8080",
		},

		// IPv6 edge cases
		{
			name:       "trusted peer IPv6 loopback",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"::1"}},
			remoteAddr: "[::1]:8080",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			wantAddr:   "203.0.113.50",
		},
		{
			name:       "trusted peer IPv6 CIDR",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"fd00::/8"}},
			remoteAddr: "[fd12:3456:789a::1]:8080",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			wantAddr:   "203.0.113.50",
		},

		// Other edge cases
		{
			name:       "RemoteAddr bare IP without port",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			wantAddr:   "203.0.113.50",
		},
		{
			name:       "no forwarding headers unchanged",
			config:     ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}},
			remoteAddr: "10.0.0.1:8080",
			wantAddr:   "10.0.0.1:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedAddr, capturedScheme, capturedHost, capturedBy string

			r := mux.NewRouter()
			r.HandleFunc("/test", func(_ http.ResponseWriter, req *http.Request) {
				capturedAddr = req.RemoteAddr
				capturedScheme = req.URL.Scheme
				capturedHost = req.Host
				capturedBy = req.Header.Get("X-Forwarded-By")
			}).Methods(http.MethodGet)

			mw, err := ProxyHeadersMiddleware(tt.config)
			require.NoError(t, err)
			r.Use(mw)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.initialHost != "" {
				req.Host = tt.initialHost
			}
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			r.ServeHTTP(httptest.NewRecorder(), req)

			assert.Equal(t, tt.wantAddr, capturedAddr, "RemoteAddr")
			assert.Equal(t, tt.wantScheme, capturedScheme, "Scheme")
			if tt.wantHost != "" {
				assert.Equal(t, tt.wantHost, capturedHost, "Host")
			}
			assert.Equal(t, tt.wantBy, capturedBy, "X-Forwarded-By")
		})
	}

	t.Run("scheme copy does not affect original URL", func(t *testing.T) {
		originalURL := &url.URL{Path: "/test"}

		r := mux.NewRouter()
		r.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet)

		mw, err := ProxyHeadersMiddleware(ProxyHeadersConfig{TrustedProxies: []string{"10.0.0.1"}})
		require.NoError(t, err)
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:8080"
		req.URL = originalURL
		req.Header.Set("X-Forwarded-Proto", "https")
		r.ServeHTTP(httptest.NewRecorder(), req)

		assert.Empty(t, originalURL.Scheme)
	})
}

func TestIsTrustedPeer(t *testing.T) {
	ts := &proxyTrustSet{
		ips:  []net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("::1")},
		nets: []*net.IPNet{mustParseCIDR("192.168.0.0/16")},
	}

	tests := []struct {
		name string
		addr string
		want bool
	}{
		{"exact IPv4 with port", "10.0.0.1:8080", true},
		{"exact IPv6 with port", "[::1]:8080", true},
		{"CIDR match", "192.168.1.100:1234", true},
		{"no match", "172.16.0.1:8080", false},
		{"bare IP without port", "10.0.0.1", true},
		{"unparseable address", "not-an-ip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isTrustedPeer(tt.addr, ts))
		})
	}
}

func TestClientIPFromXForwardedFor(t *testing.T) {
	trusted, err := parseTrustedProxies([]string{"10.0.0.0/8"})
	require.NoError(t, err)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single valid IP", "203.0.113.50", "203.0.113.50"},
		{"multiple untrusted IPs returns rightmost", "203.0.113.50, 198.51.100.10", "198.51.100.10"},
		{"trusted hops are skipped", "203.0.113.50, 10.0.0.1, 10.0.0.2", "203.0.113.50"},
		{"malformed chain is rejected", "garbage, 198.51.100.10", ""},
		{"all invalid returns empty", "not-valid, also-bad", ""},
		{"IPv6 address", "2001:db8::1", "2001:db8::1"},
		{"empty string", "", ""},
		{"whitespace handling", "  203.0.113.50  ,  198.51.100.10  ", "198.51.100.10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clientIPFromXForwardedFor(tt.input, trusted))
		})
	}
}

func TestParseForwarded(t *testing.T) {
	trusted, err := parseTrustedProxies([]string{"10.0.0.0/8"})
	require.NoError(t, err)

	tests := []struct {
		name      string
		input     string
		wantForIP string
		wantBy    string
		wantProto string
		wantHost  string
	}{
		{"empty string", "", "", "", "", ""},
		{"all directives", "for=192.0.2.60;proto=https;by=203.0.113.43;host=example.com", "192.0.2.60", "203.0.113.43", "https", "example.com"},
		{"only by directive is rejected without for", "by=203.0.113.43", "", "", "", ""},
		{"by obfuscated identifier is rejected without for", "by=_proxy01", "", "", "", ""},
		{"by quoted value is rejected without for", `by="203.0.113.43"`, "", "", "", ""},
		{"only for directive", "for=192.0.2.60", "192.0.2.60", "", "", ""},
		{"only proto directive is rejected without for", "proto=http", "", "", "", ""},
		{"only host directive is rejected without for", "host=example.com", "", "", "", ""},
		{"multiple untrusted elements uses rightmost", "for=192.0.2.43, for=198.51.100.17", "198.51.100.17", "", "", ""},
		{"trusted elements are skipped", "for=192.0.2.43;proto=https, for=10.0.0.1", "192.0.2.43", "", "https", ""},
		{"all trusted elements return empty", "for=10.0.0.1, for=10.0.0.2", "", "", "", ""},
		{"IPv6 for quoted with brackets", `for="[2001:db8::1]"`, "2001:db8::1", "", "", ""},
		{"IPv6 for with port", `for="[2001:db8::1]:4711"`, "2001:db8::1", "", "", ""},
		{"invalid for value", `for="_gazonk"`, "", "", "", ""},
		{"invalid proto value", "for=192.0.2.60;proto=ftp", "192.0.2.60", "", "", ""},
		{"proto case insensitive", "for=192.0.2.60;proto=HTTPS", "192.0.2.60", "", "https", ""},
		{"quoted host", `for=192.0.2.60;host="example.com:443"`, "192.0.2.60", "", "", "example.com:443"},
		{"spacing around semicolons", "for=192.0.2.60 ; proto=https ; host=example.com", "192.0.2.60", "", "https", "example.com"},
		{"case insensitive keys", "For=192.0.2.60;Proto=HTTPS;Host=example.com", "192.0.2.60", "", "https", "example.com"},
		{"no equals sign in param", "garbage;for=192.0.2.60", "192.0.2.60", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseForwarded(tt.input, trusted)
			assert.Equal(t, tt.wantForIP, result.forIP, "forIP")
			assert.Equal(t, tt.wantBy, result.by, "by")
			assert.Equal(t, tt.wantProto, result.proto, "proto")
			assert.Equal(t, tt.wantHost, result.host, "host")
		})
	}
}

func TestParseForwardedIP(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain IPv4", "192.0.2.60", "192.0.2.60"},
		{"quoted IPv4", `"192.0.2.60"`, "192.0.2.60"},
		{"bracketed IPv6", `"[2001:db8::1]"`, "2001:db8::1"},
		{"bracketed IPv6 with port", `"[2001:db8::1]:4711"`, "2001:db8::1"},
		{"obfuscated identifier", "_hidden", ""},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseForwardedIP(tt.input))
		})
	}
}

func FuzzProxyHeaderChains(f *testing.F) {
	f.Add("203.0.113.10, 10.0.0.2", "for=203.0.113.10, for=10.0.0.2;proto=https")
	f.Add("2001:db8::1, fd00::2", `for="[2001:db8::1]", for="[fd00::2]"`)
	f.Add("invalid, 10.0.0.2", "for=_hidden, for=10.0.0.2")
	f.Add("", "")

	trusted, err := parseTrustedProxies([]string{"10.0.0.0/8", "fd00::/8"})
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, xForwardedFor, forwarded string) {
		if clientIP := clientIPFromXForwardedFor(xForwardedFor, trusted); clientIP != "" {
			ip := net.ParseIP(clientIP)
			if ip == nil || isTrustedIP(ip, trusted) {
				t.Fatalf("invalid X-Forwarded-For result %q", clientIP)
			}
		}

		result := parseForwarded(forwarded, trusted)
		if result.forIP != "" {
			ip := net.ParseIP(result.forIP)
			if ip == nil || isTrustedIP(ip, trusted) {
				t.Fatalf("invalid Forwarded result %q", result.forIP)
			}
		}
		if result.proto != "" && result.proto != "http" && result.proto != "https" {
			t.Fatalf("invalid Forwarded protocol %q", result.proto)
		}
	})
}

func mustParseCIDR(s string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}

	return ipNet
}

func BenchmarkProxyHeaders(b *testing.B) {
	b.Run("trusted peer with X-Forwarded-For", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := ProxyHeadersMiddleware(ProxyHeadersConfig{
			TrustedProxies: []string{"10.0.0.1", "192.168.0.0/16"},
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:8080"
		req.Header.Set("X-Forwarded-For", "203.0.113.50")
		req.Header.Set("X-Forwarded-Proto", "https")

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("trusted peer with Forwarded header", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := ProxyHeadersMiddleware(ProxyHeadersConfig{
			TrustedProxies:  []string{"10.0.0.1"},
			EnableForwarded: true,
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:8080"
		req.Header.Set("Forwarded", "for=203.0.113.50;proto=https;host=example.com")

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("untrusted peer passthrough", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := ProxyHeadersMiddleware(ProxyHeadersConfig{
			TrustedProxies: []string{"10.0.0.1"},
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.50")

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}

func BenchmarkProxyHeaderChainScaling(b *testing.B) {
	trusted, err := parseTrustedProxies([]string{"10.0.0.0/8"})
	if err != nil {
		b.Fatal(err)
	}

	for _, count := range []int{16, 256, 4096} {
		b.Run(fmt.Sprintf("X-Forwarded-For/hops=%d", count), func(b *testing.B) {
			parts := make([]string, count)
			parts[0] = "203.0.113.10"
			for i := 1; i < count; i++ {
				parts[i] = fmt.Sprintf("10.%d.%d.%d", byte(i>>16), byte(i>>8), byte(i))
			}
			header := strings.Join(parts, ", ")
			b.ReportAllocs()
			b.SetBytes(int64(len(header)))
			for b.Loop() {
				if got := clientIPFromXForwardedFor(header, trusted); got != parts[0] {
					b.Fatalf("got %q, want %q", got, parts[0])
				}
			}
		})

		b.Run(fmt.Sprintf("Forwarded/hops=%d", count), func(b *testing.B) {
			parts := make([]string, count)
			parts[0] = "for=203.0.113.10;proto=https"
			for i := 1; i < count; i++ {
				parts[i] = fmt.Sprintf("for=10.%d.%d.%d", byte(i>>16), byte(i>>8), byte(i))
			}
			header := strings.Join(parts, ", ")
			b.ReportAllocs()
			b.SetBytes(int64(len(header)))
			for b.Loop() {
				if got := parseForwarded(header, trusted); got.forIP != "203.0.113.10" {
					b.Fatalf("got %q, want %q", got.forIP, "203.0.113.10")
				}
			}
		})
	}
}
