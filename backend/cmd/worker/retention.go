package main

import (
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/idempotency"
	"github.com/Alisher24/CarSharing/backend/internal/platform/ratelimit"
	"github.com/Alisher24/CarSharing/backend/internal/platform/retention"
	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
)

func retentionSweeps() []retention.Sweep {
	return []retention.Sweep{
		{Name: "signal retention", Delete: events.DeleteExpired},
		{Name: "command result retention", Delete: idempotency.DeleteExpired},
		{Name: "sign-in counter retention", Delete: ratelimit.DeleteExpired},
		{Name: "session retention", Delete: sessions.DeleteExpired},
	}
}
