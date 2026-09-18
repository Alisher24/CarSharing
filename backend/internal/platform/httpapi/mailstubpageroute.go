package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// The pages of the human inbox. A page answers the first two; a path the contract owns goes to the
// boundary, which knows those paths and answers them as the JSON operations they are.
const (
	inboxPath        = "/"
	inboxMessagePath = "/messages/{id}"
)

// inboxRoute is one page of this surface: the pattern it answers, what it reads to draw itself, and
// whether it shows one letter rather than the list. The table of routes is the single declaration of
// what the surface serves, so another page is one entry beside these rather than an edit to a growing
// conditional.
type inboxRoute struct {
	path   string
	draw   func(pageRequest) inboxAnswer
	letter bool
}

// pageRequest is one in-flight page request: the context it is read under, the query its address
// carried, and what the pattern of its page captured from the address.
type pageRequest struct {
	ctx        context.Context
	query      url.Values
	identifier string
}

// pageRoute is the page one request path was matched to: its position in the table of routes, and
// what its pattern captured from the address.
type pageRoute struct {
	index    int
	captured map[string]string
}

// inboxRouteMatch matches one request path against one pattern of this surface and answers what the
// pattern captured. A literal segment is compared as it is written and `{name}` captures one whole
// segment, so the two must have as many segments as each other: `/messages/one/two` fits nothing and
// is the unknown page it is, rather than the page of a letter named `one/two`.
func inboxRouteMatch(pattern, path string) (map[string]string, bool) {
	parts, captures := inboxPatternParts(pattern)
	segments := strings.Split(path, "/")
	if len(segments) != len(parts) {
		return nil, false
	}
	values := make(map[string]string, len(captures))
	for index, part := range parts {
		switch {
		case isInboxCapture(part):
			if segments[index] == "" {
				return nil, false
			}
			values[inboxCaptureName(part)] = segments[index]
		case segments[index] != part:
			return nil, false
		}
	}
	return values, true
}

// inboxPatternParts splits one pattern into the segments it is matched segment by segment, and names
// the segments that capture. A pattern that is not a sequence of literals and `{name}` captures is a
// defect of this build, so it stops the process where the table is declared.
func inboxPatternParts(pattern string) ([]string, []string) {
	parts := strings.Split(pattern, "/")
	captures := make([]string, 0, len(parts))
	for _, part := range parts {
		if !isInboxCapture(part) {
			if strings.ContainsAny(part, "{}") {
				panic("the inbox page pattern " + pattern + " is neither a literal nor a capture")
			}
			continue
		}
		name := inboxCaptureName(part)
		if name == "" {
			panic("the inbox page pattern " + pattern + " names no capture")
		}
		captures = append(captures, name)
	}
	return parts, captures
}

// isInboxCapture reports whether one segment of a pattern captures rather than matching a literal. A
// segment that only opens or only closes a brace is neither, and inboxPatternParts refuses it.
func isInboxCapture(part string) bool {
	return strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}")
}

// inboxCaptureName is the name one capturing segment gives what it captures.
func inboxCaptureName(part string) string {
	return strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
}

// inboxIdentifierCapture is the name the one-letter pattern gives the identifier it captures.
const inboxIdentifierCapture = "id"

// pageRequestOf reads one page request: the context it is answered in, the query its address carried,
// and what the pattern of the page captured from the address.
func pageRequestOf(r *http.Request, route *pageRoute) pageRequest {
	request := pageRequest{ctx: r.Context(), query: r.URL.Query()}
	if route == nil {
		return request
	}
	request.identifier = route.captured[inboxIdentifierCapture]
	return request
}
