// Package httpapi serves the application over HTTP behind one transport boundary. Every request is
// identified, authenticated and validated against the OpenAPI contract before it reaches a handler,
// and every failure leaves as the single JSON error envelope all four specifications declare, so a
// handler below the boundary only ever sees a request the contract has already accepted.
package httpapi
