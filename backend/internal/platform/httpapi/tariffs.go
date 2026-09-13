package httpapi

import (
	"context"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
)

// TariffReader is the read model the tariff operation answers from.
type TariffReader interface {
	Current(ctx context.Context) ([]tariffs.Tariff, error)
}

// prices answers the tariff operation. A tariff that could not be read is reported as a failure
// rather than answered with a zero price nobody set.
type prices struct{ reader TariffReader }

func (p prices) GetTariffs(
	ctx context.Context, _ servedapi.GetTariffsRequestObject,
) (servedapi.GetTariffsResponseObject, error) {
	current, err := p.reader.Current(ctx)
	if err != nil {
		return servedapi.GetTariffs503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	collection := servedapi.TariffCollection{Items: make([]servedapi.Tariff, 0, len(current))}
	for _, tariff := range current {
		collection.Items = append(collection.Items, publishedTariff(tariff))
	}
	return servedapi.GetTariffs200JSONResponse{Body: collection}, nil
}

func publishedTariff(tariff tariffs.Tariff) servedapi.Tariff {
	return servedapi.Tariff{
		Id:                               tariff.ID,
		Currency:                         servedapi.TariffCurrency(tariff.Currency),
		BillingPolicy:                    servedapi.TariffBillingPolicy(tariff.BillingPolicy),
		DrivingRateTyiynPerStartedMinute: exactInteger(tariff.DrivingRateTyiynPerStartedMinute),
		PausedRateTyiynPerStartedMinute:  exactInteger(tariff.PausedRateTyiynPerStartedMinute),
		Version:                          exactInteger(tariff.Version),
	}
}
