import { useCallback, useState } from 'react';
import { AccountPanel } from '../features/account/AccountPanel';
import { ConnectionIndicator } from '../features/connection/ConnectionIndicator';
import { useConnection, type Connection } from '../features/connection/useConnection';
import { FilterBar } from '../features/fleet/FilterBar';
import { FleetStatus } from '../features/fleet/FleetStatus';
import { NO_FILTERS, selectVehicles, type FleetFilters } from '../features/fleet/filters';
import { catalogVehicles, foundTariff, foundZones, useCatalog } from '../features/fleet/useCatalog';
import { useSelectedVehicle } from '../features/fleet/useSelectedVehicle';
import { VehicleCard } from '../features/fleet/VehicleCard';
import { VehicleList } from '../features/fleet/VehicleList';
import { FleetMap } from '../features/map/FleetMap';

/** What a narrow screen is showing, where the map and the list cannot both fit. */
type NarrowView = 'map' | 'list';

export function App() {
  const connection = useConnection();
  const catalog = useCatalog();
  const [filters, setFilters] = useState<FleetFilters>(NO_FILTERS);
  const [selectedId, setSelectedId] = useState<string>();
  const [narrowView, setNarrowView] = useState<NarrowView>('map');
  const [accountOpen, setAccountOpen] = useState(false);

  const vehicles = catalogVehicles(catalog);
  const shown = selectVehicles(vehicles, filters);
  const selected = useSelectedVehicle(vehicles, selectedId);

  const select = useCallback((vehicleId: string) => setSelectedId(vehicleId), []);

  return (
    <div className="page">
      <AppHeader
        connection={connection}
        accountOpen={accountOpen}
        onToggleAccount={() => setAccountOpen((open) => !open)}
      />
      {accountOpen && <AccountPanel />}

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
            onSelect={select}
          />
        </div>
        <div className="fleet-side-pane">
          <VehicleList vehicles={shown} selectedId={selectedId} onSelect={select} />
        </div>
      </main>

      {selected !== undefined && (
        <VehicleCard
          vehicle={selected}
          tariff={foundTariff(catalog)}
          onRetryTariff={catalog.tariffs.retry}
          withinFilters={shown.some((vehicle) => vehicle.id === selected.id)}
          onClose={() => setSelectedId(undefined)}
        />
      )}
    </div>
  );
}

type AppHeaderProps = { connection: Connection; accountOpen: boolean; onToggleAccount: () => void };

function AppHeader({ connection, accountOpen, onToggleAccount }: AppHeaderProps) {
  return (
    <header className="page-header">
      <a className="brand" href="/" aria-label="CarSharing — главная">
        <span className="brand-mark" aria-hidden="true">
          c↗
        </span>
        CarSharing
      </a>
      <span className="location">
        <span className="location-mark" aria-hidden="true">
          ◉
        </span>
        Бишкек
      </span>
      <ConnectionIndicator connection={connection} />
      <button className="header-action" type="button" aria-expanded={accountOpen} onClick={onToggleAccount}>
        Вход
      </button>
    </header>
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
