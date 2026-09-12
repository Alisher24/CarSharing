package httpapi

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers"
)

// writeRequestError reports a failure raised by the specification router or by the generated
// request validator, which report an unroutable path and an invalid payload through one error.
func writeRequestError(spec *openapi3.T, w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, routers.ErrPathNotFound):
		writeError(w, r, codeResourceNotFound, messageResourceNotFound)
	case errors.Is(err, routers.ErrMethodNotAllowed):
		writeMethodNotAllowed(spec, w, r)
	default:
		writeValidationError(w, r, err)
	}
}

// writeMethodNotAllowed answers a path the specification declares but not for this method. A path
// whose operations were all removed from the router is not part of this API at all, so it stays a
// 404 instead of advertising methods the router will never accept.
func writeMethodNotAllowed(spec *openapi3.T, w http.ResponseWriter, r *http.Request) {
	declared := spec.Paths.Value(r.URL.Path)
	if declared == nil {
		writeError(w, r, codeMethodNotAllowed, messageMethodNotAllowed)
		return
	}
	allowed := allowedMethods(declared)
	if len(allowed) == 0 {
		writeError(w, r, codeResourceNotFound, messageResourceNotFound)
		return
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, r, codeMethodNotAllowed, messageMethodNotAllowed)
}

// allowedMethods lists a path's methods in a stable order so that the Allow header of an identical
// request never varies with map iteration.
func allowedMethods(path *openapi3.PathItem) []string {
	methods := make([]string, 0, len(path.Operations()))
	for method := range path.Operations() {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
}
