package main

import (
	"net/http"

	"github.com/Alisher24/CarSharing/backend/internal/demo"
)

// assembled is what one API process runs: the handler both of its surfaces are served through, and the
// demonstration source of telemetry it confirms the fleet with.
type assembled struct {
	handler       http.Handler
	confirmations *demo.Confirmations
}
