import { useCallback, useState } from 'react';
import { AccountPanel } from '../features/account/AccountPanel';
import { FilterBar } from '../features/fleet/FilterBar';
import { FleetStatus } from '../features/fleet/FleetStatus';
import { NO_FILTERS, selectVehicles, type FleetFilters } from '../features/fleet/filters';
import { catalogVehicles, foundTariff, foundZones } from '../features/fleet/useCatalog';
import { useSelectedVehicle } from '../features/fleet/useSelectedVehicle';
import { VehicleCard, type VehicleBooking } from '../features/fleet/VehicleCard';
import { VehicleList } from '../features/fleet/VehicleList';
import { FleetMap } from '../features/map/FleetMap';
import { ReservationPanel } from '../features/reservation/ReservationPanel';
import { ReservationWarning } from '../features/reservation/ReservationWarning';
import { commandText } from '../features/reservation/commandPhase';
import { limitAllowsBooking, limitText, SIGN_IN_TO_BOOK } from '../features/reservation/reservationCopy';
import type { Reservations } from '../features/reservation/useReservations';
import type { Application } from './useApplication';
import { loadedValue } from '../shared/api/Resource';
import type { CurrentSnapshot } from '../shared/api/current';

/** What a narrow screen is showing, where the map and the list cannot both fit. */
type NarrowView = 'map' | 'list';

type MapScreenProps = {
  application: Application;

  /** Whether the entry panel is open, which the one control in the header decides. */
  entryOpen: boolean;
};

/**
 * MapScreen is the address the application opens on: the fleet on a map and in a list, the rental in
 * force above them, and the card of whichever vehicle is selected. It is where a person acts on the
 * present; what they have already ridden and what they were charged for it is the cabinet's.
 */
export function MapScreen({ application, entryOpen }: MapScreenProps) {
  const { account, submission, submit, catalog, current, reservations, ride, notifications } = application;

  const [filters, setFilters] = useState<FleetFilters>(NO_FILTERS);
  const [selectedId, setSelectedId] = useState<string>();
  const [narrowView, setNarrowView] = useState<NarrowView>('map');

  const vehicles = catalogVehicles(catalog);
  const shown = selectVehicles(vehicles, filters);
  const selected = useSelectedVehicle(vehicles, selectedId);
  const select = useCallback((vehicleId: string) => setSelectedId(vehicleId), []);

  const currentSnapshot = loadedValue(current.resource);
  const booking = bookingOf(account.state === 'signed-in', currentSnapshot, reservations);

  return (
    <>
      {entryOpen && <AccountPanel account={account} submission={submission} onSubmit={submit} />}

      <ReservationPanel
        resource={current.resource}
        account={account}
        reservations={reservations}
        ride={ride}
        notifications={notifications}
        onShowVehicle={select}
      />
      <ReservationWarning current={current.resource} notifications={notifications} />

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
          booking={booking}
        />
      )}
    </>
  );
}

/**
 * What the card offers about booking. The day's allowance is read from the private answer rather
 * than guessed at: an allowance that has not been read yet is never presented as permission to book,
 * and the server decides every command again whatever the control looked like.
 */
function bookingOf(
  signedIn: boolean,
  snapshot: CurrentSnapshot | undefined,
  reservations: Reservations,
): VehicleBooking {
  return {
    signedIn,
    limit: signedIn ? limitText(snapshot) : SIGN_IN_TO_BOOK,
    limitAllows: limitAllowsBooking(snapshot),
    awaitingRepeat: reservations.repeatable !== undefined,
    notice: commandText(reservations.phase),
    book: reservations.book,
  };
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
