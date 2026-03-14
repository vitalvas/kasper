package muxhandlers

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// ErrIPAllowEmpty is returned when IPAllowConfig.Allowed is empty.
var ErrIPAllowEmpty = errors.New("ip allow: allowed list must not be empty")

// ErrIPAllowInvalidEntry is returned when an Allowed entry is neither a valid
// IP address nor a valid CIDR range.
var ErrIPAllowInvalidEntry = errors.New("ip allow: invalid entry")

// IPAllowConfig configures the IP allow middleware.
type IPAllowConfig struct {
	// Allowed is a list of IP addresses and CIDR ranges that are permitted
	// to access the protected routes. Required; must contain at least one
	// entry. Bare IPs are normalized to /32 (IPv4) or /128 (IPv6).
	// Examples: "10.0.0.1", "192.168.0.0/16", "::1", "fd00::/8"
	Allowed []string

	// DeniedHandler is called when the client IP is not in the allowed
	// list. When nil, a default handler returns 403 Forbidden with an
	// empty body.
	DeniedHandler http.Handler
}

// IPAllowMiddleware returns a middleware that restricts access to requests
// originating from the configured IP addresses and CIDR ranges. Requests
// from IPs not in the allowed list are rejected. The client IP is
// extracted from r.RemoteAddr.
func IPAllowMiddleware(cfg IPAllowConfig) (mux.MiddlewareFunc, error) {
	if len(cfg.Allowed) == 0 {
		return nil, ErrIPAllowEmpty
	}

	nets, err := parseIPAllowNets(cfg.Allowed)
	if err != nil {
		return nil, err
	}

	denied := cfg.DeniedHandler
	if denied == nil {
		denied = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isAllowedIP(r.RemoteAddr, nets) {
				denied.ServeHTTP(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

// parseIPAllowNets parses a list of IP addresses and CIDR ranges into a
// slice of *net.IPNet. Bare IPs are converted to single-host CIDRs
// (/32 for IPv4, /128 for IPv6). It returns an error wrapping
// ErrIPAllowInvalidEntry for any invalid entry.
func parseIPAllowNets(entries []string) ([]*net.IPNet, error) {
	nets := make([]*net.IPNet, 0, len(entries))

	for _, entry := range entries {
		if strings.Contains(entry, "/") {
			_, ipNet, err := net.ParseCIDR(entry)
			if err != nil {
				return nil, fmt.Errorf("%w: %q", ErrIPAllowInvalidEntry, entry)
			}

			nets = append(nets, ipNet)
		} else {
			ip := net.ParseIP(entry)
			if ip == nil {
				return nil, fmt.Errorf("%w: %q", ErrIPAllowInvalidEntry, entry)
			}

			mask := net.CIDRMask(128, 128)
			if ip.To4() != nil {
				mask = net.CIDRMask(32, 32)
			}

			nets = append(nets, &net.IPNet{IP: ip, Mask: mask})
		}
	}

	return nets, nil
}

// isAllowedIP reports whether the remote address is in the allowed nets.
func isAllowedIP(remoteAddr string, nets []*net.IPNet) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, ipNet := range nets {
		if ipNet.Contains(ip) {
			return true
		}
	}

	return false
}
