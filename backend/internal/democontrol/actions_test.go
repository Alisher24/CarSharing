package democontrol_test

import (
	"encoding/json"
	"flag"
	"io"
	"slices"
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/democontrol"
)

// Every action the vocabulary declares is stated by its own declaration and read back by the surface's
// own reader, so a row that no terminal can state or no surface can read is a failing test rather than
// a command that exists on one side only.
func TestEveryActionTravelsFromATerminalToARequest(t *testing.T) {
	for _, action := range democontrol.Actions() {
		t.Run(action.Command(), func(t *testing.T) {
			stated := statedOf(action)
			request, err := action.Request(stated)
			if err != nil {
				t.Fatalf("the %s action could not be stated: %v", action.Name, err)
			}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatalf("the %s request could not be written: %v", action.Name, err)
			}
			read, err := democontrol.Read(body)
			if err != nil {
				t.Fatalf("the %s request could not be read: %v", action.Name, err)
			}
			if read.Action != action.Name {
				t.Fatalf("the request names the action %q, want %q", read.Action, action.Name)
			}
			for _, value := range action.Values {
				if !slices.Equal(read.Values[value.Field], stated[value.Field]) {
					t.Errorf("the %s field reads back as %v, want %v",
						value.Field, read.Values[value.Field], stated[value.Field])
				}
			}
		})
	}
}

// A terminal's flags are the declaration's own names, so the command line of every action is readable
// from the vocabulary rather than written beside it.
func TestATerminalDeclaresTheFlagsOfTheDeclaration(t *testing.T) {
	action, known := democontrol.ActionNamed("set-position")
	if !known {
		t.Fatal("the vocabulary does not declare the set-position command")
	}
	flags := flag.NewFlagSet(action.Command(), flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	action.Declare(flags)
	if err := flags.Parse([]string{"--vehicle", "01994342-6ba7-7000-8000-000000000001",
		"--longitude", "74.6", "--latitude", "42.87"}); err != nil {
		t.Fatalf("the flags of the declaration could not be parsed: %v", err)
	}
	request, err := action.Request(action.StatedOf(flags))
	if err != nil {
		t.Fatalf("the stated command was refused: %v", err)
	}
	position, isPoint := request[string(democontrol.PositionField)].(map[string]any)
	if !isPoint {
		t.Fatalf("the position of the request is %#v", request[string(democontrol.PositionField)])
	}
	if !slices.Equal(position["coordinates"].([]any), []any{74.6, 42.87}) {
		t.Fatalf("the position of the request is %v", position["coordinates"])
	}
}

// A value the action cannot be applied without is refused by the terminal that states it, so a command
// line is not sent to the surface only to be refused there.
func TestACommandWithoutAValueItNeedsIsRefusedByTheTerminal(t *testing.T) {
	action, _ := democontrol.ActionNamed("mark-serviced")
	_, err := action.Request(map[democontrol.Field][]string{democontrol.VehicleIDField: {""}})
	if err == nil {
		t.Fatal("a command naming no vehicle was accepted")
	}
}

// The two names of an action cannot collide with those of another: a terminal finds exactly one
// command to apply, and the surface exactly one action to read.
func TestEveryActionHasItsOwnTwoNames(t *testing.T) {
	names := map[string]string{}
	commands := map[string]string{}
	for _, action := range democontrol.Actions() {
		if other, taken := names[action.Name]; taken {
			t.Errorf("the actions %s and %s share the name %q", other, action.Command(), action.Name)
		}
		if other, taken := commands[action.Command()]; taken {
			t.Errorf("the commands %s and %s share the name %q", other, action.Name, action.Command())
		}
		names[action.Name] = action.Command()
		commands[action.Command()] = action.Name
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
