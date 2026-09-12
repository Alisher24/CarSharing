package httpapi

import "crypto/subtle"

// requireSessionCSRFToken refuses a mutation made with a live session but without the token only
// this application can read. A request carrying no live session is left alone: there is nothing to
// protect, which is what keeps a repeated sign-out safe.
func requireSessionCSRFToken(b *boundaryRequest) *apiError {
	if !declaresHeader(b.route.Operation, csrfTokenHeader, false) {
		return nil
	}
	ctx := b.request.Context()
	snapshot, live, err := sessionOf(ctx).resolve(ctx)
	if err != nil {
		return &apiError{code: codeServiceUnavailable, message: messageServiceUnavailable}
	}
	if !live {
		return nil
	}
	// Constant time, so a wrong token does not reveal how much of it was right.
	presented := b.request.Header.Get(csrfTokenHeader)
	if subtle.ConstantTimeCompare([]byte(presented), []byte(snapshot.CSRFToken)) != 1 {
		return &apiError{code: codeCSRFInvalid, message: messageCSRFInvalid}
	}
	return nil
}
