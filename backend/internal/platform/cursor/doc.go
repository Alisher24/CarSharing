// Package cursor issues and reads the signed position a paginated operation hands back for its next
// page. A cursor is a keyset position over one operation, one owner and one set of query parameters,
// signed with HMAC-SHA256, so a position issued for one collection cannot be replayed against
// another, against another account, or against the same collection with different parameters.
//
// A cursor is forward only and carries no deadline: it names where the next page starts rather than
// until when it may be used. Rotating the signing key therefore invalidates every cursor issued
// under the previous one, which is the intended way to withdraw a page somebody kept.
//
// The package reports a typed failure and knows nothing about HTTP; turning that failure into the
// status the contract declares belongs to the transport that serves the operation.
package cursor
