package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMailstubServerLoadsTheWholeProcessShape(t *testing.T) {
	secrets := t.TempDir()
	setSecret := func(variable, value string) {
		t.Helper()
		path := filepath.Join(secrets, strings.ToLower(variable))
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(variable, path)
	}

	setSecret(DatabasePasswordFileVariable, strings.Repeat("d", 48))
	setSecret(MailstubDeliveryTokenFileVariable, "delivery-token")
	setSecret(MailstubDemoTokenFileVariable, "demo-token")
	setSecret(MailstubCursorHMACKeyFileVariable, strings.Repeat("k", 48))
	t.Setenv(HTTPAddrVariable, ":9080")
	t.Setenv(EnvironmentVariable, DemoEnvironment)

	configuration, err := MailstubServerFromEnvironment()
	if err != nil {
		t.Fatalf("the mail stub configuration was refused: %v", err)
	}
	if configuration.InternalAddr != ":9080" {
		t.Errorf("the internal listener is %q", configuration.InternalAddr)
	}
	if configuration.Environment.Name != DemoEnvironment {
		t.Errorf("the environment is %q", configuration.Environment.Name)
	}
	if configuration.Database.Password != strings.Repeat("d", 48) {
		t.Error("the database password was not loaded")
	}
}
