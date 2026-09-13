// Package demo declares and installs the demonstration scenario: the service zone, the tariff, a
// fleet of twenty-five vehicles showing every public state, the accounts the prepared rentals
// belong to, and the source that keeps stationary telemetry confirmed.
//
// The declaration is the single description of that scenario. Seeding creates whatever part of it
// is missing and changes nothing that is already there; restoring puts the prepared vehicles back
// and refuses outright rather than overwrite what a person did with one.
package demo
