package main

import (
	"net"
	"net/http"
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

	// readyPort is the port the probe looks on when HTTP_ADDR names no port at all: the port the
	// listener defaults to, so a deployment that moves the default moves the probe with it.
	readyPort = config.DefaultHTTPPort

	probeTimeout = 3 * time.Second
)

// readyProbe reports whether the API in this container answers its readiness operation.
func readyProbe() int {
	client := http.Client{Timeout: probeTimeout}
	resp, err := client.Get(readyURL())
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// readyURL is the readiness URL of the API in this container. The port comes from the same setting
// the API listens on, so a deployment that moves the listener does not leave the health check
// probing the port it used to be on, and the path comes from the operation the API serves.
func readyURL() string {
	_, port, err := net.SplitHostPort(config.HTTPAddrFromEnvironment())
	if err != nil || port == "" {
		port = readyPort
	}
	return readyScheme + net.JoinHostPort(readyHost, port) + httpapi.ReadyPath
}
