package muxhandlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/vitalvas/kasper/mux"
)

// Default NEL report-to group name used when NELConfig.ReportTo is empty.
const DefaultNELGroup = "nel"

// NEL middleware configuration errors.
var (
	// ErrNELMaxAgeNotPositive is returned when NELConfig.MaxAge is not greater than zero.
	ErrNELMaxAgeNotPositive = errors.New("nel: max_age must be greater than zero")

	// ErrNELSuccessFractionOutOfRange is returned when NELConfig.SuccessFraction is
	// outside the inclusive range [0.0, 1.0].
	ErrNELSuccessFractionOutOfRange = errors.New("nel: success_fraction must be within [0.0, 1.0]")

	// ErrNELFailureFractionOutOfRange is returned when NELConfig.FailureFraction is
	// outside the inclusive range [0.0, 1.0].
	ErrNELFailureFractionOutOfRange = errors.New("nel: failure_fraction must be within [0.0, 1.0]")

	// ErrNELNoReportToGroups is returned when NELConfig.ReportToGroups is empty.
	ErrNELNoReportToGroups = errors.New("nel: at least one Report-To group is required")

	// ErrNELReportToGroupName is returned when a Report-To group has an empty name.
	ErrNELReportToGroupName = errors.New("nel: report-to group name must not be empty")

	// ErrNELReportToGroupMaxAge is returned when a Report-To group has a non-positive max-age.
	ErrNELReportToGroupMaxAge = errors.New("nel: report-to group max_age must be greater than zero")

	// ErrNELReportToGroupNoEndpoints is returned when a Report-To group has no endpoints.
	ErrNELReportToGroupNoEndpoints = errors.New("nel: report-to group must have at least one endpoint")

	// ErrNELEndpointURL is returned when a Report-To endpoint URL is empty or invalid.
	ErrNELEndpointURL = errors.New("nel: report-to endpoint URL must be a valid absolute URL")

	// ErrNELEndpointScheme is returned when a Report-To endpoint URL does not use the
	// HTTPS scheme as required by the Reporting API.
	ErrNELEndpointScheme = errors.New("nel: report-to endpoint URL must use the https scheme")

	// ErrNELEndpointPriority is returned when a Report-To endpoint priority is negative.
	ErrNELEndpointPriority = errors.New("nel: report-to endpoint priority must not be negative")

	// ErrNELEndpointWeight is returned when a Report-To endpoint weight is negative.
	ErrNELEndpointWeight = errors.New("nel: report-to endpoint weight must not be negative")

	// ErrNELReportToGroupNameDuplicate is returned when two Report-To groups share the same name.
	ErrNELReportToGroupNameDuplicate = errors.New("nel: report-to group names must be unique")

	// ErrNELReportToGroupMissing is returned when NELConfig.ReportTo does not match any
	// configured Report-To group name.
	ErrNELReportToGroupMissing = errors.New("nel: NEL report-to group is not declared in ReportToGroups")
)

// ReportToEndpoint represents a single endpoint within a Report-To group per
// the Reporting API. The URL must be an absolute HTTPS URL where reports are POSTed.
type ReportToEndpoint struct {
	// URL is the absolute HTTPS URL that receives report payloads.
	// Required when URLFunc is nil; otherwise it is used as a fallback
	// when URLFunc returns an empty string.
	URL string `json:"url"`

	// URLFunc is called per request to compute the endpoint URL
	// dynamically (e.g. to embed a signed per-request token). It receives
	// the router so the callback can inspect matched route metadata or
	// route names. When set, URLFunc takes priority over URL; when it
	// returns an empty string, URL is used as the fallback. The returned
	// URL is not re-validated at runtime; callers are responsible for
	// returning an absolute https URL.
	URLFunc func(router *mux.Router, r *http.Request) string `json:"-"`

	// Priority controls failover ordering. Lower values are tried first.
	// Endpoints sharing the same priority form a load-balanced pool.
	// Zero is a valid value; negative is rejected.
	Priority int `json:"priority,omitempty"`

	// Weight controls relative load distribution among endpoints with the
	// same priority. Higher weights receive proportionally more reports.
	// Zero is a valid value; negative is rejected.
	Weight int `json:"weight,omitempty"`
}

// ReportToGroup defines a named reporting group in the Report-To header.
// User agents queue reports against the group and POST them to one of its
// endpoints according to priority and weight.
type ReportToGroup struct {
	// Group is the name referenced by NEL and other Reporting API consumers.
	// Required and must be unique within the configuration.
	Group string `json:"group"`

	// MaxAge is the lifetime of the reporting configuration in seconds.
	// User agents discard the configuration after this many seconds.
	// Must be greater than zero.
	MaxAge int `json:"max_age"`

	// IncludeSubdomains, when true, applies the reporting configuration to
	// all subdomains of the origin that served the header.
	IncludeSubdomains bool `json:"include_subdomains,omitempty"`

	// Endpoints lists the endpoints that receive reports for this group.
	// At least one endpoint is required.
	Endpoints []ReportToEndpoint `json:"endpoints"`
}

// NELConfig configures the Network Error Logging middleware.
//
// See: https://www.w3.org/TR/network-error-logging/
// See: https://www.w3.org/TR/reporting-1/
type NELConfig struct {
	// MaxAge is the NEL policy lifetime in seconds. User agents discard the
	// policy after this many seconds. Must be greater than zero.
	MaxAge int

	// ReportTo is the name of the Report-To group that receives NEL reports.
	// Must match one of the groups declared in ReportToGroups.
	// When empty, DefaultNELGroup ("nel") is used.
	ReportTo string

	// IncludeSubdomains, when true, applies the NEL policy to all subdomains
	// of the origin that served the header.
	IncludeSubdomains bool

	// SuccessFraction is the sampling rate for successful requests, in the
	// inclusive range [0.0, 1.0]. When zero, the field is omitted from the
	// header so the user agent applies its default (no success reporting).
	SuccessFraction float64

	// FailureFraction is the sampling rate for failed requests, in the
	// inclusive range [0.0, 1.0]. When zero, the field is omitted from the
	// header so the user agent applies its default (1.0, all failures).
	FailureFraction float64

	// ReportToGroups defines the Report-To header groups served alongside
	// the NEL header. At least one group is required, and the group named
	// by ReportTo (or DefaultNELGroup) must be present.
	ReportToGroups []ReportToGroup
}

// nelPayload is the JSON object value of the NEL header.
type nelPayload struct {
	ReportTo          string  `json:"report_to"`
	MaxAge            int     `json:"max_age"`
	IncludeSubdomains bool    `json:"include_subdomains,omitempty"`
	SuccessFraction   float64 `json:"success_fraction,omitempty"`
	FailureFraction   float64 `json:"failure_fraction,omitempty"`
}

// NELMiddleware returns a middleware that sets the Network Error Logging
// (NEL) header along with the Report-To header that declares where NEL
// reports should be delivered. The router is passed to each endpoint's
// URLFunc so per-request URL computation can inspect matched route
// metadata or route names.
//
// See: https://www.w3.org/TR/network-error-logging/
// See: https://www.w3.org/TR/reporting-1/
func NELMiddleware(router *mux.Router, cfg NELConfig) (mux.MiddlewareFunc, error) {
	if cfg.MaxAge <= 0 {
		return nil, ErrNELMaxAgeNotPositive
	}

	if cfg.SuccessFraction < 0 || cfg.SuccessFraction > 1 {
		return nil, ErrNELSuccessFractionOutOfRange
	}

	if cfg.FailureFraction < 0 || cfg.FailureFraction > 1 {
		return nil, ErrNELFailureFractionOutOfRange
	}

	if len(cfg.ReportToGroups) == 0 {
		return nil, ErrNELNoReportToGroups
	}

	groupName := cfg.ReportTo
	if groupName == "" {
		groupName = DefaultNELGroup
	}

	seen := make(map[string]struct{}, len(cfg.ReportToGroups))
	found := false
	for _, g := range cfg.ReportToGroups {
		if g.Group == "" {
			return nil, ErrNELReportToGroupName
		}
		if _, ok := seen[g.Group]; ok {
			return nil, ErrNELReportToGroupNameDuplicate
		}
		seen[g.Group] = struct{}{}

		if g.MaxAge <= 0 {
			return nil, ErrNELReportToGroupMaxAge
		}
		if len(g.Endpoints) == 0 {
			return nil, ErrNELReportToGroupNoEndpoints
		}

		for _, ep := range g.Endpoints {
			if err := validateNELEndpoint(ep); err != nil {
				return nil, err
			}
		}

		if g.Group == groupName {
			found = true
		}
	}

	if !found {
		return nil, ErrNELReportToGroupMissing
	}

	nelHeader, err := buildNELHeader(groupName, cfg)
	if err != nil {
		return nil, err
	}

	dynamic := hasDynamicEndpoints(cfg.ReportToGroups)

	var staticReportTo string
	if !dynamic {
		staticReportTo, err = buildReportToHeader(cfg.ReportToGroups, nil, nil)
		if err != nil {
			return nil, err
		}
	}

	groups := cfg.ReportToGroups

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("NEL", nelHeader)

			if dynamic {
				value, ferr := buildReportToHeader(groups, router, r)
				if ferr == nil {
					h.Set("Report-To", value)
				}
			} else {
				h.Set("Report-To", staticReportTo)
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}

func hasDynamicEndpoints(groups []ReportToGroup) bool {
	for _, g := range groups {
		for _, ep := range g.Endpoints {
			if ep.URLFunc != nil {
				return true
			}
		}
	}
	return false
}

func validateNELEndpoint(ep ReportToEndpoint) error {
	trimmed := strings.TrimSpace(ep.URL)
	if trimmed == "" {
		if ep.URLFunc == nil {
			return ErrNELEndpointURL
		}
	} else {
		u, err := url.Parse(ep.URL)
		if err != nil || !u.IsAbs() || u.Host == "" {
			return ErrNELEndpointURL
		}
		if u.Scheme != "https" {
			return ErrNELEndpointScheme
		}
	}
	if ep.Priority < 0 {
		return ErrNELEndpointPriority
	}
	if ep.Weight < 0 {
		return ErrNELEndpointWeight
	}
	return nil
}

func buildNELHeader(group string, cfg NELConfig) (string, error) {
	payload := nelPayload{
		ReportTo:          group,
		MaxAge:            cfg.MaxAge,
		IncludeSubdomains: cfg.IncludeSubdomains,
		SuccessFraction:   cfg.SuccessFraction,
		FailureFraction:   cfg.FailureFraction,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func buildReportToHeader(groups []ReportToGroup, router *mux.Router, r *http.Request) (string, error) {
	parts := make([]string, 0, len(groups))
	for _, g := range groups {
		resolved := g
		resolved.Endpoints = make([]ReportToEndpoint, 0, len(g.Endpoints))
		for _, ep := range g.Endpoints {
			out := ep
			if ep.URLFunc != nil && r != nil {
				if dynamic := ep.URLFunc(router, r); dynamic != "" {
					out.URL = dynamic
				}
			}
			if out.URL == "" {
				continue
			}
			resolved.Endpoints = append(resolved.Endpoints, out)
		}
		if len(resolved.Endpoints) == 0 {
			continue
		}
		buf, err := json.Marshal(resolved)
		if err != nil {
			return "", err
		}
		parts = append(parts, string(buf))
	}
	return strings.Join(parts, ", "), nil
}
