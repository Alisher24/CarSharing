// Package httpheader names the protocol values shared by HTTP callers and servers.
package httpheader

const (
	RequestID     = "X-Request-ID"
	Authorization = "Authorization"
	ContentType   = "Content-Type"
	BearerPrefix  = "Bearer "
	JSON          = "application/json"
)
