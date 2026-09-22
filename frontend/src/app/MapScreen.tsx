import { useCallback, useState } from 'react';
import { FleetView } from '../features/fleet/FleetView.tsx';
import type { FleetMapPane } from '../features/fleet/FleetView.tsx';
import type { VehicleBooking } from '../features/fleet/VehicleCard.tsx';
import { FleetMap } from '../features/map/FleetMap.tsx';
import { ReservationPanel } from '../features/reservation/ReservationPanel.tsx';
import { ReservationWarning } from '../features/reservation/ReservationWarning.tsx';
import { limitAllowsBooking, limitText } from '../features/reservation/reservationCopy.ts';
import type { Reservations } from '../features/reservation/useReservations.ts';
import type { Account } from '../shared/account/session.ts';
import type { CurrentSnapshot } from '../shared/api/current.ts';
import { commandText } from '../shared/command/commandPhase.ts';
import { SIGN_IN_TO_BOOK } from '../shared/copy.ts';
import { loadedValue } from '../shared/read/Resource.ts';
import type { Application } from './useApplication.ts';

type MapScreenProps = {
  application: Application;
};

/**
 * MapScreen is the address the application opens on: what the person's own rental is doing, and the
 * fleet below it. It is where a person acts on the present; what they have already ridden and what
 * they were charged for it is the cabinet's.
 *
 * Which vehicle is selected is held here rather than by the fleet, because the panel of the rental in
 * force selects one too: «показать машину» and a click in the list are two ways to the same card.
 */
export function MapScreen({ application }: MapScreenProps) {
  const { account, catalog, current, reservations, ride, notifications } = application;
  const [selectedId, setSelectedId] = useState<string>();

  const select = useCallback((vehicleId: string) => setSelectedId(vehicleId), []);
  const clearSelection = useCallback(() => setSelectedId(undefined), []);
  const booking = bookingOf(account, loadedValue(current.resource), reservations);

  // The map belongs to its own feature and the fleet states what it is about, so the screen the two
  // share hands the map over rather than either feature naming the other.
  const map = useCallback<FleetMapPane>(
    (vehicles, zones) => (
      <FleetMap
        vehicles={vehicles}
        zones={zones}
        onRetryZones={catalog.zones.retry}
        selectedId={selectedId}
        onSelect={select}
      />
    ),
    [catalog.zones.retry, selectedId, select],
  );

  return (
    <>
      <ReservationPanel
        resource={current.resource}
        account={account}
        reservations={reservations}
        ride={ride}
        notifications={notifications}
        onShowVehicle={select}
      />
      <ReservationWarning current={current.resource} notifications={notifications} />

      <FleetView
        catalog={catalog}
        booking={booking}
        map={map}
        selectedId={selectedId}
        onSelect={select}
        onClearSelection={clearSelection}
      />
    </>
  );
}

/**
 * What the card offers about booking. The day's allowance is read from the private answer rather
 * than guessed at: an allowance that has not been read yet is never presented as permission to book,
 * and the server decides every command again whatever the control looked like.
 */
function bookingOf(
  account: Account,
  snapshot: CurrentSnapshot | undefined,
  reservations: Reservations,
): VehicleBooking {
  const signedIn = account.state === 'signed-in';
  return {
    signedIn,
    limit: signedIn ? limitText(snapshot) : SIGN_IN_TO_BOOK,
    limitAllows: limitAllowsBooking(snapshot),
    awaitingRepeat: reservations.repeatable !== undefined,
    notice: commandText(reservations.phase),
    book: reservations.book,
  };
}
