package mailstub

import (
	"errors"
	"testing"
)

// invoiced is the identifier of the invoice every key below names: a UUIDv7, which is what the
// contract requires of a stored resource.
const invoiced = "01994342-6ba7-7000-8000-000000000001"

// A key is built from the identifier of the invoice the letter is about, and it spells the shape the
// contract declares for the Delivery-Key header.
func TestAKeyIsBuiltFromTheInvoiceItNames(t *testing.T) {
	key, err := InvoiceKey(invoiced)
	if err != nil {
		t.Fatalf("the invoice was refused: %v", err)
	}
	if key.InvoiceID() != invoiced {
		t.Errorf("the key names %q", key.InvoiceID())
	}
	if spelled := key.String(); spelled != "invoice:"+invoiced+":issued" {
		t.Errorf("the key is spelled %q", spelled)
	}
	if !key.Known() {
		t.Error("a built key is not known")
	}
}

// A key is read back as the invoice it names rather than as a value whose shape happened to fit.
func TestAKeyIsReadBackAsTheInvoiceItNames(t *testing.T) {
	key, err := ParseKey("invoice:" + invoiced + ":issued")
	if err != nil {
		t.Fatalf("the declared form was refused: %v", err)
	}
	if key.InvoiceID() != invoiced {
		t.Errorf("the key names %q", key.InvoiceID())
	}
	built, err := InvoiceKey(key.InvoiceID())
	if err != nil {
		t.Fatalf("the identifier read back was refused: %v", err)
	}
	if built.String() != key.String() {
		t.Errorf("a key read and written again is %q", built.String())
	}
}

// Every value that is not the declared shape is refused: another purpose, another subject, a
// different number of segments, and an identifier that is not the canonical UUIDv7 of a resource.
func TestAKeyOfAnotherShapeIsRefused(t *testing.T) {
	for _, refused := range []struct {
		name string
		raw  string
	}{
		{name: "no version 7", raw: "invoice:11111111-1111-4111-8111-111111111111:issued"},
		{name: "not canonical", raw: "invoice:01994342-6BA7-7000-8000-000000000001:issued"},
		{name: "another subject", raw: "receipt:" + invoiced + ":issued"},
		{name: "another purpose", raw: "invoice:" + invoiced + ":refunded"},
		{name: "a missing segment", raw: "invoice:" + invoiced},
		{name: "an extra segment", raw: "invoice:" + invoiced + ":issued:again"},
		{name: "nothing at all", raw: ""},
		{name: "not a uuid", raw: "invoice:not-an-identifier:issued"},
	} {
		t.Run(refused.name, func(t *testing.T) {
			if _, err := ParseKey(refused.raw); !errors.Is(err, ErrInvalidDeliveryKey) {
				t.Errorf("%q was accepted or refused for another reason: %v", refused.raw, err)
			}
		})
	}
}

// A key that was never built names no invoice, which is what the store refuses before it reaches a
// statement.
func TestAKeyThatWasNeverBuiltIsNotKnown(t *testing.T) {
	var key Key
	if key.Known() {
		t.Error("an empty key is known")
	}
	if _, err := InvoiceKey(""); !errors.Is(err, ErrInvalidDeliveryKey) {
		t.Errorf("an empty identifier was accepted: %v", err)
	}
}

// The vocabulary declares one purpose, which is what a parsed key is judged by.
func TestOnlyTheDeclaredPurposeIsKnown(t *testing.T) {
	if !Issued.Known() {
		t.Error("the declared purpose is not known")
	}
	if Purpose("refunded").Known() {
		t.Error("a purpose this build does not declare is known")
	}
}
