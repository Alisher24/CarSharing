// Package auth holds the rules an account is created and proven under: what a usable email and
// password are, how a password is hashed, and how the two are checked against stored accounts.
// Nothing here knows about HTTP or about sessions, so the same rules apply to a request arriving
// over the network and to an account created by the seeder.
package auth
