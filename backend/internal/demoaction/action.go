// Package demoaction names every set-to-value change a demonstration may make. Each identifier is the
// word the contract publishes, and it is declared once here because four readers name the same words:
// the module that applies a change, the mail stub that arms one, the terminal that states one and the
// surface that reads one.
package demoaction

// Kind is one set-to-value change a demonstration may make.
type Kind string

const (
	// SetTelemetryState links or unlinks a vehicle, which is what shows the difference between a
	// vehicle that reports and one that keeps the reading it last confirmed.
	SetTelemetryState Kind = "set_telemetry_state"

	// SetPosition puts a vehicle somewhere by hand. It is refused while the vehicle is booked or
	// moving, and the model drives it back to its route from wherever it was put.
	SetPosition Kind = "set_position"

	// SetEnergyRemaining states what one source of a vehicle holds now, which is how a demonstration
	// prepares a ride that runs out within minutes.
	SetEnergyRemaining Kind = "set_energy_remaining"

	// MarkServiced returns a vehicle to its service point, fills every source and clears the flag
	// that took it out of service.
	MarkServiced Kind = "mark_serviced"

	// SetNextPaymentOutcome records what the next attempt at the payment of one ride will decide.
	SetNextPaymentOutcome Kind = "set_next_payment_outcome"

	// DropNextResponseAfterAccept asks the mail stub to store the next letter it is given and lose
	// the answer to it, which is the state a retry is shown from.
	DropNextResponseAfterAccept Kind = "drop_next_response_after_accept"
)
