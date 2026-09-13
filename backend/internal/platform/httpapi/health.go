package httpapi

import (
	"context"
	"time"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/go-chi/chi/v5/middleware"
)

// readinessProbeTimeout bounds the dependency check, so a stalled database answers as unavailable
// instead of holding the probe open until the client gives up.
const readinessProbeTimeout = 2 * time.Second

// health answers the two health operations. Liveness reports only that the process is running, so
// an orchestrator never restarts a healthy process over a failing dependency.
type health struct{ probe ReadinessProbe }

func (h health) GetHealthLive(
	ctx context.Context, _ servedapi.GetHealthLiveRequestObject,
) (servedapi.GetHealthLiveResponseObject, error) {
	live := servedapi.LiveStatus{Status: "ok", ServerTime: serverTime()}
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
			Message:   messageServiceUnavailable,
			RequestId: middleware.GetReqID(ctx),
		}}, nil
	}
	readiness.Status, readiness.ServerTime = "ok", serverTime()
	return servedapi.GetHealthReady200JSONResponse{Body: readiness}, nil
}

// serverTime is the time this process states about itself. Every other time a response carries is
// read from the database, which stays the authority; liveness is about the process rather than about
// the data, so it is answered from the process clock.
func serverTime() string {
	return timestamp.Format(time.Now())
}
