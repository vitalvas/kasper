package muxhandlers

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// ErrInvalidProxy is returned when a TrustedProxies entry is neither a valid
// IP address nor a valid CIDR range.
var ErrInvalidProxy = errors.New("proxy headers: invalid proxy entry")

// DefaultTrustedProxies is the set of private and loopback ranges used when
// ProxyHeadersConfig.TrustedProxies is empty.
//
// Included ranges:
//   - 127.0.0.0/8    — IPv4 loopback (RFC 1122)
//   - 10.0.0.0/8     — Class A private (RFC 1918)
//   - 172.16.0.0/12  — Class B private (RFC 1918)
//   - 192.168.0.0/16 — Class C private (RFC 1918)
//   - 100.64.0.0/10  — CGNAT shared address space (RFC 6598)
//   - ::1/128        — IPv6 loopback (RFC 4291)
//   - fc00::/7       — IPv6 unique local (RFC 4193)
var DefaultTrustedProxies = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"100.64.0.0/10",
	"::1/128",
	"fc00::/7",
}

// ProxyHeadersConfig configures the ProxyHeaders middleware behaviour.
type ProxyHeadersConfig struct {
	// TrustedProxies is a list of IP addresses and CIDR ranges.
	// Forwarding headers are only honoured when r.RemoteAddr is in this set.
	// When empty, DefaultTrustedProxies (private/loopback ranges) is used.
	// Examples: "10.0.0.1", "192.168.0.0/16", "::1", "fd00::/8"
	TrustedProxies []string

	// EnableForwarded enables parsing of the RFC 7239 Forwarded header.
	// When enabled, the Forwarded header is used as a fallback after the
	// de-facto X-Forwarded-* and X-Real-IP headers.
	//
	// Spec reference: https://www.rfc-editor.org/rfc/rfc7239
	EnableForwarded bool
}

// proxyTrustSet holds pre-parsed IPs and CIDRs for fast runtime lookup.
type proxyTrustSet struct {
	ips  []net.IP
	nets []*net.IPNet
}

// ProxyHeadersMiddleware returns a middleware that populates request fields
// from reverse proxy headers when the request originates from a trusted proxy.
//
// Supported headers (checked in priority order):
//   - r.RemoteAddr: X-Forwarded-For > X-Real-IP [> Forwarded for=]
//   - r.URL.Scheme: X-Forwarded-Proto > X-Forwarded-Scheme [> Forwarded proto=]
//   - r.Host:       X-Forwarded-Host [> Forwarded host=]
//   - X-Forwarded-By header: [Forwarded by=]
//
// Bracketed entries require EnableForwarded (RFC 7239).
// The by= directive is exposed as a synthetic X-Forwarded-By request header.
//
// When TrustedProxies is empty, DefaultTrustedProxies (private RFC 1918/4193
// and loopback ranges) is used.
//
// It returns an error if the configuration contains unparseable IP/CIDR entries.
func ProxyHeadersMiddleware(cfg ProxyHeadersConfig) (mux.MiddlewareFunc, error) {
	proxies := cfg.TrustedProxies
	if len(proxies) == 0 {
		proxies = DefaultTrustedProxies
	}

	ts, err := parseTrustedProxies(proxies)
	if err != nil {
		return nil, err
	}

	enableFwd := cfg.EnableForwarded

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isTrustedPeer(r.RemoteAddr, ts) {
				next.ServeHTTP(w, r)
				return
			}

			// Optionally pre-parse RFC 7239 Forwarded header for fallback.
			var fwd forwardedParams
			if enableFwd {
				fwd = parseForwarded(r.Header.Get("Forwarded"))
			}

			// Client IP: X-Forwarded-For > X-Real-IP > Forwarded for=.
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				if ip := parseXForwardedFor(xff); ip != "" {
					r.RemoteAddr = ip
				}
			} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
				if ip := net.ParseIP(strings.TrimSpace(realIP)); ip != nil {
					r.RemoteAddr = strings.TrimSpace(realIP)
				}
			} else if fwd.forIP != "" {
				r.RemoteAddr = fwd.forIP
			}

			// Scheme: X-Forwarded-Proto > X-Forwarded-Scheme > Forwarded proto=.
			if scheme := proxyScheme(r); scheme != "" {
				u := *r.URL
				u.Scheme = scheme
				r.URL = &u
			} else if fwd.proto != "" {
				u := *r.URL
				u.Scheme = fwd.proto
				r.URL = &u
			}

			// Host: X-Forwarded-Host > Forwarded host=.
			if host := r.Header.Get("X-Forwarded-Host"); host != "" {
				r.Host = host
			} else if fwd.host != "" {
				r.Host = fwd.host
			}

			// By: Forwarded by= -> X-Forwarded-By synthetic header.
			if fwd.by != "" {
				r.Header.Set("X-Forwarded-By", fwd.by)
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

// proxyScheme returns the normalized scheme from X-Forwarded-Proto or
// X-Forwarded-Scheme. Returns empty string if neither header is present or the
// value is not http/https.
func proxyScheme(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-Proto", "X-Forwarded-Scheme"} {
		if val := r.Header.Get(header); val != "" {
			normalized := strings.ToLower(strings.TrimSpace(val))
			if normalized == "http" || normalized == "https" {
				return normalized
			}

			return ""
		}
	}

	return ""
}

// parseTrustedProxies parses a list of IP addresses and CIDR ranges into a
// proxyTrustSet. It returns an error wrapping ErrInvalidProxy for any entry
// that is neither a valid IP nor a valid CIDR.
func parseTrustedProxies(entries []string) (*proxyTrustSet, error) {
	ts := &proxyTrustSet{}

	for _, entry := range entries {
		if strings.Contains(entry, "/") {
			_, ipNet, err := net.ParseCIDR(entry)
			if err != nil {
				return nil, fmt.Errorf("%w: %q", ErrInvalidProxy, entry)
			}

			ts.nets = append(ts.nets, ipNet)
		} else {
			ip := net.ParseIP(entry)
			if ip == nil {
				return nil, fmt.Errorf("%w: %q", ErrInvalidProxy, entry)
			}

			ts.ips = append(ts.ips, ip)
		}
	}

	return ts, nil
}

// isTrustedPeer reports whether the peer address (r.RemoteAddr) is in the
// trusted proxy set.
func isTrustedPeer(remoteAddr string, ts *proxyTrustSet) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// RemoteAddr may be a bare IP without port.
		host = remoteAddr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, trusted := range ts.ips {
		if trusted.Equal(ip) {
			return true
		}
	}

	for _, ipNet := range ts.nets {
		if ipNet.Contains(ip) {
			return true
		}
	}

	return false
}

// parseXForwardedFor returns the leftmost valid IP from a comma-separated
// X-Forwarded-For header value. Returns an empty string if no valid IP is
// found.
func parseXForwardedFor(xff string) string {
	for part := range strings.SplitSeq(xff, ",") {
		candidate := strings.TrimSpace(part)
		if ip := net.ParseIP(candidate); ip != nil {
			return candidate
		}
	}

	return ""
}

// forwardedParams holds the extracted directives from the first element of an
// RFC 7239 Forwarded header value.
type forwardedParams struct {
	forIP string // validated IP from "for=" directive
	by    string // raw value from "by=" directive (proxy interface identifier)
	proto string // normalized scheme from "proto=" directive (http or https)
	host  string // raw value from "host=" directive
}

// parseForwarded extracts for=, proto=, and host= from the first element of
// an RFC 7239 Forwarded header. Multiple elements are comma-separated; only the
// first is used (the client-facing proxy).
//
// Spec reference: https://www.rfc-editor.org/rfc/rfc7239
func parseForwarded(header string) forwardedParams {
	if header == "" {
		return forwardedParams{}
	}

	// Take only the first element (before the first comma).
	if idx := strings.IndexByte(header, ','); idx != -1 {
		header = header[:idx]
	}

	var result forwardedParams

	for param := range strings.SplitSeq(header, ";") {
		param = strings.TrimSpace(param)

		eqIdx := strings.IndexByte(param, '=')
		if eqIdx == -1 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(param[:eqIdx]))
		val := strings.TrimSpace(param[eqIdx+1:])

		switch key {
		case "for":
			result.forIP = parseForwardedIP(val)
		case "proto":
			val = strings.ToLower(strings.Trim(val, `"`))
			if val == "http" || val == "https" {
				result.proto = val
			}
		case "by":
			val = strings.Trim(val, `"`)
			if val != "" {
				result.by = val
			}
		case "host":
			val = strings.Trim(val, `"`)
			if val != "" {
				result.host = val
			}
		}
	}

	return result
}

// parseForwardedIP extracts and validates an IP from a Forwarded for= value.
// IPv6 addresses are quoted and may include brackets and ports, e.g.:
//
//	for=192.0.2.60
//	for="[2001:db8::1]"
//	for="[2001:db8::1]:4711"
//	for="_hidden"
func parseForwardedIP(val string) string {
	val = strings.Trim(val, `"`)

	// Handle [host]:port format (e.g. [2001:db8::1]:4711).
	if host, _, err := net.SplitHostPort(val); err == nil {
		val = host
	} else {
		// Handle [host] without port (e.g. [2001:db8::1]).
		val = strings.TrimPrefix(val, "[")
		val = strings.TrimSuffix(val, "]")
	}

	if ip := net.ParseIP(val); ip != nil {
		return val
	}

	return ""
}
