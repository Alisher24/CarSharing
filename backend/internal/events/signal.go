package events

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/google/uuid"
)

// Kind is the name of one kind of change a connected browser is told about. The spelling is the SSE
// event name the contract declares, so the word a worker writes is the word a client listens for.
type Kind string

// The changes this build announces. A public change is about a resource every visitor can read; a
// private change belongs to one account, and the stream of every other account never receives it.
const (
	VehicleChanged      Kind = "vehicle.changed"
	ZoneChanged         Kind = "zone.changed"
	TariffChanged       Kind = "tariff.changed"
	RentalChanged       Kind = "rental.changed"
	InvoiceChanged      Kind = "invoice.changed"
	NotificationChanged Kind = "notification.changed"
)

// AnnouncedKinds is every kind this build announces. The delivery table and the retention policy of
// the queue are both derived from it, so a kind cannot be added with one of them wired up nowhere.
var AnnouncedKinds = []Kind{
	VehicleChanged,
	ZoneChanged,
	TariffChanged,
	RentalChanged,
	InvoiceChanged,
	NotificationChanged,
}

// AnnouncedKindNames spells the announced kinds the way a stored task names them.
func AnnouncedKindNames() []string {
	names := make([]string, 0, len(AnnouncedKinds))
	for _, kind := range AnnouncedKinds {
		names = append(names, string(kind))
	}
	return names
}

// Channel is the PostgreSQL notification channel a worker publishes signals on and every API process
// listens to. It is not durable storage: a signal published while nothing is listening is lost, and
// the REST snapshot is what repairs a client that missed it.
const Channel = "carsharing_events"

// Signal is one committed change: which resource changed, the version it reached, and the account it
// belongs to. It carries no object and no private field, so one vocabulary serves both streams.
type Signal struct {
	Kind       Kind
	ResourceID string
	Version    int64

	// Recipient is the account this change belongs to, empty when every reader may hear about it.
	Recipient uuid.UUID
}

// Public reports whether every reader may hear about this change.
func (s Signal) Public() bool { return s.Recipient == uuid.Nil }

// published is one signal as the notification channel carries it. The recipient is absent for a
// public change rather than written as an all-zero identifier.
type published struct {
	Kind       Kind       `json:"kind"`
	ResourceID string     `json:"id"`
	Version    int64      `json:"version"`
	Recipient  *uuid.UUID `json:"recipient,omitempty"`
}

// encoded renders the signal as the payload of a notification. Only a worker publishes it, and only
// an API process reads it back, so the shape stays between the two.
func (s Signal) encoded() ([]byte, error) {
	carried := published{Kind: s.Kind, ResourceID: s.ResourceID, Version: s.Version}
	if !s.Public() {
		recipient := s.Recipient
		carried.Recipient = &recipient
	}
	return json.Marshal(carried)
}

// decodeSignal reads a published payload. A payload this build cannot read is a defect of the
// process that published it rather than a signal to deliver, so the caller reports it and keeps
// listening.
func decodeSignal(payload []byte) (Signal, error) {
	var carried published
	if err := json.Unmarshal(payload, &carried); err != nil {
		return Signal{}, err
	}
	if carried.Kind == "" || carried.ResourceID == "" {
		return Signal{}, fmt.Errorf("published signal names no %s", missingField(carried))
	}
	signal := Signal{Kind: carried.Kind, ResourceID: carried.ResourceID, Version: carried.Version}
	if carried.Recipient != nil {
		signal.Recipient = *carried.Recipient
	}
	return signal, nil
}

func missingField(carried published) string {
	if carried.Kind == "" {
		return "kind"
	}
	return "resource"
}

// change is the data of a change frame. The version is a decimal string because the contract states
// every bigint that way, so a client that parses JSON numbers cannot lose a digit of it.
type change struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// frame renders the signal as the SSE frame the contract declares: the kind names the event, and the
// data carries the identifier of the resource and the version it reached.
func (s Signal) frame() []byte {
	data, _ := json.Marshal(change{ID: s.ResourceID, Version: strconv.FormatInt(s.Version, 10)})
	return []byte("event: " + string(s.Kind) + "\ndata: " + string(data) + "\n\n")
}

// retryHintMilliseconds is how long a browser should wait before reconnecting. It is stated in the
// handshake rather than left to the browser, so a client that reconnects immediately after an API
// restart does not turn a restart into a flood.
const retryHintMilliseconds = 3000

// ready is the data of the handshake frame: the authoritative instant the subscription was
// established, which is read from the database rather than from this process.
type ready struct {
	ServerTime string `json:"server_time"`
}

// readyFrame announces an established subscription. A stream is ready once this frame has been
// written, not when the HTTP response opened.
func readyFrame(establishedAt time.Time) []byte {
	data, _ := json.Marshal(ready{ServerTime: timestamp.Format(establishedAt)})
	return []byte("retry: " + strconv.Itoa(retryHintMilliseconds) + "\nevent: ready\ndata: " +
		string(data) + "\n\n")
}

// keepaliveFrame keeps a connection alive across proxies and load balancers that close a connection
// nothing has been written to. It is a comment, so a client ignores it as an event.
var keepaliveFrame = []byte(": keepalive\n\n")
