package httpapi

import (
	"context"
	"net/http"
	"time"

	healthapi "github.com/Alisher24/CarSharing/backend/internal/contracts/healthapi"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Status = healthapi.ReadyStatus

type Check func(context.Context) (Status, error)

func DatabaseCheck(pool *pgxpool.Pool) Check {
	return func(ctx context.Context) (Status, error) {
		var s Status
		var postgis string
		err := pool.QueryRow(ctx, `SELECT city, currency, timezone, postgis_version()
			FROM bootstrap_metadata WHERE singleton = true`).Scan(&s.City, &s.Currency, &s.Timezone, &postgis)
		return s, err
	}
}

func Router(check Check) http.Handler {
	spec, err := healthapi.GetSwagger()
	if err != nil {
		panic(err)
	}
	for path, item := range spec.Paths.Map() {
		if len(item.Operations()) == 0 {
			spec.Paths.Delete(path)
		}
	}
	handler := healthapi.NewStrictHandlerWithOptions(health{check: check}, nil, healthapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, r, 400, "MALFORMED_JSON", "Malformed JSON")
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, r, 500, "INTERNAL_ERROR", "Internal server error")
		},
	})
	return boundary(spec, healthapi.Handler(handler))
}

type health struct{ check Check }

func (h health) GetHealthLive(ctx context.Context, _ healthapi.GetHealthLiveRequestObject) (healthapi.GetHealthLiveResponseObject, error) {
	return healthapi.GetHealthLive200JSONResponse{Body: healthapi.LiveStatus{Status: "ok", ServerTime: timestamp()}}, nil
}

func (h health) GetHealthReady(ctx context.Context, _ healthapi.GetHealthReadyRequestObject) (healthapi.GetHealthReadyResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	s, err := h.check(ctx)
	if err != nil {
		return healthapi.GetHealthReady503JSONResponse{Body: healthapi.ApiError{Code: "SERVICE_UNAVAILABLE", Message: "Service unavailable", RequestId: middleware.GetReqID(ctx)}}, nil
	}
	s.Status, s.ServerTime = "ok", timestamp()
	return healthapi.GetHealthReady200JSONResponse{Body: s}, nil
}

func timestamp() string {
	return time.Now().UTC().Truncate(time.Microsecond).Format("2006-01-02T15:04:05.000000Z")
}
