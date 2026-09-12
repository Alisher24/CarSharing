// Command healthcheck probes readiness from inside the container for the Docker health check.
package main

import (
	"net/http"
	"os"
	"time"
)

const (
	// readyURL is reached from inside the container, so it targets the loopback address the API
	// listens on rather than the published port.
	readyURL = "http://127.0.0.1:8080/api/v1/health/ready"

	probeTimeout = 3 * time.Second
)

func main() {
	client := http.Client{Timeout: probeTimeout}
	resp, err := client.Get(readyURL)
	if err != nil {
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
