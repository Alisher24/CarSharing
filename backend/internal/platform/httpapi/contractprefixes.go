package httpapi

import (
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// contractPrefixes are the path prefixes a specification owns: the longest literal beginning of every
// path it declares, up to the first parameter of that path. They answer where the contract's own
// namespace ends, which is what a surface serving something beside the contract has to know — a
// request inside it belongs to the boundary, and a request outside it was never the contract's.
//
// A prefix is derived from the paths themselves rather than written down beside them, so a path added
// to the specification extends its own namespace and nothing has to be kept in step by hand.
type contractPrefixes []string

// newContractPrefixes reads the prefixes one specification declares.
func newContractPrefixes(spec *openapi3.T) contractPrefixes {
	prefixes := make(contractPrefixes, 0, len(spec.Paths.Map()))
	for path := range spec.Paths.Map() {
		if prefix := literalPathPrefix(path); prefix != "" {
			prefixes = append(prefixes, prefix)
		}
	}
	sort.Strings(prefixes)
	return prefixes
}

// owns reports whether a request path belongs to the contract rather than to the surface beside it.
func (p contractPrefixes) owns(path string) bool {
	for _, prefix := range p {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// literalPathPrefix is the longest literal beginning of one contract path: the segments before the
// first one that names a parameter. The root path has no literal beginning, and a request under it
// belongs to nothing this can decide, so it answers none.
func literalPathPrefix(path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if strings.ContainsAny(segment, "{}") {
			segments = segments[:index]
			break
		}
	}
	if len(segments) == 0 {
		return ""
	}
	return strings.Join(segments, "/")
}
