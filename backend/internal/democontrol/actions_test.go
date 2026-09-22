package democontrol_test

import (
	"encoding/json"
	"flag"
	"io"
	"slices"
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/demoaction"
	"github.com/Alisher24/CarSharing/backend/internal/democontrol"
)

// Every action the vocabulary declares is stated by its own declaration and read back by the surface's
// own reader, so a row no terminal can state or no surface can read is a failing test rather than a
// command that exists on one side only.
func TestEveryActionTravelsFromATerminalToARequest(t *testing.T) {
	for _, action := range democontrol.Actions() {
		t.Run(action.Command(), func(t *testing.T) {
			stated := statedOf(action)
			request, err := action.Request(stated)
			if err != nil {
				t.Fatalf("the %s action could not be stated: %v", action.Kind, err)
			}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatalf("the %s request could not be written: %v", action.Kind, err)
			}
			read, err := democontrol.Read(body)
			if err != nil {
				t.Fatalf("the %s request could not be read: %v", action.Kind, err)
			}
			if read.Action != action.Kind {
				t.Fatalf("the request names the action %q, want %q", read.Action, action.Kind)
			}
			if !slices.Equal(read.Fields(), fieldsOf(action)) {
				t.Fatalf("the request states %v, want %v", read.Fields(), fieldsOf(action))
			}
			for _, value := range action.Values {
				if value.Kind == democontrol.Point {
					continue
				}
				carried, err := read.Text(value.Field)
				if err != nil {
					t.Fatalf("the %s field could not be read: %v", value.Field, err)
				}
				if carried != stated[value.Field][0] {
					t.Errorf("the %s field reads back as %q, want %q",
						value.Field, carried, stated[value.Field][0])
				}
			}
		})
	}
}

// A terminal's flags are the declaration's own names and hints, so the command line of every action is
// readable from the vocabulary rather than written beside it.
func TestATerminalDeclaresTheFlagsOfTheDeclaration(t *testing.T) {
	action, known := democontrol.ActionNamed("set-position")
	if !known {
		t.Fatal("the vocabulary does not declare the set-position command")
	}
	flags := flag.NewFlagSet(action.Command(), flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	action.Declare(flags)
	for _, named := range []string{"vehicle", "longitude", "latitude"} {
		if flags.Lookup(named) == nil {
			t.Fatalf("the declaration declares no --%s flag", named)
		}
	}
	if err := flags.Parse([]string{"--vehicle", "01994342-6ba7-7000-8000-000000000001",
		"--longitude", "74.6", "--latitude", "42.87"}); err != nil {
		t.Fatalf("the flags of the declaration could not be parsed: %v", err)
	}
	request, err := action.Request(action.StatedOf(flags))
	if err != nil {
		t.Fatalf("the stated command was refused: %v", err)
	}
	position, isPoint := request["position"].(map[string]any)
	if !isPoint {
		t.Fatalf("the position of the request is %#v", request["position"])
	}
	if !slices.Equal(position["coordinates"].([]any), []any{74.6, 42.87}) {
		t.Fatalf("the position of the request is %v", position["coordinates"])
	}
}

// A value the action cannot be applied without is refused by the terminal that states it, so a command
// line is not sent to the surface only to be refused there.
func TestACommandWithoutAValueItNeedsIsRefusedByTheTerminal(t *testing.T) {
	action, _ := democontrol.ActionNamed("mark-serviced")
	if _, err := action.Request(map[democontrol.Field][]string{
		democontrol.VehicleIDField: {""},
	}); err == nil {
		t.Fatal("a command naming no vehicle was accepted")
	}
}

// The two names of an action cannot collide with those of another: a terminal finds exactly one command
// to apply, and the surface exactly one action to read.
func TestEveryActionHasItsOwnTwoNames(t *testing.T) {
	names := map[demoaction.Kind]string{}
	commands := map[string]demoaction.Kind{}
	for _, action := range democontrol.Actions() {
		if other, taken := names[action.Kind]; taken {
			t.Errorf("the commands %s and %s share the action %q", other, action.Command(), action.Kind)
		}
		if other, taken := commands[action.Command()]; taken {
			t.Errorf("the actions %s and %s share the command %q", other, action.Kind, action.Command())
		}
		names[action.Kind] = action.Command()
		commands[action.Command()] = action.Kind
	}
	for _, command := range democontrol.Commands() {
		if _, declared := commands[command]; !declared {
			t.Errorf("the usage line offers the command %q, which no action declares", command)
		}
	}
}

// statedOf states one value of every kind the vocabulary declares, so an action travels the whole way
// from the command line to the request it is applied by.
func statedOf(action democontrol.Action) map[democontrol.Field][]string {
	stated := make(map[democontrol.Field][]string, len(action.Values))
	for _, value := range action.Values {
		stated[value.Field] = sampleOf(value)
	}
	return stated
}

func fieldsOf(action democontrol.Action) []democontrol.Field {
	fields := make([]democontrol.Field, 0, len(action.Values))
	for _, value := range action.Values {
		fields = append(fields, value.Field)
	}
	return fields
}

func sampleOf(value democontrol.Value) []string {
	switch value.Kind {
	case democontrol.Word:
		return []string{value.Words[0]}
	case democontrol.Number:
		return []string{"12.5"}
	case democontrol.Point:
		return []string{"74.6", "42.87"}
	default:
		return []string{"01994342-6ba7-7000-8000-000000000001"}
	}
}
