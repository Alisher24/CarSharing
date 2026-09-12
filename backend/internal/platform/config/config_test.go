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
