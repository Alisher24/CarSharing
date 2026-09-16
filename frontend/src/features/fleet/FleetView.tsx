import { useState } from 'react';
import { FilterBar } from './FilterBar';
import { FleetStatus } from './FleetStatus';
import { NO_FILTERS, selectVehicles, type FleetFilters } from './filters';
import { catalogVehicles, foundTariff, foundZones, type Catalog } from './useCatalog';
import { useSelectedVehicle } from './useSelectedVehicle';
import { VehicleCard, type VehicleBooking } from './VehicleCard';
import { VehicleList } from './VehicleList';
import { FleetMap } from '../map/FleetMap';

/** What a narrow screen is showing, where the map and the list cannot both fit. */
type NarrowView = 'map' | 'list';

type FleetViewProps = {
  catalog: Catalog;

  /** What the card of a selected vehicle offers about booking it. */
  booking: VehicleBooking;

  /** Which vehicle is selected, which the panel above the fleet can also decide. */
  selectedId: string | undefined;
  onSelect: (vehicleId: string) => void;
  onClearSelection: () => void;
};

/**
 * FleetView is the fleet itself: the filters, the map, the list and the card of whichever vehicle is
 * selected. Which vehicle that is belongs to the screen above it, because the panel of the rental in
 * force selects one too; what is filtered and which of the two panes a phone shows are its own.
 */
export function FleetView({ catalog, booking, selectedId, onSelect, onClearSelection }: FleetViewProps) {
  const [filters, setFilters] = useState<FleetFilters>(NO_FILTERS);
  const [narrowView, setNarrowView] = useState<NarrowView>('map');

  const vehicles = catalogVehicles(catalog);
  const shown = selectVehicles(vehicles, filters);
  const selected = useSelectedVehicle(vehicles, selectedId);

  return (
    <>
      <div className="fleet-bar">
        <FleetStatus resource={catalog.fleet.resource} onRetry={catalog.fleet.retry} />
        <NarrowViewSwitch view={narrowView} onChange={setNarrowView} />
        <FilterBar filters={filters} onChange={setFilters} />
      </div>

      <main className={`fleet-layout fleet-layout-${narrowView}`}>
        <div className="fleet-map-pane">
          <FleetMap
            vehicles={shown}
            zones={foundZones(catalog)}
            onRetryZones={catalog.zones.retry}
            selectedId={selectedId}
            onSelect={onSelect}
          />
        </div>
        <div className="fleet-side-pane">
          <VehicleList vehicles={shown} selectedId={selectedId} onSelect={onSelect} />
        </div>
      </main>

      {selected !== undefined && (
        <VehicleCard
          vehicle={selected}
          tariff={foundTariff(catalog)}
          onRetryTariff={catalog.tariffs.retry}
          withinFilters={shown.some((vehicle) => vehicle.id === selected.id)}
          onClose={onClearSelection}
          booking={booking}
        />
      )}
    </>
  );
}

/**
 * NarrowViewSwitch is how a phone moves between the map and the list. It changes only which of the
 * two is on screen: the filters and the selected vehicle are held above it and survive the switch.
 */
function NarrowViewSwitch({ view, onChange }: { view: NarrowView; onChange: (view: NarrowView) => void }) {
  return (
    <div className="view-switch" role="group" aria-label="Карта или список">
      <button className="view-tab" type="button" aria-pressed={view === 'map'} onClick={() => onChange('map')}>
        Карта
      </button>
      <button className="view-tab" type="button" aria-pressed={view === 'list'} onClick={() => onChange('list')}>
        Список
      </button>
    </div>
  );
}
