package httpapi

import "fmt"

// Catalog is the read side of everything a visitor sees without signing in. Each resource is read
// on its own, so one of them failing leaves the other two answerable.
type Catalog struct {
	Vehicles VehicleReader
	Zones    ZoneReader
	Tariffs  TariffReader
}

// catalogHandlers are the operations a visitor reads without an account, each over the reader
// that answers it.
type catalogHandlers struct {
	vehicles
	serviceZones
	prices
}

// newCatalogHandlers builds them, or names the reader that is missing. A reader is refused rather
// than defaulted, because a handler that reached a nil one would answer a request it never read.
func newCatalogHandlers(catalog Catalog) (catalogHandlers, error) {
	for _, required := range []struct {
		name     string
		supplied bool
	}{
		{"vehicle catalog", catalog.Vehicles != nil},
		{"service zones", catalog.Zones != nil},
		{"tariffs", catalog.Tariffs != nil},
	} {
		if !required.supplied {
			return catalogHandlers{}, fmt.Errorf("%w: %s", ErrIncompleteApplication, required.name)
		}
	}
	return catalogHandlers{
		vehicles:     vehicles{reader: catalog.Vehicles},
		serviceZones: serviceZones{reader: catalog.Zones},
		prices:       prices{reader: catalog.Tariffs},
	}, nil
}

func registerCatalogHandlers(served *server, dependencies Dependencies) error {
	var err error
	served.catalogHandlers, err = newCatalogHandlers(dependencies.Catalog)
	return err
}
