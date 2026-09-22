// Command democontrol is the terminal client of the demonstration surface. One subcommand is one
// set-to-value change, so a demonstration is driven by commands whose names say what they set, through
// the closed API and under the token of the demonstration capability.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/Alisher24/CarSharing/backend/internal/democontrol"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
)

// usage is what the command says when it is asked for something it does not apply, which is also the
// list a demonstration reads to find the command it needs. It names every command the vocabulary
// declares, so a new demonstration action appears in it without this file being edited.
var usage = "usage: democontrol <" + strings.Join(democontrol.Commands(), "|") + "> [flags]"

func main() {
	// Both the log and the answer of a command go to stdout, as they do in every other process here:
	// a collector that reads what a demonstration did reads one stream, not two.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

// appliedCommand is one subcommand as the demonstration applies it: the request the contract receives,
// and the service that applies it. The mail stub serves the loss of an answer under a capability of
// its own, so a subcommand names the surface it belongs to rather than the address being guessed at
// here.
type appliedCommand struct {
	ActionID string
	Request  democontrol.Request

	// Mail reports a command the mail stub applies rather than the API.
	Mail bool
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New(usage)
	}
	applied, err := command(os.Args[1], os.Args[2:])
	if err != nil {
		return err
	}
	client, err := clientFor(applied.Mail)
	if err != nil {
		return err
	}
	id, err := democontrol.ActionID(applied.ActionID)
	if err != nil {
		return err
	}
	applied.Request[democontrol.ActionIDField] = id

	ctx, cancel := context.WithTimeout(context.Background(), democontrol.RequestTimeout)
	defer cancel()
	answer, err := client.Apply(ctx, applied.Request)
	if err != nil {
		return err
	}
	fmt.Println(string(answer))
	return nil
}

// clientFor builds the client of the surface one command is applied to, each called under its own
// capability: the process allowed to move a vehicle by hand is not thereby allowed to arm the loss of
// a letter, and the one that arms it cannot move a vehicle.
func clientFor(mail bool) (*democontrol.Client, error) {
	capability, err := capabilityOf(mail)
	if err != nil {
		return nil, err
	}
	return democontrol.NewClient(capability.APIURL, capability.Token, http.DefaultTransport)
}

// capabilityOf reads what a command is called with: the demonstration capability of the API, or the
// one the mail stub declares for its own demonstration action.
func capabilityOf(mail bool) (config.InternalClient, error) {
	if mail {
		return config.MailstubDemoClient()
	}
	return config.DemoControlClient()
}

// command reads one subcommand and the flags it takes into the request the contract receives. The
// flags, what each of them says and what the action carries are the vocabulary's own declaration, so a
// new demonstration command is a row there rather than a case here.
func command(name string, arguments []string) (appliedCommand, error) {
	action, known := democontrol.ActionNamed(name)
	if !known {
		return appliedCommand{}, fmt.Errorf("the demonstration action %s is not one this build applies\n%s",
			name, usage)
	}
	flags := flag.NewFlagSet(name, flag.ExitOnError)
	actionID := flags.String("action-id", "",
		"the identifier this command is remembered by; a repeat of it reproduces its answer")
	action.Declare(flags)
	if err := flags.Parse(arguments); err != nil {
		return appliedCommand{}, err
	}
	request, err := action.Request(action.StatedOf(flags))
	if err != nil {
		return appliedCommand{}, fmt.Errorf("%w\n%s", err, usage)
	}
	return appliedCommand{ActionID: *actionID, Request: request, Mail: action.Mail}, nil
}
