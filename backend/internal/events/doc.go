// Package events carries the change signals a browser is told about. A signal is recorded in the same
// transaction as the change it describes, delivered by a worker over a PostgreSQL notification, and
// written to the streams connected browsers hold open.
//
// A signal is not a log and not a source of truth. It names a resource and the version that resource
// reached, never the object itself, and a client that missed one recovers by reading the REST
// snapshot rather than by replaying what it did not receive.
package events
