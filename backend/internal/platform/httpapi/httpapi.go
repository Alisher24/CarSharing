package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Status struct {
	Status     string `json:"status"`
	City       string `json:"city,omitempty"`
	Currency   string `json:"currency,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	ServerTime string `json:"server_time"`
}

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
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer)
	r.Get("/health/live", func(w http.ResponseWriter, r *http.Request) {
		write(w, http.StatusOK, Status{Status: "ok"})
	})
	ready := func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		s, err := check(ctx)
		if err != nil {
			write(w, http.StatusServiceUnavailable, Status{Status: "unavailable"})
			return
		}
		s.Status = "ok"
		write(w, http.StatusOK, s)
	}
	r.Get("/health/ready", ready)
	r.Get("/api/health", ready)
	return r
}

func write(w http.ResponseWriter, code int, s Status) {
	s.ServerTime = time.Now().UTC().Format(time.RFC3339Nano)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(s)
}
