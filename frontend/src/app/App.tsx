import { useCallback, useEffect, useRef, useState } from 'react';
import { AccountPanel } from '../features/account/AccountPanel';
import { useAccount } from '../features/account/useAccount';
import { ConnectionIndicator } from '../features/connection/ConnectionIndicator';
import { StreamNotice } from '../features/connection/StreamNotice';
import { useConnection, type Connection } from '../features/connection/useConnection';
import type { EventsConnection } from '../features/events/useEventStream';
import { useEvents } from '../features/events/useEvents';
import { usePrivateEvents } from '../features/events/usePrivateEvents';
import { FilterBar } from '../features/fleet/FilterBar';
import { FleetStatus } from '../features/fleet/FleetStatus';
import { NO_FILTERS, selectVehicles, type FleetFilters } from '../features/fleet/filters';
import { catalogVehicles, foundTariff, foundZones, useCatalog } from '../features/fleet/useCatalog';
import { useSelectedVehicle } from '../features/fleet/useSelectedVehicle';
import { VehicleCard, type VehicleBooking } from '../features/fleet/VehicleCard';
import { VehicleList } from '../features/fleet/VehicleList';
import { FleetMap } from '../features/map/FleetMap';
import { useNotifications } from '../features/notifications/useNotifications';
import { ReservationPanel } from '../features/reservation/ReservationPanel';
import { ReservationWarning } from '../features/reservation/ReservationWarning';
import { commandText } from '../features/reservation/commandPhase';
import { limitAllowsBooking, limitText, SIGN_IN_TO_BOOK } from '../features/reservation/reservationCopy';
import { useCurrentRental } from '../features/reservation/useCurrentRental';
import { useReservations } from '../features/reservation/useReservations';
import { useRideCommands } from '../features/reservation/useRideCommands';
import { loadedValue } from '../shared/api/Resource';
import type { CurrentSnapshot } from '../shared/api/current';
import type { Reservations } from '../features/reservation/useReservations';

/** What a narrow screen is showing, where the map and the list cannot both fit. */
type NarrowView = 'map' | 'list';

export function App() {
  const connection = useConnection();
  const { account, submission, submit, leave, recheck } = useAccount();
  const events = useEvents();
  const catalog = useCatalog(events);

  // The private stream is opened by the session and lives above every panel that shows it, so
  // closing that panel changes nothing about it. A subscription that has ended asks who the caller
  // is now: an answer that nobody is signed in clears the session, and with it this stream.
  const session = account.state === 'signed-in' ? account.snapshot.user.id : undefined;
  const privateEvents = usePrivateEvents(session, recheck);
  useSessionCheckOnRecovery(events.connection, recheck);

  // What the person is doing now, and the commands that change it, are read and held here rather
  // than by a panel: the panel above the map and the card that books a vehicle are two views of one
  // reservation, and the private stream keeps both of them current.
  const current = useCurrentRental(account, privateEvents);
  const reservations = useReservations(account, current);
  const ride = useRideCommands(account, current);
  const notifications = useNotifications(account, privateEvents);
  const currentSnapshot = loadedValue(current.resource);

  const [filters, setFilters] = useState<FleetFilters>(NO_FILTERS);
  const [selectedId, setSelectedId] = useState<string>();
  const [narrowView, setNarrowView] = useState<NarrowView>('map');
  const [accountOpen, setAccountOpen] = useState(false);

  const vehicles = catalogVehicles(catalog);
  const shown = selectVehicles(vehicles, filters);
  const selected = useSelectedVehicle(vehicles, selectedId);

  const select = useCallback((vehicleId: string) => setSelectedId(vehicleId), []);

  const booking = bookingOf(account.state === 'signed-in', currentSnapshot, reservations);

  return (
    <div className="page">
      <AppHeader
        connection={connection}
        stream={events.connection}
        accountOpen={accountOpen}
        onToggleAccount={() => setAccountOpen((open) => !open)}
      />
      {accountOpen && <AccountPanel account={account} submission={submission} onSubmit={submit} onLeave={leave} />}

      <ReservationPanel
        resource={current.resource}
        owner={session}
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
    </div>
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
 * A connection that has come back is the moment to ask the server who the caller is now: the
 * session may have been revoked while the browser could not reach anything, and a session that is
 * still live lets the private stream open again. Reading the snapshots again is the handshake's
 * business, and a handshake follows this recovery.
 */
function useSessionCheckOnRecovery(stream: EventsConnection, recheck: () => void): void {
  const previous = useRef(stream);

  useEffect(() => {
    const recovered = stream === 'connected' && previous.current !== 'connected';
    previous.current = stream;
    if (recovered) recheck();
  }, [stream, recheck]);
}

type AppHeaderProps = {
  connection: Connection;
  stream: EventsConnection;
  accountOpen: boolean;
  onToggleAccount: () => void;
};

function AppHeader({ connection, stream, accountOpen, onToggleAccount }: AppHeaderProps) {
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
      <StreamNotice stream={stream} connection={connection} />
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
