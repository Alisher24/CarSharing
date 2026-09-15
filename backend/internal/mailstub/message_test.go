package mailstub

import (
	"strings"
	"testing"
	"time"
)

// sent is the letter every check below delivers, so a check that refuses a request is about the field
// it changed rather than about one it left out.
func sent() Request {
	return Request{
		To:      "rider@example.test",
		Subject: "Поездка завершена",
		Text:    "Итог: 12,34 сома",
	}
}

func TestAWellFormedLetterIsAccepted(t *testing.T) {
	if err := sent().Validate(); err != nil {
		t.Fatalf("the letter was refused: %v", err)
	}
}

// A request no row would accept is refused before anything is written: no address, an address longer
// than the contract allows, no subject, and a text larger than the body the contract caps.
func TestARequestThatCannotBeALetterIsRefused(t *testing.T) {
	for _, refused := range []struct {
		name   string
		change func(*Request)
	}{
		{name: "no address", change: func(r *Request) { r.To = "" }},
		{name: "an address that is too long", change: func(r *Request) {
			r.To = strings.Repeat("a", MaxRecipientLength) + "@example.test"
		}},
		{name: "no subject", change: func(r *Request) { r.Subject = "" }},
		{name: "a text larger than the body limit", change: func(r *Request) {
			r.Text = strings.Repeat("a", MaxBodyBytes+1)
		}},
	} {
		t.Run(refused.name, func(t *testing.T) {
			request := sent()
			refused.change(&request)
			if err := request.Validate(); err == nil {
				t.Error("the request was accepted")
			}
		})
	}
}

// The longest address the contract accepts is accepted: the limit is a bound rather than a hint.
func TestTheLongestAddressTheContractAcceptsIsAccepted(t *testing.T) {
	request := sent()
	request.To = strings.Repeat("a", MaxRecipientLength)
	if err := request.Validate(); err != nil {
		t.Fatalf("the longest address was refused: %v", err)
	}
}

// Which fields make two letters the same is the rule a repeated delivery is judged by, so a letter
// that differs in any of the three is not the letter the key already holds.
func TestALetterDiffersFromAnotherInEachOfItsThreeFields(t *testing.T) {
	stored := Message{
		ID:          "01994342-6ba7-7000-8000-000000000001",
		DeliveryKey: "invoice:01994342-6ba7-7000-8000-000000000002:issued",
		To:          "rider@example.test",
		Subject:     "Поездка завершена",
		Text:        "Итог: 12,34 сома",
		AcceptedAt:  time.Date(2026, time.September, 15, 8, 32, 11, 123456000, time.UTC),
	}
	request := Request{To: stored.To, Subject: stored.Subject, Text: stored.Text}
	if !stored.states(request) {
		t.Error("the letter the key holds is not the letter that was delivered")
	}
	for _, changed := range []struct {
		name   string
		change func(*Request)
	}{
		{name: "another address", change: func(r *Request) { r.To = "somebody@example.test" }},
		{name: "another subject", change: func(r *Request) { r.Subject = "Счёт" }},
		{name: "another text", change: func(r *Request) { r.Text = "Итог: 12,35 сома" }},
	} {
		t.Run(changed.name, func(t *testing.T) {
			other := request
			changed.change(&other)
			if stored.states(other) {
				t.Error("another letter is answered as the stored one")
			}
		})
	}
}
