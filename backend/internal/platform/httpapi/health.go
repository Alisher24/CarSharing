package httpapi

import (
	"context"
	"time"

	healthapi "github.com/Alisher24/CarSharing/backend/internal/contracts/healthapi"
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
type Status = healthapi.ReadyStatus

// health answers the two health operations. Liveness reports only that the process is running, so
// an orchestrator never restarts a healthy process over a failing dependency.
type health struct{ probe ReadinessProbe }

func (h health) GetHealthLive(
	ctx context.Context, _ healthapi.GetHealthLiveRequestObject,
) (healthapi.GetHealthLiveResponseObject, error) {
	live := healthapi.LiveStatus{Status: "ok", ServerTime: timestamp()}
	return healthapi.GetHealthLive200JSONResponse{Body: live}, nil
}

func (h health) GetHealthReady(
	ctx context.Context, _ healthapi.GetHealthReadyRequestObject,
) (healthapi.GetHealthReadyResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, readinessProbeTimeout)
	defer cancel()
	readiness, err := h.probe(ctx)
	if err != nil {
		return healthapi.GetHealthReady503JSONResponse{Body: healthapi.ApiError{
			Code:      codeServiceUnavailable,
			Message:   "Service unavailable",
			RequestId: middleware.GetReqID(ctx),
		}}, nil
	}
	readiness.Status, readiness.ServerTime = "ok", timestamp()
	return healthapi.GetHealthReady200JSONResponse{Body: readiness}, nil
}

func timestamp() string {
	return time.Now().UTC().Truncate(time.Microsecond).Format(timestampLayout)
}
