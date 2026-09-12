// Package sessions keeps server-side sessions in PostgreSQL. The browser holds only an opaque
// token; the store holds its SHA-256 hash, so reading the table yields nothing that can be
// presented as a session. Every statement goes through the transaction of the request being
// served when there is one, which is what lets a registration create its user and its session as
// a single committed unit.
package sessions
