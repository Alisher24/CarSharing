package main

import (
	"github.com/Alisher24/CarSharing/backend/internal/auth"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/notifications"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
)

type apiModules struct {
	users               *auth.UserStore
	authentication      *auth.Service
	vehicles            *fleet.Store
	prices              *tariffs.Store
	models              *simulation.Store
	issued              *invoices.Store
	rentalRecords       *rentals.Store
	reservations        *rentals.Service
	notificationRecords *notifications.Store
	notificationReads   *notifications.Service
	cursors             *cursor.Signer
}
