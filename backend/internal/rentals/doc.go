// Package rentals owns the one rental a person and a vehicle can be in at a time. A reservation
// and the ride it becomes are stages of that rental rather than separate records, so a single
// database constraint refuses a second live rental and a single module performs every transition
// between stages. Nothing outside this package writes a stage.
package rentals
