package httpapi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
)

// An application assembled without one of its parts must fail where it is built, because the
// alternative is a handler that dereferences what it was not given and answers a client with a
// crash. The dependencies are filled in one at a time, so each step has exactly one absent and the
// error must name it.
func TestIncompleteApplicationIsRefusedAtConstruction(t *testing.T) {
	probe := func(context.Context) (servedapi.ReadyStatus, error) { return servedapi.ReadyStatus{}, nil }
	dependencies := Dependencies{Probe: probe}
	steps := []struct {
		absent string
		supply func(*Dependencies)
	}{
		{"database pool", func(d *Dependencies) { d.Pool = &pgxpool.Pool{} }},
		{"session manager", func(d *Dependencies) { d.Sessions = &sessions.Manager{} }},
		{"account service", func(d *Dependencies) { d.Auth = &auth.Service{} }},
		{"user store", func(d *Dependencies) { d.Users = &auth.UserStore{} }},
		{"rate-limit throttle", func(d *Dependencies) { d.Throttle = &auth.Throttle{} }},
	}

	for _, step := range steps {
		t.Run(step.absent, func(t *testing.T) {
			handler, err := NewHandler(dependencies)
			if !errors.Is(err, ErrIncompleteApplication) {
				t.Fatalf("err = %v, want %v", err, ErrIncompleteApplication)
			}
			if !strings.Contains(err.Error(), step.absent) {
				t.Fatalf("err = %v, does not name %q", err, step.absent)
			}
			if handler != nil {
				t.Fatal("an incomplete application produced a handler")
			}
		})
		step.supply(&dependencies)
	}
}
