package muxhandlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vitalvas/kasper/mux"
)

// HTCPCP method tokens defined by RFC 2324 Section 2.1.1 and extended
// for tea by RFC 7168 Section 2.1.1.
const (
	MethodBrew = "BREW"
	MethodWhen = "WHEN"
)

// StatusImATeapot is the HTCPCP status code returned when a teapot is
// asked to brew coffee (RFC 2324 Section 2.3.2, RFC 7168 Section 2.3.3).
// Mirrors http.StatusTeapot from the standard library for callers that
// prefer the protocol-native name.
const StatusImATeapot = http.StatusTeapot

// ContentTypeMessageCoffeePot is the message/coffeepot media type
// returned by a coffee pot to a successful BREW (RFC 2324 Section 4).
const ContentTypeMessageCoffeePot = "message/coffeepot"

// ContentTypeMessageTeapot is the message/teapot media type returned by
// a teapot to a successful BREW (RFC 7168 Section 2.3.1).
const ContentTypeMessageTeapot = "message/teapot"

// PotType identifies the kind of pot the middleware represents.
type PotType int

const (
	// PotCoffee is a coffee pot. Brews coffee; refuses tea per RFC 7168
	// Section 2.1.1 (only teapots brew tea).
	PotCoffee PotType = iota

	// PotTeapot is a teapot. Brews tea; refuses coffee with 418 per
	// RFC 2324 Section 2.3.2 and RFC 7168 Section 2.3.3.
	PotTeapot
)

// DefaultTeaVarieties is the tea variety registry from RFC 7168
// Section 2.1.1, used when HTCPCPConfig.Teas is nil for a teapot.
var DefaultTeaVarieties = []string{
	"black",
	"chai",
	"earl-grey",
	"english-breakfast",
	"green",
	"jasmine",
	"oolong",
	"peppermint",
	"rooibos",
}

// HTCPCPConfig configures the HTCPCP-TEA middleware.
type HTCPCPConfig struct {
	// PotType selects coffee pot or teapot semantics.
	PotType PotType

	// Teas lists the tea varieties this teapot can brew (RFC 7168
	// Section 2.1.1). Names are matched case-insensitively against the
	// Accept-Additions header. Ignored when PotType is PotCoffee. When
	// nil for a teapot, DefaultTeaVarieties is used.
	Teas []string

	// AvailableAdditions lists the additions (milk-type, syrup-type,
	// sweetener-type, etc., per RFC 2324 Section 2.2.2.1) the pot
	// currently has on hand. Requests asking for an addition outside
	// this set receive 406 Not Acceptable.
	AvailableAdditions []string

	// Empty signals that the pot has nothing to brew. BREW requests
	// receive 503 Service Unavailable with a Retry-After header per
	// RFC 2324 Section 2.3.3.
	Empty bool

	// RetryAfter is the value of the Retry-After header sent with 503
	// responses, in seconds. Defaults to 60.
	RetryAfter int

	// ActiveOn restricts the days on which HTCPCP semantics apply. When
	// the predicate returns false the middleware becomes a no-op and
	// forwards the request to the next handler (which typically yields
	// a 404 or 405 since BREW/WHEN are not real HTTP methods). When nil,
	// IsAprilFirst is used, matching the publication date of RFC 2324.
	// The argument is the time returned by Now.
	ActiveOn func(time.Time) bool

	// Now overrides the clock source used by ActiveOn. Defaults to
	// time.Now. Intended for tests; production code should leave it nil.
	Now func() time.Time
}

// HTCPCPMiddleware implements the Hyper Text Coffee Pot Control Protocol
// (RFC 2324) extended for tea (RFC 7168). It intercepts BREW and WHEN
// requests and responds according to the configured pot type. All other
// methods pass through to the next handler.
//
// Behavior summary:
//
//   - Teapot receiving BREW with a coffee variety: 418 I'm a Teapot
//     (RFC 7168 Section 2.3.3).
//   - Teapot receiving BREW with a supported tea variety: 200 OK with
//     Content-Type: message/teapot.
//   - Coffee pot receiving BREW with a tea variety: 406 Not Acceptable.
//   - Either pot receiving BREW while Empty is true: 503 Service
//     Unavailable + Retry-After.
//   - WHEN: 200 OK acknowledging the pour stop (RFC 2324 Section 2.1.2).
//   - BREW asking for an addition not listed in AvailableAdditions:
//     406 Not Acceptable.
func HTCPCPMiddleware(cfg HTCPCPConfig) mux.MiddlewareFunc {
	state := newHTCPCPState(cfg)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !state.activeOn(state.now()) {
				next.ServeHTTP(w, r)
				return
			}
			switch r.Method {
			case MethodBrew:
				state.handleBrew(w, r)
				return
			case MethodWhen:
				handleWhen(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IsAprilFirst reports whether the given instant falls on April 1 in
// its location. Used as the default ActiveOn predicate so HTCPCP-TEA
// (RFC 2324 published 1 April 1998, extended by RFC 7168 on 1 April
// 2014) only activates on its anniversary.
func IsAprilFirst(t time.Time) bool {
	return t.Month() == time.April && t.Day() == 1
}

// htcpcpState is the prebuilt runtime state derived from HTCPCPConfig.
// Method handlers hang off this struct so they can stay within the
// project's function-signature parameter budget.
type htcpcpState struct {
	potType    PotType
	teas       map[string]struct{}
	additions  map[string]struct{}
	empty      bool
	retryAfter int
	activeOn   func(time.Time) bool
	now        func() time.Time
}

func newHTCPCPState(cfg HTCPCPConfig) *htcpcpState {
	teas := cfg.Teas
	if cfg.PotType == PotTeapot && teas == nil {
		teas = DefaultTeaVarieties
	}
	retryAfter := cfg.RetryAfter
	if retryAfter <= 0 {
		retryAfter = 60
	}
	activeOn := cfg.ActiveOn
	if activeOn == nil {
		activeOn = IsAprilFirst
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &htcpcpState{
		potType:    cfg.PotType,
		teas:       lowerSet(teas),
		additions:  lowerSet(cfg.AvailableAdditions),
		empty:      cfg.Empty,
		retryAfter: retryAfter,
		activeOn:   activeOn,
		now:        now,
	}
}

// handleBrew dispatches a BREW request to the correct response per the
// configured pot type and request additions.
func (s *htcpcpState) handleBrew(w http.ResponseWriter, r *http.Request) {
	requested := parseAcceptAdditions(r.Header.Get("Accept-Additions"))
	teaRequest, otherAdditions := splitTeaAdditions(requested)

	if s.empty {
		w.Header().Set("Retry-After", strconv.Itoa(s.retryAfter))
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}

	if s.potType == PotTeapot {
		if teaRequest == "" {
			// Teapot asked to brew coffee (no tea variety requested).
			// RFC 7168 Section 2.3.3: "Any attempt to brew coffee with
			// a teapot should result in the error code 418 I'm a Teapot."
			respondTeapotShortAndStout(w)
			return
		}
		if _, ok := s.teas[teaRequest]; !ok {
			http.Error(w, fmt.Sprintf("tea variety %q not available", teaRequest), http.StatusNotAcceptable)
			return
		}
	} else if teaRequest != "" {
		// Coffee pot. RFC 7168 Section 2.1.1: a coffee pot is not a
		// teapot, so a tea request is not acceptable here.
		http.Error(w, "this pot brews coffee, not tea", http.StatusNotAcceptable)
		return
	}

	for _, add := range otherAdditions {
		if _, ok := s.additions[add]; !ok {
			http.Error(w, fmt.Sprintf("addition %q not available", add), http.StatusNotAcceptable)
			return
		}
	}

	contentType := ContentTypeMessageCoffeePot
	body := "Brewing coffee."
	if s.potType == PotTeapot {
		contentType = ContentTypeMessageTeapot
		body = fmt.Sprintf("Brewing %s tea.", teaRequest)
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// handleWhen acknowledges a WHEN request, stopping the addition pour per
// RFC 2324 Section 2.1.2.
func handleWhen(w http.ResponseWriter) {
	w.Header().Set("Content-Type", ContentTypeMessageCoffeePot)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("When."))
}

// respondTeapotShortAndStout writes the canonical 418 I'm a Teapot
// response. The body is the verse from RFC 2324 Section 2.3.2 because
// it would be a shame not to.
func respondTeapotShortAndStout(w http.ResponseWriter) {
	w.Header().Set("Content-Type", fmt.Sprintf("%s; charset=utf-8", mux.ContentTypeTextPlain))
	w.WriteHeader(StatusImATeapot)
	_, _ = w.Write([]byte("I'm a teapot, short and stout.\n"))
}

// parseAcceptAdditions tokenises the Accept-Additions header value into a
// slice of lowercased addition names. Empty input yields nil.
func parseAcceptAdditions(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		// Strip parameters such as ";q=0.5" if present.
		if i := strings.Index(p, ";"); i >= 0 {
			p = p[:i]
		}
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitTeaAdditions separates tea variety requests (the "tea-*" form
// from RFC 7168 Section 2.1.1) from the remaining additions. Only the
// first tea variety is honoured; subsequent ones are ignored, matching
// the prose in RFC 7168 ("the first tea variety encountered in the
// Accept-Additions header").
func splitTeaAdditions(additions []string) (string, []string) {
	var tea string
	others := make([]string, 0, len(additions))
	for _, a := range additions {
		if rest, ok := strings.CutPrefix(a, "tea-"); ok {
			if tea == "" {
				tea = rest
			}
			continue
		}
		others = append(others, a)
	}
	return tea, others
}

// lowerSet returns a set of the lowercased input values. Returns an
// empty map for a nil input so lookups stay branch-free at call sites.
func lowerSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		out[strings.ToLower(strings.TrimSpace(v))] = struct{}{}
	}
	return out
}
