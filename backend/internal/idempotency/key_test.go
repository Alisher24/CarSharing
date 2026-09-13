package idempotency

import "testing"

// The fingerprint is what tells a repeat of one command from the same key applied to another. A
// command that asks for something else must produce another fingerprint, and two attempts that ask
// for the same thing must produce the same one however they differ in whitespace.
func TestFingerprintSeparatesCommands(t *testing.T) {
	body := []byte(`{"vehicle_id":"01994342-6ba7-7000-8000-000000000001"}`)
	asked := FingerprintOf("POST", "/api/v1/reservations", body)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"another method", "PUT", "/api/v1/reservations", body},
		{"another resource", "POST", "/api/v1/reservations/01994342-6ba7-7000-8000-000000000002/cancel", body},
		{"another body", "POST", "/api/v1/reservations", []byte(`{"vehicle_id":"01994342-6ba7-7000-8000-000000000002"}`)},
		{"no body", "POST", "/api/v1/reservations", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if FingerprintOf(tc.method, tc.path, tc.body) == asked {
				t.Fatal("a different command fingerprints the same way")
			}
		})
	}

	if FingerprintOf("POST", "/api/v1/reservations", body) != asked {
		t.Fatal("the same command fingerprints differently on a repeat")
	}
}

// A path and a body are separated in the hashed text, so moving a character from the end of one to
// the beginning of the other does not describe the same command twice.
func TestFingerprintSeparatesThePartsOfACommand(t *testing.T) {
	first := FingerprintOf("POST", "/api/v1/reservations", []byte("a"))
	second := FingerprintOf("POST", "/api/v1/reservationsa", []byte{})
	if first == second {
		t.Fatal("a command without a body fingerprints as another command with one")
	}
}
