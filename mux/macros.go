package mux

import (
	"fmt"
	"regexp"
)

// varMatcher validates a single route variable value.
// *regexp.Regexp satisfies this interface.
type varMatcher interface {
	MatchString(string) bool
	String() string
}

// lengthMatcher wraps a regexp with an additional maximum length constraint.
type lengthMatcher struct {
	re     *regexp.Regexp
	maxLen int
}

func (m *lengthMatcher) MatchString(s string) bool {
	return len(s) <= m.maxLen && m.re.MatchString(s)
}

func (m *lengthMatcher) String() string {
	return m.re.String()
}

// macro holds a pattern string and its pre-compiled validation matcher.
type macro struct {
	pattern string
	matcher varMatcher
}

// patternMacros maps macro names to their compiled patterns.
// Used in route variable definitions: {name:macro}.
var patternMacros = func() map[string]macro {
	raw := map[string]string{
		"uuid":     `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`,
		"int":      `[0-9]+`,
		"float":    `[0-9]*\.?[0-9]+`,
		"slug":     `[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*`,
		"alpha":    `[a-zA-Z]+`,
		"alphanum": `[a-zA-Z0-9]+`,
		"date":     `[0-9]{4}-[0-9]{2}-[0-9]{2}`,
		"hex":      `[0-9a-fA-F]+`,
		// RFC 1035/1123: labels 1-63 chars, total up to 253 chars.
		"domain": `(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?`,
	}

	// Macros that require additional length validation beyond regex.
	maxLengths := map[string]int{
		"domain": 253,
	}

	m := make(map[string]macro, len(raw))
	for name, pattern := range raw {
		re := regexp.MustCompile(fmt.Sprintf("^%s$", pattern))

		var matcher varMatcher
		if maxLen, ok := maxLengths[name]; ok {
			matcher = &lengthMatcher{re: re, maxLen: maxLen}
		} else {
			matcher = re
		}

		m[name] = macro{
			pattern: pattern,
			matcher: matcher,
		}
	}

	return m
}()

// expandMacro returns the regex pattern string and a pre-compiled
// validation matcher for a macro name. If the name is not a known macro,
// it returns the input unchanged with a nil matcher (caller must compile).
func expandMacro(pattern string) (string, varMatcher) {
	if m, ok := patternMacros[pattern]; ok {
		return m.pattern, m.matcher
	}

	return pattern, nil
}
