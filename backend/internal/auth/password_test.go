package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePasswordAcceptsTheWholeAllowedRange(t *testing.T) {
	cases := map[string]string{
		"shortest allowed":  strings.Repeat("a", minPasswordCodePoints),
		"longest allowed":   strings.Repeat("a", maxPasswordCodePoints),
		"punctuation":       "!@#$%^&*()_+{}",
		"cyrillic":          "парольнадежный",
		"astral code point": strings.Repeat("🔑", minPasswordCodePoints),
	}
	for name, password := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePassword(password); err != nil {
				t.Fatalf("ValidatePassword rejected a valid password: %v", err)
			}
		})
	}
}

func TestValidatePasswordRefusesLengthsOutsideTheRange(t *testing.T) {
	cases := map[string]string{
		"empty":               "",
		"one short":           strings.Repeat("a", minPasswordCodePoints-1),
		"one long":            strings.Repeat("a", maxPasswordCodePoints+1),
		"astral one short":    strings.Repeat("🔑", minPasswordCodePoints-1),
		"astral one too many": strings.Repeat("🔑", maxPasswordCodePoints+1),
	}
	for name, password := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePassword(password); !errors.Is(err, ErrPasswordInvalid) {
				t.Fatalf("ValidatePassword(%d code points) = %v, want ErrPasswordInvalid",
					len([]rune(password)), err)
			}
		})
	}
}

// The range counts Unicode code points, so a password of astral characters is measured by the
// characters a person typed rather than by the units an encoding happens to use.
func TestValidatePasswordCountsCodePointsRatherThanEncodedUnits(t *testing.T) {
	astral := strings.Repeat("🔑", maxPasswordCodePoints)
	if len(astral) <= maxPasswordCodePoints {
		t.Fatal("test password is not longer in bytes than in code points")
	}
	if err := ValidatePassword(astral); err != nil {
		t.Fatalf("ValidatePassword rejected %d code points: %v", len([]rune(astral)), err)
	}
}

func TestValidatePasswordRefusesAnyUnicodeWhitespace(t *testing.T) {
	whitespace := map[string]rune{
		"space":           ' ',
		"tab":             '\t',
		"newline":         '\n',
		"carriage return": '\r',
		"vertical tab":    '\v',
		"form feed":       '\f',
		"no-break space":  '\u00a0',
		"ogham space":     '\u1680',
		"en quad":         '\u2000',
		"line separator":  '\u2028',
		"ideographic":     '\u3000',
	}
	for name, blank := range whitespace {
		t.Run(name, func(t *testing.T) {
			password := "correcthorse" + string(blank)
			if err := ValidatePassword(password); !errors.Is(err, ErrPasswordInvalid) {
				t.Fatalf("ValidatePassword accepted %s (U+%04X)", name, blank)
			}
		})
	}
}

// A password is never trimmed or normalized, so two spellings that a normalizer would fold
// together stay different secrets.
func TestValidatePasswordDoesNotFoldDistinctSpellings(t *testing.T) {
	composed := "e\u0301clairdepain"   // combining acute accent
	precomposed := "\u00e9clairdepain" // single precomposed character
	for _, password := range []string{composed, precomposed} {
		if err := ValidatePassword(password); err != nil {
			t.Fatalf("ValidatePassword rejected %q: %v", password, err)
		}
	}
	if composed == precomposed {
		t.Fatal("test inputs are not distinct")
	}
}
