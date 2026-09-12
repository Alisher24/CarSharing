package httpapi

import (
	"context"
	"net"
	"net/http"
)

// clientAddressKey carries the address the request came from.
type clientAddressKey struct{}

// forwardedForHeader is set by the reverse proxy in front of this service to the address of the
// peer it accepted, replacing anything the caller sent. The API is not reachable except through
// that proxy, so the value cannot be chosen by the caller it is used to limit.
const forwardedForHeader = "X-Forwarded-For"

// withClientAddress records the address a request came from, so the rate limits count the source
// rather than the proxy and the handlers stay free of transport details.
func withClientAddress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(
			context.WithValue(r.Context(), clientAddressKey{}, addressOf(r))))
	})
}

func clientAddress(ctx context.Context) string {
	address, _ := ctx.Value(clientAddressKey{}).(string)
	return address
}

// addressOf prefers what the proxy reported and falls back to the peer, so a service reached
// directly still counts attempts against something rather than against one shared empty key.
func addressOf(r *http.Request) string {
	if forwarded := r.Header.Get(forwardedForHeader); forwarded != "" {
		return forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
