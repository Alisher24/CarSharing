// Command healthcheck probes readiness from inside the container for the Docker health check.
package main

import (
	"net"
	"net/http"
	"os"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/Alisher24/CarSharing/backend/internal/platform/httpapi"
)

const (
	// readyScheme is plain HTTP because the probe runs inside the container, on the loopback
	// address, beside the API it checks rather than through the proxy that terminates TLS.
	readyScheme = "http://"

	// readyHost is the container's own loopback address. The API listens on every interface, so a
	// container that published another one still reaches it here.
	readyHost = "127.0.0.1"

	probeTimeout = 3 * time.Second
)

func main() {
	os.Exit(probe(readyURL()))
}

// readyURL is the readiness URL of the API in this container. The address comes from the same
// setting the API listens on, so a deployment that moves the listener does not leave the health
// check probing the port it used to be on.
func readyURL() string {
	host, port, err := net.SplitHostPort(envOrDefault("HTTP_ADDR", config.DefaultHTTPAddr))
	if err != nil {
		host, port = readyHost, ""
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = readyHost
	}
	return readyScheme + net.JoinHostPort(host, port) + httpapi.ReadyPath
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func probe(url string) int {
	client := http.Client{Timeout: probeTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
