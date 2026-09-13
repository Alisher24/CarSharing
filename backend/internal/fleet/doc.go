// Package fleet is the read model of the vehicles the public catalog publishes: what each vehicle
// carries, how much of it is left, how recently the vehicle confirmed where it is, and the state a
// reader is shown. The state is derived on every read from the rental that holds the vehicle and
// from the vehicle's own fitness, so no second copy of it can disagree with the rentals module.
package fleet
