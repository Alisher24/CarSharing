// Package outbox is the durable queue of work that must survive the process that created it: a task
// is written in the same transaction as the change it describes, claimed by one worker at a time
// under a lease, delivered outside any transaction, and kept with its error until it succeeds.
//
// The queue owns the lease, the retries and the retention mechanics. What a task means, and how it
// is delivered, belong to the feature that records it.
package outbox
