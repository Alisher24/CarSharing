package auth

import (
	"strings"
	"testing"
)

// testParameters keep the unit tests fast. The parameters the service runs with are configuration
// and are measured separately in the target environment.
var testParameters = HashingParameters{
	MemoryKiB: 8 << 10, Passes: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32,
}

func TestHashGivesEveryPasswordItsOwnSalt(t *testing.T) {
	hasher := NewPasswordHasher(testParameters)
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
	hasher := NewPasswordHasher(testParameters)
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
	encoded, err := NewPasswordHasher(testParameters).Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	raised := testParameters
	raised.Passes, raised.MemoryKiB = testParameters.Passes+2, testParameters.MemoryKiB*2
	ok, err := NewPasswordHasher(raised).Verify(encoded, password)
	if err != nil || !ok {
		t.Fatalf("Verify under raised parameters = %v, %v", ok, err)
	}
}

func TestVerifyReportsAnUnusableStoredHashAsAnError(t *testing.T) {
	hasher := NewPasswordHasher(testParameters)
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
	encoded, err := NewPasswordHasher(testParameters).Hash(password)
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
