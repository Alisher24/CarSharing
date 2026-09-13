package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/google/uuid"
)

const testVehicle = "01994342-6ba7-7000-8000-000000000001"

// A change frame carries the resource and the version it reached, never the object: what the client
// loads in full is the REST snapshot, and the frame only tells it that the snapshot is old.
func TestAChangeFrameCarriesOnlyTheIdentifierAndTheVersion(t *testing.T) {
	signal := Signal{Kind: VehicleChanged, ResourceID: testVehicle, Version: 42}
	want := "event: vehicle.changed\n" +
		`data: {"id":"` + testVehicle + `","version":"42"}` + "\n\n"
	if got := string(signal.frame()); got != want {
		t.Fatalf("frame = %q, want %q", got, want)
	}
}

// The handshake states the retry hint and the authoritative instant, so a browser reconnects on the
// schedule this server asked for rather than one it invented.
func TestTheHandshakeStatesTheRetryHintAndTheServerTime(t *testing.T) {
	established := time.Date(2026, time.September, 12, 7, 15, 30, 123456000, time.UTC)
	frame := string(readyFrame(established))
	if !strings.HasPrefix(frame, "retry: 3000\nevent: ready\ndata: ") {
		t.Fatalf("handshake = %q", frame)
	}
	var stated struct {
		ServerTime string `json:"server_time"`
	}
	if err := json.Unmarshal([]byte(frame[strings.Index(frame, "{"):strings.Index(frame, "}")+1]), &stated); err != nil {
		t.Fatal(err)
	}
	if stated.ServerTime != timestamp.Format(established) {
		t.Fatalf("handshake states %s, want %s", stated.ServerTime, timestamp.Format(established))
	}
}

// A signal crosses between two processes as a notification payload, so what one encodes the other
// has to read back exactly, recipient included.
func TestAPublishedSignalIsReadBackUnchanged(t *testing.T) {
	owner := uuid.New()
	for _, signal := range []Signal{
		{Kind: VehicleChanged, ResourceID: testVehicle, Version: 7},
		{Kind: RentalChanged, ResourceID: testVehicle, Version: 8, Recipient: owner},
	} {
		payload, err := signal.encoded()
		if err != nil {
			t.Fatal(err)
		}
		read, err := decodeSignal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if read != signal {
			t.Fatalf("read %+v, wrote %+v", read, signal)
		}
	}
}

// The recipient is absent from a public change rather than written as an all-zero identifier, so a
// reader cannot mistake one for the other.
func TestAPublicSignalNamesNoRecipient(t *testing.T) {
	payload, err := (Signal{Kind: ZoneChanged, ResourceID: testVehicle, Version: 1}).encoded()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "recipient") {
		t.Fatalf("a public signal named a recipient: %s", payload)
	}
}

func TestASignalThatNamesNothingIsRefused(t *testing.T) {
	for _, payload := range []string{
		`{"id":"` + testVehicle + `"}`,
		`{"kind":"vehicle.changed"}`,
		`not json`,
	} {
		if _, err := decodeSignal([]byte(payload)); err == nil {
			t.Errorf("%s was accepted", payload)
		}
	}
}

// A stream hears the changes of its own audience and nothing else: the public stream never carries a
// private change and a private stream never carries another account's.
func TestAStreamHearsOnlyItsOwnAudience(t *testing.T) {
	owner, other := uuid.New(), uuid.New()
	public := Signal{Kind: VehicleChanged, ResourceID: testVehicle, Version: 1}
	own := Signal{Kind: RentalChanged, ResourceID: testVehicle, Version: 2, Recipient: owner}
	for _, heard := range []struct {
		name     string
		audience Audience
		signal   Signal
		wanted   bool
	}{
		{"public hears a public change", Public(), public, true},
		{"public ignores a private change", Public(), own, false},
		{"owner hears an own change", Private(owner), own, true},
		{"owner ignores a public change", Private(owner), public, false},
		{"another account ignores it", Private(other), own, false},
	} {
		t.Run(heard.name, func(t *testing.T) {
			if got := heard.audience.hears(heard.signal); got != heard.wanted {
				t.Fatalf("hears = %t, want %t", got, heard.wanted)
			}
		})
	}
}

// Every announced kind is one the delivery can route and the retention can recognize, so the two
// cannot drift apart.
func TestEveryAnnouncedKindIsNamedOnce(t *testing.T) {
	names := AnnouncedKindNames()
	if len(names) != len(AnnouncedKinds) {
		t.Fatalf("%d kinds have %d names", len(AnnouncedKinds), len(names))
	}
	seen := map[string]bool{}
	for index, kind := range AnnouncedKinds {
		if kind == "" {
			t.Fatalf("kind %d is empty", index)
		}
		if seen[names[index]] {
			t.Fatalf("%s is announced twice", kind)
		}
		seen[names[index]] = true
		if names[index] != string(kind) {
			t.Fatalf("%s is named %s", kind, names[index])
		}
	}
}
