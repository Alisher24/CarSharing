// Package completion is the vocabulary of why a ride ended. It sits beside the stage of a rental and
// the modes of a ride, below every module that writes that fact — the rentals module that ends a ride,
// the invoices module that records what the ride cost and the notifications module that reports it —
// so all of them name one declaration rather than spelling the reason again.
package completion

// Reason is why a rental stopped being a ride. The spelling is the one the contract publishes as the
// completion of a finished rental and of an invoice, so the word a stored row carries is the word a
// client reads.
type Reason string

const (
	// UserFinished is a ride the person who was riding it ended.
	UserFinished Reason = "user_finished"

	// EnergyDepleted is a ride the service ended because no source fit to drive it was left. No
	// transition of this build writes it yet; the value is declared here because it is one of the two
	// reasons the contract publishes and the storage admits.
	EnergyDepleted Reason = "energy_depleted"
)

// Known reports whether a reason is one this vocabulary declares, which is what a row read from
// storage is judged by before it is published under a shape that states one.
func (r Reason) Known() bool {
	return r == UserFinished || r == EnergyDepleted
}
