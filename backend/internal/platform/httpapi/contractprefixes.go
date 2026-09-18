package httpapi

import (
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// contractPrefixes are the path prefixes a specification owns: a namespace every path of the
// specification belongs to, and the longest literal beginning of a path that falls outside those
// namespaces. They answer where the contract's own namespace ends, which is what a surface serving
// something beside the contract has to know — a request inside it belongs to the boundary, which
// answers it as the JSON operation or the JSON refusal the contract declares, and a request outside it
// was never the contract's.
//
// The prefixes are derived from the paths themselves rather than written down beside them, so a path
// added to the specification extends its own namespace and nothing has to be kept in step by hand.
type contractPrefixes []string

// contractNamespaceSegments is how many segments of a path name the namespace it belongs to. A
// versioned API states its version in the second segment, so everything under `/api/v1/` is the
// contract's even when the path beneath it is one the contract does not declare.
const contractNamespaceSegments = 2

// newContractPrefixes reads the prefixes one specification declares.
func newContractPrefixes(spec *openapi3.T) contractPrefixes {
	owned := make(map[string]bool, len(spec.Paths.Map())*2)
	for path := range spec.Paths.Map() {
		owned[contractNamespace(path)] = true
		if prefix := literalPathPrefix(path); prefix != "" {
			owned[prefix] = true
		}
	}
	prefixes := make(contractPrefixes, 0, len(owned))
	for prefix := range owned {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	return prefixes
}

// owns reports whether a request path belongs to the contract rather than to the surface beside it.
// A namespace is a prefix that ends at a segment boundary, so anything under it belongs to the
// contract. Any other prefix is the literal beginning of a declared path, and it answers for that
// path and for what continues it rather than for a path that merely starts with the same letters.
func (p contractPrefixes) owns(path string) bool {
	for _, prefix := range p {
		if strings.HasSuffix(prefix, "/") {
			if strings.HasPrefix(path, prefix) {
				return true
			}
			continue
		}
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// contractNamespace is the namespace one contract path belongs to, as a prefix ending at a segment
// boundary. A path too short to name a namespace has none, and answers with the empty string.
func contractNamespace(path string) string {
	segments := strings.Split(path, "/")
	if len(segments) <= contractNamespaceSegments {
		return ""
	}
	return strings.Join(segments[:contractNamespaceSegments], "/") + "/"
}

// literalPathPrefix is the longest literal beginning of one contract path: the segments before the
// first one that names a parameter. A path that is nothing but parameters has no literal beginning,
// and answers with the empty string.
func literalPathPrefix(path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if strings.ContainsAny(segment, "{}") {
			segments = segments[:index]
			break
		}
	}
	prefix := strings.Join(segments, "/")
	if prefix == "" || prefix == "/" {
		return ""
	}
	return prefix
}
