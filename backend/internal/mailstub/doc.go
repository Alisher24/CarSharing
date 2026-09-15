// Package mailstub is the mail stub's own module: the letter it accepts, the key one delivery is
// written under, and the rules that make a repeated delivery a repeat rather than a second letter.
//
// It holds no transport and no configuration. What it stores it states, what it states it never
// changes, and every statement runs on the querier the context carries so that a letter and the
// demonstration demand that decided its fate are written by one transaction or by none.
package mailstub
