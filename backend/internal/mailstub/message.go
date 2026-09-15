package mailstub

import (
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

// The limits the contract declares for one letter. They are restated here so that a message built
// without passing through the HTTP boundary is judged by the same rules the boundary applies.
const (
	// MaxRecipientLength is the longest address the contract accepts.
	MaxRecipientLength = 254

	// MaxBodyBytes is the cap the contract declares for the whole request of a delivery, which no
	// letter can exceed: the three fields travel inside one body of that size.
	MaxBodyBytes = 262144
)

// Request is one letter as the delivery operation carries it: the three fields the contract
// declares, and nothing else. The stub decides nothing about what a letter says, so there is no
// fourth field through which a caller could ask it for something.
type Request struct {
	To      string
	Subject string
	Text    string
}

// Validate reports why a request cannot be stored, or nil when it can. It guards the statement below
// from a value no row would accept; the table states the same rules again, so a request that passed
// here and was refused there is a defect rather than the only check there was.
func (r Request) Validate() error {
	switch {
	case r.To == "":
		return errors.New("a letter must name the address it is sent to")
	case utf8.RuneCountInString(r.To) > MaxRecipientLength:
		return fmt.Errorf("the address of a letter cannot be longer than %d characters", MaxRecipientLength)
	case r.Subject == "":
		return errors.New("a letter must state what it is about")
	case len(r.Text) > MaxBodyBytes:
		return fmt.Errorf("the text of a letter cannot be longer than %d bytes", MaxBodyBytes)
	}
	return nil
}

// Message is one letter as the stub stores it: what it says, the key it was delivered under, and the
// moment it was accepted.
//
// The moment is the moment of the first delivery, not of the delivery being answered: a repeat
// answers this value as it stands, so the box never shows one letter accepted twice at two moments.
type Message struct {
	ID          string
	DeliveryKey string
	To          string
	Subject     string
	Text        string
	AcceptedAt  time.Time
}

// states reports whether a message holds exactly the letter a request asks for. Which fields make two
// letters the same is decided here rather than by the caller, because it is the rule a repeated
// delivery is judged by: the same key with anything else to say is a conflict, not a repeat.
func (m Message) states(request Request) bool {
	return m.To == request.To && m.Subject == request.Subject && m.Text == request.Text
}
