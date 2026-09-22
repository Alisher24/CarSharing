package cursor

import "testing"

// This fixture predates the shared payload type; either direction must keep its wire spelling.
const issuedCursorV1 = "51tmVtnm7T1ylY8dnzE44uRT72FiqJS13CkT05xDPHUeyJ2IjoxLCJ0IjoiMjAyNi0wOS0xMlQwNzoxNTozMC4xMjM" +
	"0NTZaIiwiaWQiOiIwMTk5NDM0Mi02YmE3LTcwMDAtODAwMC0wMDAwMDAwMDAwMTAiLCJvcCI6ImdldE5vdGlmaWNhd" +
	"GlvbnMiLCJzdWIiOiIwMTk5NDM0Mi02YmE3LTcwMDAtODAwMC0wMDAwMDAwMDAwMDEiLCJxIjp7ImxpbWl0IjoiMjA" +
	"ifX0"

func TestPreviouslyIssuedCursorKeepsItsWireFormat(t *testing.T) {
	signer := mustSigner(t, firstKey)
	token, err := signer.Issue(middlePosition, scopeOf("20"))
	if err != nil {
		t.Fatal(err)
	}
	if token != issuedCursorV1 {
		t.Fatalf("the cursor payload changed: %s", token)
	}
	position, err := signer.Read(issuedCursorV1, scopeOf("20"))
	if err != nil {
		t.Fatal(err)
	}
	if position != middlePosition {
		t.Fatalf("old cursor read as %+v", position)
	}
}
