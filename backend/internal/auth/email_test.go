package auth

import (
	"strings"
	"testing"
)

func TestParseEmailCanonicalizesTheWholeAddress(t *testing.T) {
	cases := map[string]string{
		"plain":                  "user@example.test",
		"  surrounding spaces  ": "user@example.test",
		"uppercase local part":   "user@example.test",
		"uppercase domain":       "user@example.test",
		"tab and newline":        "user@example.test",
	}
	raw := map[string]string{
		"plain":                  "user@example.test",
		"  surrounding spaces  ": "  user@example.test  ",
		"uppercase local part":   "User@example.test",
		"uppercase domain":       "user@EXAMPLE.TEST",
		"tab and newline":        "\t user@Example.Test \n",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseEmail(raw[name])
			if err != nil {
				t.Fatalf("ParseEmail(%q) failed: %v", raw[name], err)
			}
			if string(got) != want {
				t.Fatalf("ParseEmail(%q) = %q, want %q", raw[name], got, want)
			}
		})
	}
}

func TestParseEmailRefusesAddressesTheContractCannotAccept(t *testing.T) {
	longLocalPart := strings.Repeat("a", 244) + "@example.test" // 257 characters
	cases := map[string]string{
		"empty":              "",
		"only whitespace":    "   ",
		"no at sign":         "userexample.test",
		"no domain dot":      "user@example",
		"two at signs":       "user@@example.test",
		"inner whitespace":   "us er@example.test",
		"missing local part": "@example.test",
		"missing domain":     "user@",
		"longer than 254":    longLocalPart,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := ParseEmail(raw); err == nil {
				t.Fatalf("ParseEmail(%q) = %q, want an error", raw, got)
			}
		})
	}
}

// A canonical address at the contract's limit is accepted, so the bound is inclusive rather than
// one character short of what the contract declares.
func TestParseEmailAcceptsTheLongestAllowedAddress(t *testing.T) {
	raw := strings.Repeat("a", 241) + "@example.test" // 254 characters
	got, err := ParseEmail(raw)
	if err != nil {
		t.Fatalf("ParseEmail of a 254-character address failed: %v", err)
	}
	if len([]rune(got)) != maxEmailLength {
		t.Fatalf("length = %d, want %d", len([]rune(got)), maxEmailLength)
	}
}
