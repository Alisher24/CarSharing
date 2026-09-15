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
	"os"
	"strconv"

	"github.com/Alisher24/CarSharing/backend/internal/democontrol"
	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
)

// usage is what the command says when it is asked for something it does not apply, which is also the
// list a demonstration reads to find the command it needs.
const usage = "usage: democontrol <set-telemetry-state|set-position|set-energy-remaining|" +
	"mark-serviced|set-next-payment-outcome> [flags]"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New(usage)
	}
	actionID, request, err := command(os.Args[1], os.Args[2:])
	if err != nil {
		return err
	}

	cfg, err := config.DemoControlClient()
	if err != nil {
		return err
	}
	client, err := democontrol.NewClient(cfg.APIURL, cfg.Token)
	if err != nil {
		return err
	}
	id, err := democontrol.ActionID(actionID)
	if err != nil {
		return err
	}
	request["action_id"] = id

	ctx, cancel := context.WithTimeout(context.Background(), democontrol.RequestTimeout)
	defer cancel()
	answer, err := client.Apply(ctx, request)
	if err != nil {
		return err
	}
	fmt.Println(string(answer))
	return nil
}

// command reads one subcommand and the flags it takes into the request the contract receives. A new
// demonstration command is a new case here, and its flags are declared where it is.
func command(name string, arguments []string) (string, democontrol.Request, error) {
	flags := flag.NewFlagSet(name, flag.ExitOnError)
	actionID := flags.String("action-id", "",
		"the identifier this command is remembered by; a repeat of it reproduces its answer")

	switch name {
	case "set-telemetry-state":
		vehicle := flags.String("vehicle", "", "the vehicle to link or unlink")
		state := flags.String("state", "", "online or offline")
		if err := flags.Parse(arguments); err != nil {
			return "", nil, err
		}
		if *vehicle == "" || (*state != "online" && *state != "offline") {
			return "", nil, fmt.Errorf("--vehicle is required and --state is online or offline: %s", usage)
		}
		return *actionID, democontrol.Request{
			"action":          "set_telemetry_state",
			"vehicle_id":      *vehicle,
			"telemetry_state": *state,
		}, nil

	case "set-position":
		vehicle := flags.String("vehicle", "", "the vehicle to move")
		longitude := flags.Float64("longitude", 0, "the longitude to put it at")
		latitude := flags.Float64("latitude", 0, "the latitude to put it at")
		if err := flags.Parse(arguments); err != nil {
			return "", nil, err
		}
		if *vehicle == "" || !given(flags, "longitude") || !given(flags, "latitude") {
			return "", nil, fmt.Errorf("--vehicle, --longitude and --latitude are required: %s", usage)
		}
		return *actionID, democontrol.Request{
			"action":     "set_position",
			"vehicle_id": *vehicle,
			"position": map[string]any{
				"type":        "Point",
				"coordinates": []any{*longitude, *latitude},
			},
		}, nil

	case "set-energy-remaining":
		vehicle := flags.String("vehicle", "", "the vehicle whose reserve is stated")
		source := flags.String("source", "", "battery, gasoline, diesel, lpg or cng")
		remaining := flags.String("remaining", "", "what the source is left holding")
		if err := flags.Parse(arguments); err != nil {
			return "", nil, err
		}
		if *vehicle == "" || *source == "" || *remaining == "" {
			return "", nil, fmt.Errorf("--vehicle, --source and --remaining are required: %s", usage)
		}
		if _, err := strconv.ParseFloat(*remaining, 64); err != nil {
			return "", nil, fmt.Errorf("--remaining states a decimal: %w", err)
		}
		return *actionID, democontrol.Request{
			"action":      "set_energy_remaining",
			"vehicle_id":  *vehicle,
			"source_kind": *source,
			"remaining":   *remaining,
		}, nil

	case "mark-serviced":
		vehicle := flags.String("vehicle", "", "the vehicle to return to service")
		if err := flags.Parse(arguments); err != nil {
			return "", nil, err
		}
		if *vehicle == "" {
			return "", nil, fmt.Errorf("--vehicle is required: %s", usage)
		}
		return *actionID, democontrol.Request{
			"action":     "mark_serviced",
			"vehicle_id": *vehicle,
		}, nil

	case "set-next-payment-outcome":
		rental := flags.String("rental", "", "the ride whose next attempt is decided")
		outcome := flags.String("outcome", "", "paid or failed")
		if err := flags.Parse(arguments); err != nil {
			return "", nil, err
		}
		if *rental == "" || (*outcome != "paid" && *outcome != "failed") {
			return "", nil, fmt.Errorf("--rental is required and --outcome is paid or failed: %s", usage)
		}
		return *actionID, democontrol.Request{
			"action":    "set_next_payment_outcome",
			"rental_id": *rental,
			"outcome":   *outcome,
		}, nil

	default:
		return "", nil, fmt.Errorf("the demonstration action %s is not one this build applies\n%s",
			name, usage)
	}
}

// given reports whether a flag was named, which is how a coordinate of zero is told from one that was
// never stated.
func given(flags *flag.FlagSet, name string) bool {
	found := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == name {
			found = true
		}
	})
	return found
}
