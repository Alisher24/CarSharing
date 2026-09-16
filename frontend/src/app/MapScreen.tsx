import { useCallback, useState } from 'react';
import { AccountPanel } from '../features/account/AccountPanel';
import { FleetView } from '../features/fleet/FleetView';
import type { VehicleBooking } from '../features/fleet/VehicleCard';
import { ReservationPanel } from '../features/reservation/ReservationPanel';
import { ReservationWarning } from '../features/reservation/ReservationWarning';
import { commandText } from '../features/reservation/commandPhase';
import { limitAllowsBooking, limitText, SIGN_IN_TO_BOOK } from '../features/reservation/reservationCopy';
import type { Reservations } from '../features/reservation/useReservations';
import type { Application } from './useApplication';
import { loadedValue } from '../shared/api/Resource';
import type { CurrentSnapshot } from '../shared/api/current';

type MapScreenProps = {
  application: Application;

  /** Whether the entry panel is open, which the one control in the header decides. */
  entryOpen: boolean;
};

/**
 * MapScreen is the address the application opens on: what the person's own rental is doing, and the
 * fleet below it. It is where a person acts on the present; what they have already ridden and what
 * they were charged for it is the cabinet's.
 *
 * Which vehicle is selected is held here rather than by the fleet, because the panel of the rental in
 * force selects one too: «показать машину» and a click in the list are two ways to the same card.
 */
export function MapScreen({ application, entryOpen }: MapScreenProps) {
  const { account, submission, submit, catalog, current, reservations, ride, notifications } = application;
  const [selectedId, setSelectedId] = useState<string>();

  const select = useCallback((vehicleId: string) => setSelectedId(vehicleId), []);
  const clearSelection = useCallback(() => setSelectedId(undefined), []);
  const booking = bookingOf(account.state === 'signed-in', loadedValue(current.resource), reservations);

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

      <FleetView
        catalog={catalog}
        booking={booking}
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
