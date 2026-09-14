package cursor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/google/uuid"
)

// The keys the cases sign with: the first one an installation starts with, the second one it rotates
// to. Both are long enough to be accepted and stand in for what setup writes.
var (
	firstKey  = []byte("cursor-signing-key-of-the-first-installation")
	secondKey = []byte("cursor-signing-key-of-the-second-installation")
)

const operation = "getNotifications"

var owner = uuid.MustParse("01994342-6ba7-7000-8000-000000000001")

// The moment every position below is taken from, and two positions inside one collection.
var (
	positionMoment = time.Date(2026, time.September, 12, 7, 15, 30, 123456000, time.UTC)
	middlePosition = Position{CreatedAt: positionMoment, ID: "01994342-6ba7-7000-8000-000000000010"}
	lastPosition   = Position{CreatedAt: positionMoment, ID: "01994342-6ba7-7000-8000-000000000020"}
)

func scopeOf(limit string) Scope {
	return OperationOn(operation, owner, Parameter{Name: "limit", Value: limit})
}

func mustSigner(t *testing.T, key []byte) *Signer {
	t.Helper()
	signer, err := NewSigner(key)
	if err != nil {
		t.Fatalf("a key of %d bytes was refused: %v", len(key), err)
	}
	return signer
}

// forged signs one payload with a key of its own, which is what a client trying to edit its cursor
// produces. The payload is the shape the signer writes, so only the signature can be what refuses it.
func forged(t *testing.T, key []byte, moment time.Time, id string) string {
	t.Helper()
	payload, err := json.Marshal(payload{
		Version:   version,
		CreatedAt: timestamp.Format(moment),
		ID:        id,
		Operation: operation,
		Owner:     owner.String(),
		Params:    map[string]string{"limit": "20"},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return mustSigner(t, key).signature(encoded) + encoded
}

func TestIssueAndReadRoundTrip(t *testing.T) {
	signer := mustSigner(t, firstKey)
	for _, position := range []Position{middlePosition, lastPosition} {
		issued, err := signer.Issue(position, scopeOf("20"))
		if err != nil {
			t.Fatalf("the position %s was not issued: %v", position.ID, err)
		}

		read, err := signer.Read(issued, scopeOf("20"))
		if err != nil {
			t.Fatalf("the issued cursor was refused: %v", err)
		}
		if !read.CreatedAt.Equal(positionMoment) || read.ID != position.ID {
			t.Fatalf("the cursor reads %s at %s rather than %s at %s",
				read.ID, timestamp.Format(read.CreatedAt), position.ID, timestamp.Format(positionMoment))
		}
	}
}

func TestReadRefusesMalformedTokens(t *testing.T) {
	signer := mustSigner(t, firstKey)
	issued, err := signer.Issue(middlePosition, scopeOf("20"))
	if err != nil {
		t.Fatal(err)
	}
	signature, encoded, _ := splitToken(issued)

	for name, token := range map[string]string{
		"empty":            "",
		"a plain value":    "eyJ2IjoxfQ",
		"a shorter token":  issued[:signatureLength],
		"not an alphabet":  strings.Repeat("!", signatureLength) + encoded,
		"a longer segment": signature + encoded + "=",
	} {
		if _, err := signer.Read(token, scopeOf("20")); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s was not refused as malformed: %v", name, err)
		}
	}
}

func TestReadRefusesATamperedPayloadOrSignature(t *testing.T) {
	signer := mustSigner(t, firstKey)
	issued, err := signer.Issue(middlePosition, scopeOf("20"))
	if err != nil {
		t.Fatal(err)
	}
	signature, encoded, _ := splitToken(issued)
	edited := encoded[:len(encoded)-1] + "A"

	for name, token := range map[string]string{
		// The payload is replaced by one describing another position, under a key this installation
		// did not sign with: the signature is the only thing standing between the two.
		"another position":  forged(t, secondKey, positionMoment.Add(time.Hour), lastPosition.ID),
		"another signature": flip(signature) + encoded,
		"an edited payload": signature + edited,
		"a short signature": signature[:8] + encoded,
		"a bare payload":    encoded,
	} {
		if _, err := signer.Read(token, scopeOf("20")); !errors.Is(err, ErrMalformed) && !errors.Is(err, ErrSignature) {
			t.Errorf("%s was not refused as a broken cursor: %v", name, err)
		}
	}
}

func TestReadRefusesAnotherScope(t *testing.T) {
	signer := mustSigner(t, firstKey)
	issued, err := signer.Issue(middlePosition, scopeOf("20"))
	if err != nil {
		t.Fatal(err)
	}

	another := uuid.MustParse("01994342-6ba7-7000-8000-000000000002")
	for name, expected := range map[string]Scope{
		"another operation": OperationOn("getRides", owner, Parameter{Name: "limit", Value: "20"}),
		"another account":   OperationOn(operation, another, Parameter{Name: "limit", Value: "20"}),
		"another limit":     scopeOf("50"),
		"no parameters":     OperationOn(operation, owner),
		"one more parameter": OperationOn(operation, owner,
			Parameter{Name: "limit", Value: "20"}, Parameter{Name: "kind", Value: "reservation_expiring"}),
	} {
		if _, err := signer.Read(issued, expected); !errors.Is(err, ErrScope) {
			t.Errorf("%s was not refused as another scope: %v", name, err)
		}
	}
}

func TestRotatingTheKeyInvalidatesEarlierCursors(t *testing.T) {
	issued, err := mustSigner(t, firstKey).Issue(middlePosition, scopeOf("20"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err = mustSigner(t, secondKey).Read(issued, scopeOf("20")); !errors.Is(err, ErrSignature) {
		t.Fatalf("a cursor of the previous key was accepted after the rotation: %v", err)
	}
	// The rotated key still reads what it issued itself, so a rotation withdraws the cursors of the
	// previous key rather than disabling pagination.
	rotated := mustSigner(t, secondKey)
	fresh, err := rotated.Issue(lastPosition, scopeOf("20"))
	if err != nil {
		t.Fatal(err)
	}
	read, err := rotated.Read(fresh, scopeOf("20"))
	if err != nil || read.ID != lastPosition.ID {
		t.Fatalf("the rotated key refused its own cursor: %v", err)
	}
}

func TestNewSignerRefusesAKeyTooShort(t *testing.T) {
	for _, length := range []int{0, 1, MinKeyLength - 1} {
		if _, err := NewSigner([]byte(strings.Repeat("k", length))); err == nil {
			t.Errorf("a key of %d bytes was accepted", length)
		}
	}
	// The key is copied, so a caller that reuses its buffer cannot change what was signed.
	key := []byte(strings.Repeat("k", MinKeyLength))
	signer, err := NewSigner(key)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := signer.Issue(middlePosition, scopeOf("20"))
	if err != nil {
		t.Fatal(err)
	}
	for index := range key {
		key[index] = 'x'
	}
	if _, err = signer.Read(issued, scopeOf("20")); err != nil {
		t.Fatalf("the signer followed the caller's buffer: %v", err)
	}
}

func TestIssueRefusesAnIncompleteScopeOrPosition(t *testing.T) {
	signer := mustSigner(t, firstKey)
	for name, bad := range map[string]struct {
		position Position
		scope    Scope
	}{
		"no moment":     {Position{ID: middlePosition.ID}, scopeOf("20")},
		"no identifier": {Position{CreatedAt: positionMoment}, scopeOf("20")},
		"no operation":  {middlePosition, OperationOn("", owner, Parameter{Name: "limit", Value: "20"})},
		"no account":    {middlePosition, OperationOn(operation, uuid.Nil, Parameter{Name: "limit", Value: "20"})},
		"no name":       {middlePosition, OperationOn(operation, owner, Parameter{Value: "20"})},
		"a repeated name": {middlePosition, OperationOn(operation, owner,
			Parameter{Name: "limit", Value: "20"}, Parameter{Name: "limit", Value: "50"})},
	} {
		if _, err := signer.Issue(bad.position, bad.scope); err == nil {
			t.Errorf("%s was issued", name)
		}
	}
}

func TestCursorsUseTheDeclaredAlphabet(t *testing.T) {
	signer := mustSigner(t, firstKey)
	issued, err := signer.Issue(middlePosition, scopeOf("20"))
	if err != nil {
		t.Fatal(err)
	}
	// The contract declares the cursor's alphabet, so a page link carries a value a browser and a
	// query string both accept without escaping.
	if strings.ContainsAny(issued, "+/=") {
		t.Fatalf("the cursor %q is not a URL-safe value", issued)
	}
}

// flip returns a signature different from the one it is given while keeping the alphabet, so a
// tampered signature is refused for being wrong rather than for being unreadable.
func flip(signature string) string {
	last := signature[len(signature)-1]
	if last == 'A' {
		return signature[:len(signature)-1] + "B"
	}
	return signature[:len(signature)-1] + "A"
}
