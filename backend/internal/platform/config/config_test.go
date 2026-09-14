package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The failure names a length no constant can be interpolated into, so the two are held together
// here instead: a person who reads the message must be reading the rule that was applied.
func TestPasswordLengthFailureStatesTheEnforcedLength(t *testing.T) {
	if !strings.Contains(minPasswordLengthMessage, strconv.Itoa(minPasswordLength)) {
		t.Fatalf("%q does not state the enforced length %d", minPasswordLengthMessage, minPasswordLength)
	}
}

func TestSecretFileAndPortValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password")
	secret := strings.Repeat("x", 48)
	if err := os.WriteFile(path, []byte(secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_PASSWORD_FILE", path)
	t.Setenv("DB_PORT", "5432")
	c, err := Load()
	if err != nil || c.DBPassword != secret {
		t.Fatal("secret file was not loaded")
	}
	for _, port := range []string{"0", "65536", "invalid"} {
		t.Setenv("DB_PORT", port)
		if _, err := Load(); err == nil {
			t.Fatalf("accepted invalid port %q", port)
		}
	}
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_PASSWORD_FILE", "")
	if _, err := Load(); err == nil {
		t.Fatal("accepted missing secret file")
	}
}

// The cursor key is read from its own file rather than from the environment, and the loader hands
// the bytes over unchanged: an installation is given a key or it is given none.
func TestCursorSigningKey(t *testing.T) {
	key := strings.Repeat("k", 48)
	path := filepath.Join(t.TempDir(), "cursor_hmac_key")
	if err := os.WriteFile(path, []byte(key+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(CursorHMACKeyFileVariable, "")
	absent, err := CursorSigningKey()
	if err != nil || len(absent) != 0 {
		t.Fatalf("a process given no key received %d bytes: %v", len(absent), err)
	}

	t.Setenv(CursorHMACKeyFileVariable, path)
	loaded, err := CursorSigningKey()
	if err != nil || string(loaded) != key {
		t.Fatalf("the key file was not loaded: %v", err)
	}

	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, []byte("\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(CursorHMACKeyFileVariable, empty)
	if _, err = CursorSigningKey(); err == nil {
		t.Fatal("accepted an empty cursor signing key")
	}

	t.Setenv(CursorHMACKeyFileVariable, filepath.Join(t.TempDir(), "absent"))
	if _, err = CursorSigningKey(); err == nil {
		t.Fatal("accepted a cursor signing key file that cannot be read")
	}
}
