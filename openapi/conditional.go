package openapi

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// computeETag returns a strong ETag from the FNV-128a hash of data.
func computeETag(data []byte) string {
	h := fnv.New128a()
	_, _ = h.Write(data)
	return fmt.Sprintf(`"%x"`, h.Sum(nil))
}

// etagMatch checks whether the If-None-Match header value contains
// the server ETag. Supports comma-separated lists and "*".
func etagMatch(clientHeader, serverETag string) bool {
	if clientHeader == "" {
		return false
	}
	if clientHeader == "*" {
		return true
	}
	for val := range strings.SplitSeq(clientHeader, ",") {
		if strings.TrimSpace(val) == serverETag {
			return true
		}
	}
	return false
}
