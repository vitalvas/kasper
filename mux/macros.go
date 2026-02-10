package mux

import (
	"fmt"
	"regexp"
)

// macro holds a pattern string and its pre-compiled validation regexp.
type macro struct {
	pattern string
	regexp  *regexp.Regexp
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
	}

	m := make(map[string]macro, len(raw))
	for name, pattern := range raw {
		m[name] = macro{
			pattern: pattern,
			regexp:  regexp.MustCompile(fmt.Sprintf("^%s$", pattern)),
		}
	}

	return m
}()

// expandMacro returns the regex pattern string and a pre-compiled
// validation regexp for a macro name. If the name is not a known macro,
// it returns the input unchanged with a nil regexp (caller must compile).
func expandMacro(pattern string) (string, *regexp.Regexp) {
	if m, ok := patternMacros[pattern]; ok {
		return m.pattern, m.regexp
	}

	return pattern, nil
}
