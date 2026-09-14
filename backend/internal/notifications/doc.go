// Package notifications is what the service has to tell one account and the account has not read
// yet: one durable record per rental and kind, its history and the moment it was read.
//
// The record is what makes a message happen once. Its unique key names the rental and the kind, so a
// second worker, a repeated pass and a read that arrives after a sweep all write nothing rather than
// producing a second notification, and nothing in the domain keeps a flag of its own that could
// disagree with the rows.
package notifications
