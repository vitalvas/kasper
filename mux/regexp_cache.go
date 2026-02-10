package mux

import (
	"regexp"
	"sync"
)

// regexpCache caches compiled regular expressions by pattern string.
// The number of unique patterns is bounded by the number of registered
// routes, so the cache grows to a fixed size and stays there.
var regexpCache sync.Map

// compileRegexp returns a cached *regexp.Regexp for the given pattern,
// compiling and caching it on first use.
func compileRegexp(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexpCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	actual, _ := regexpCache.LoadOrStore(pattern, re)

	return actual.(*regexp.Regexp), nil
}
