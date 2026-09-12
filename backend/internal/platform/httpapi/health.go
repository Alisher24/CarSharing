package httpapi

import (
	"context"
	"time"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	// readinessProbeTimeout bounds the dependency check, so a stalled database answers as
	// unavailable instead of holding the probe open until the client gives up.
	readinessProbeTimeout = 2 * time.Second

	// timestampLayout renders the exactly six fractional digits the contract's Timestamp declares.
	timestampLayout = "2006-01-02T15:04:05.000000Z"
)

// Status is the readiness payload: the city, currency and timezone this deployment serves.
type Status = servedapi.ReadyStatus

// health answers the two health operations. Liveness reports only that the process is running, so
// an orchestrator never restarts a healthy process over a failing dependency.
type health struct{ probe ReadinessProbe }

func (h health) GetHealthLive(
	ctx context.Context, _ servedapi.GetHealthLiveRequestObject,
) (servedapi.GetHealthLiveResponseObject, error) {
	live := servedapi.LiveStatus{Status: "ok", ServerTime: timestamp()}
	return servedapi.GetHealthLive200JSONResponse{Body: live}, nil
}

func (h health) GetHealthReady(
	ctx context.Context, _ servedapi.GetHealthReadyRequestObject,
) (servedapi.GetHealthReadyResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, readinessProbeTimeout)
	defer cancel()
	readiness, err := h.probe(ctx)
	if err != nil {
		return servedapi.GetHealthReady503JSONResponse{Body: servedapi.ApiError{
			Code:      codeServiceUnavailable,
			Message:   "Service unavailable",
			RequestId: middleware.GetReqID(ctx),
		}}, nil
	}
	readiness.Status, readiness.ServerTime = "ok", timestamp()
	return servedapi.GetHealthReady200JSONResponse{Body: readiness}, nil
}

func timestamp() string {
	return formatTimestamp(time.Now())
}

// formatTimestamp renders an instant the way every contract timestamp is declared, so a stored
// time and a server time are never spelled differently in one response.
func formatTimestamp(instant time.Time) string {
	return instant.UTC().Truncate(time.Microsecond).Format(timestampLayout)
}
