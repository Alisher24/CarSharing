package main

import (
	"context"

	"github.com/Alisher24/CarSharing/backend/internal/platform/httpapi"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

func readinessMetadata(store *rentals.Store) httpapi.ReadReadyMetadata {
	return func(ctx context.Context) (httpapi.ReadyMetadata, error) {
		return readReadyMetadata(ctx, store)
	}
}

func readReadyMetadata(ctx context.Context, store *rentals.Store) (httpapi.ReadyMetadata, error) {
	metadata, err := store.Metadata(ctx)
	if err != nil {
		return httpapi.ReadyMetadata{}, err
	}
	return httpapi.ReadyMetadata{
		City:     metadata.City,
		Currency: metadata.Currency,
		Timezone: metadata.Timezone,
	}, nil
}
