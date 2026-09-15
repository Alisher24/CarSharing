package mailstub

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// The vocabulary of a delivery key: the subject a letter is about and the purpose of the delivery.
// One task produces one letter about one invoice, so a key states both parts of that fact rather
// than only being recognizable as a shape.
const (
	subjectInvoice = "invoice"
	keySeparator   = ":"

	// keySegments is how many parts a key is written in: the subject, its identifier and the
	// purpose. It is stated rather than derived from the spelling above, because the number of parts
	// is what says whether a value carries a segment this vocabulary does not declare.
	keySegments = 3
)

// ErrInvalidDeliveryKey reports a key that is not one this build writes: a value of another shape, a
// subject or a purpose this vocabulary does not declare, or an identifier that is not the canonical
// UUIDv7 the contract requires of a stored resource.
var ErrInvalidDeliveryKey = errors.New("the delivery key is not one this build writes")

// Purpose is what a key says is being delivered about its subject.
type Purpose string

// Issued is the letter about an invoice that was issued. It is the only purpose this build declares:
// a second letter about one invoice would be a second word here rather than a second key shape.
const Issued Purpose = "issued"

// Known reports whether a purpose is one this build declares, which is what a key is judged by
// before the delivery it names is performed.
func (p Purpose) Known() bool { return p == Issued }

// Key is the delivery key of one letter: the invoice the letter is about, and the fact that what is
// delivered is the letter about its issuance.
//
// It is built or parsed rather than assembled, so a key that reached the stub always names an
// invoice: the sender reads the invoice and builds the key from it, which is what makes the key and
// the letter describe one invoice instead of two.
type Key struct{ invoiceID string }

// InvoiceKey builds the key of the letter about one invoice.
func InvoiceKey(invoiceID string) (Key, error) {
	if err := canonicalResourceID(invoiceID); err != nil {
		return Key{}, fmt.Errorf("%w: %w", ErrInvalidDeliveryKey, err)
	}
	return Key{invoiceID: invoiceID}, nil
}

// ParseKey reads a delivery key and reports the invoice it names.
func ParseKey(raw string) (Key, error) {
	parts := strings.Split(raw, keySeparator)
	if len(parts) != keySegments {
		return Key{}, fmt.Errorf("%w: %q", ErrInvalidDeliveryKey, raw)
	}
	if parts[0] != subjectInvoice {
		return Key{}, fmt.Errorf("%w: %q names no subject this build delivers", ErrInvalidDeliveryKey, raw)
	}
	if !Purpose(parts[2]).Known() {
		return Key{}, fmt.Errorf("%w: %q states no purpose this build delivers", ErrInvalidDeliveryKey, raw)
	}
	key, err := InvoiceKey(parts[1])
	if err != nil {
		return Key{}, fmt.Errorf("%w: %q", ErrInvalidDeliveryKey, raw)
	}
	return key, nil
}

// String spells the key the way the contract's Delivery-Key header declares it.
func (k Key) String() string {
	return subjectInvoice + keySeparator + k.invoiceID + keySeparator + string(Issued)
}

// InvoiceID is the invoice the key names, which is the invoice the letter is about.
func (k Key) InvoiceID() string { return k.invoiceID }

// Known reports whether a key was built or parsed. A key that was not is refused before it reaches
// storage, where it would name no invoice at all.
func (k Key) Known() bool { return k.invoiceID != "" }

// canonicalResourceID reports why a value is not the resource identifier the contract declares: the
// contract's ResourceId is a UUIDv7 in its canonical spelling, so a value only this build can
// recognize is refused rather than normalized into one.
func canonicalResourceID(value string) error {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return fmt.Errorf("%q is not a UUID", value)
	}
	if parsed.String() != value {
		return fmt.Errorf("%q is not the canonical spelling of a UUID", value)
	}
	if parsed.Version() != 7 {
		return fmt.Errorf("%q is not a version 7 UUID", value)
	}
	if parsed.Variant() != uuid.RFC4122 {
		return fmt.Errorf("%q is not an RFC 4122 UUID", value)
	}
	return nil
}
