// Command healthcheck answers one health check of a container from inside it, for the service that
// runs it: the readiness of a process that serves an interface, or the mark a process with nothing to
// serve leaves behind. The probe is named by the argument the service passes, and its exit status is
// what Docker reads.
package main

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
)

// probes are the health checks this command answers, by the name the service passes it. A service
// adds a probe by adding a row here and naming it in its own health check.
var probes = map[string]func() int{
	"ready":     readyProbe,
	"heartbeat": heartbeatProbe,
}

func main() {
	probe, known := probes[probeArgument()]
	if !known {
		fmt.Fprintf(os.Stderr, "usage: healthcheck <%s>\n", joinNames())
		os.Exit(2)
	}
	os.Exit(probe())
}

// probeArgument is the probe the service asked for, which is absent when the command is run by hand.
func probeArgument() string {
	if len(os.Args) < 2 {
		return ""
	}
	return os.Args[1]
}

// joinNames spells the probes as the usage line states them, so the line cannot offer a probe this
// command does not answer.
func joinNames() string {
	return strings.Join(slices.Sorted(maps.Keys(probes)), "|")
}
