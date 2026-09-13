package auth

import (
	"errors"
	"strings"
	"testing"
)

// testParameters keep the unit tests fast. The parameters the service runs with are configuration
// and are measured separately in the target environment.
var testParameters = hashingParameters{
	MemoryKiB: 8 << 10, Passes: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32,
}

// testConcurrency keeps the ceiling out of the way of the tests that are about hashing itself.
const testConcurrency = 4

func TestHashGivesEveryPasswordItsOwnSalt(t *testing.T) {
	hasher := newPasswordHasher(testParameters, testConcurrency)
	const password = "correcthorsebattery"
	first, err := hasher.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := hasher.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("the same password hashed to the same value, so the salt is not per password")
	}
	if saltOf(t, first) == saltOf(t, second) {
		t.Fatal("two hashes of one password share a salt")
	}
}

func TestVerifyAcceptsTheCorrectPasswordAndRefusesEveryOther(t *testing.T) {
	hasher := newPasswordHasher(testParameters, testConcurrency)
	const password = "correcthorsebattery"
	encoded, err := hasher.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := hasher.Verify(encoded, password)
	if err != nil || !ok {
		t.Fatalf("Verify of the correct password = %v, %v", ok, err)
	}
	for _, wrong := range []string{"correcthorsebatter", "correcthorsebatteryy", "", "Correcthorsebattery"} {
		ok, err = hasher.Verify(encoded, wrong)
		if err != nil {
			t.Fatalf("Verify(%q) failed: %v", wrong, err)
		}
		if ok {
			t.Fatalf("Verify accepted the wrong password %q", wrong)
		}
	}
}

// A stored hash carries the parameters it was produced with, so raising the configured cost later
// still leaves every existing account able to sign in.
func TestVerifyUsesTheParametersStoredWithTheHash(t *testing.T) {
	const password = "correcthorsebattery"
	encoded, err := newPasswordHasher(testParameters, testConcurrency).Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	raised := testParameters
	raised.Passes, raised.MemoryKiB = testParameters.Passes+2, testParameters.MemoryKiB*2
	ok, err := newPasswordHasher(raised, testConcurrency).Verify(encoded, password)
	if err != nil || !ok {
		t.Fatalf("Verify under raised parameters = %v, %v", ok, err)
	}
}

func TestVerifyReportsAnUnusableStoredHashAsAnError(t *testing.T) {
	hasher := newPasswordHasher(testParameters, testConcurrency)
	valid, err := hasher.Hash("correcthorsebattery")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"empty":               "",
		"not a phc string":    "correcthorsebattery",
		"wrong algorithm":     strings.Replace(valid, "argon2id", "argon2i", 1),
		"truncated":           valid[:len(valid)-10],
		"missing field":       strings.Join(strings.Split(valid, "$")[:4], "$"),
		"unparsable cost":     strings.Replace(valid, "m=", "m=x", 1),
		"unsupported version": strings.Replace(valid, "v=19", "v=16", 1),
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if ok, err := hasher.Verify(encoded, "correcthorsebattery"); err == nil || ok {
				t.Fatalf("Verify(%q) = %v, %v, want an error", encoded, ok, err)
			}
		})
	}
}

// The stored value is what an attacker reads after a database disclosure, so it must not carry the
// secret it was derived from.
func TestEncodedHashDoesNotCarryThePassword(t *testing.T) {
	const password = "correcthorsebattery"
	encoded, err := newPasswordHasher(testParameters, testConcurrency).Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encoded, password) {
		t.Fatal("the encoded hash contains the password")
	}
}

func saltOf(t *testing.T, encoded string) string {
	t.Helper()
	fields := strings.Split(encoded, "$")
	if len(fields) != 6 {
		t.Fatalf("encoded hash has %d fields, want 6: %q", len(fields), encoded)
	}
	return fields[4]
}

// The ceiling is what keeps a memory-hard function from being turned into a way of exhausting the
// instance, so it is tested as a refusal rather than as a delay.
func TestHasherAdmitsOnlyItsCeilingOfConcurrentComputations(t *testing.T) {
	const ceiling = 2
	hasher := newPasswordHasher(testParameters, ceiling)
	occupied := make(chan struct{})
	released := make(chan struct{})
	for slot := 0; slot < ceiling; slot++ {
		if !hasher.acquire() {
			t.Fatalf("slot %d of the ceiling was refused", slot)
		}
	}
	if hasher.acquire() {
		t.Fatal("a computation beyond the ceiling was admitted")
	}
	if _, err := hasher.Hash("correcthorsebattery"); !errors.Is(err, ErrHashingBusy) {
		t.Fatalf("Hash under a full ceiling = %v, want ErrHashingBusy", err)
	}
	encoded, err := newPasswordHasher(testParameters, 1).Hash("correcthorsebattery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = hasher.Verify(encoded, "correcthorsebattery"); !errors.Is(err, ErrHashingBusy) {
		t.Fatalf("Verify under a full ceiling = %v, want ErrHashingBusy", err)
	}
	// A released slot is reusable, so saturation is a passing condition rather than a latch.
	go func() { close(occupied); <-released }()
	<-occupied
	hasher.release()
	if _, err = hasher.Hash("correcthorsebattery"); err != nil {
		t.Fatalf("Hash after a slot was released failed: %v", err)
	}
	close(released)
}

// An unknown address must cost the same memory-hard work as a known one, or the sign-in form
// becomes a way of asking which addresses have accounts.
func TestAuthenticateSpendsAHashOnAnUnknownAddress(t *testing.T) {
	hasher := newPasswordHasher(testParameters, 1)
	service, err := NewService(nil, hasher)
	if err != nil {
		t.Fatal(err)
	}
	if service.standInHash == "" {
		t.Fatal("no stand-in hash was derived")
	}
	// Occupying the only slot makes the hashing visible: if the unknown address were refused
	// without hashing, this would report invalid credentials instead of a busy hasher.
	if !hasher.acquire() {
		t.Fatal("the only slot was already taken")
	}
	defer hasher.release()
	if _, err = hasher.Verify(service.standInHash, "correcthorsebattery"); !errors.Is(err, ErrHashingBusy) {
		t.Fatalf("verifying the stand-in hash = %v, want ErrHashingBusy", err)
	}
}
