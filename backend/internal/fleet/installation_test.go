package fleet

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type installedRouteRow struct {
	route string
}

func (r installedRouteRow) Scan(destinations ...any) error {
	route := r.route
	*destinations[0].(**string) = &route
	return nil
}

type installationTransaction struct {
	pgx.Tx
	installedRoute string
	published      bool
	change         *string
}

func (t *installationTransaction) Exec(
	_ context.Context,
	statement string,
	_ ...any,
) (pgconn.CommandTag, error) {
	if statement == insertVehicleStatement {
		return pgconn.NewCommandTag("INSERT 0 0"), nil
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (t *installationTransaction) QueryRow(
	_ context.Context,
	statement string,
	arguments ...any,
) pgx.Row {
	if statement == installedRouteSelection {
		return installedRouteRow{route: t.installedRoute}
	}
	t.published = true
	t.change = arguments[3].(*string)
	return recordedVersionRow{version: 2}
}

func TestInstallingAChangedRoutePublishesTheVehicleChange(t *testing.T) {
	transaction := &installationTransaction{installedRoute: "old-route"}
	store := &Store{}

	err := store.Install(context.Background(), transaction, InstalledVehicle{
		ID:      "01994342-6ba7-7000-8000-000000000001",
		RouteID: "new-route",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !transaction.published {
		t.Fatal("changed route did not publish a vehicle change")
	}
	if transaction.change == nil || *transaction.change != "new-route" {
		t.Fatalf("published route = %v", transaction.change)
	}
}

func TestInstallingTheSameRouteDoesNotPublishAChange(t *testing.T) {
	transaction := &installationTransaction{installedRoute: "route"}
	store := &Store{}

	err := store.Install(context.Background(), transaction, InstalledVehicle{
		ID:      "01994342-6ba7-7000-8000-000000000001",
		RouteID: "route",
	})
	if err != nil {
		t.Fatal(err)
	}
	if transaction.published {
		t.Fatal("unchanged route published a vehicle change")
	}
}
