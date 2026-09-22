import { isLiveRental, type CurrentSnapshot, type Rental } from '../../shared/api/current.ts';
import { identityOf } from '../../shared/account/identity.ts';
import type { Notifications } from '../../shared/account/notifications.ts';
import type { Account } from '../../shared/account/session.ts';
import { usePayment, type Payment } from '../../shared/command/usePayment.ts';
import type { UnfinishedCommand } from '../../shared/command/unfinishedCommand.ts';
import { withinRepeatWindow } from '../../shared/command/unfinishedCommand.ts';
import { REPEAT_ACTION, REPEAT_EXPIRED, UNKNOWN_COMMAND } from '../../shared/copy.ts';
import type { Resource } from '../../shared/read/Resource.ts';
import { loadedValue } from '../../shared/read/Resource.ts';
import { currentRental, type ServerClock } from '../../shared/ride/serverClock.ts';
import { NOTHING_CURRENT } from '../../shared/ride/spell.ts';
import { useClockTick } from '../../shared/ride/useClockTick.ts';
import type { CompletedRideResult } from './completedResult.ts';
import { FinishedRideView } from './FinishedRideView.tsx';
import { limitText, PANEL_HEADING } from './reservationCopy.ts';
import { ReservedRental } from './ReservedRental.tsx';
import { RideView } from './RideView.tsx';
import { useCompletedRideResult } from './useCompletedRideResult.ts';
import type { Reservations } from './useReservations.ts';
import type { RideCommands } from './useRideCommands.ts';

type ReservationPanelProps = {
  /** What the private read answered. */
  resource: Resource<CurrentSnapshot | undefined>;

  /** The account itself, which the payment of an ending needs the session of. */
  account: Account;

  reservations: Reservations;

  ride: RideCommands;

  /** What the account was told, which is where a ride the service ended is reported. */
  notifications: Notifications;

  /** What the panel asks the application to do with a vehicle that is held. */
  onShowVehicle: (vehicleId: string) => void;
};

/**
 * ReservationPanel is what a person reads about their own rental above the map. It is permanent while
 * somebody is signed in: a reservation shows the vehicle, the time left, the frozen rates, the
 * cancellation and the control that starts the ride, and a ride that has started shows its mode, its
 * durations and what it has cost so far.
 *
 * A ride that is over shows what it cost, why it ended, when it ended and where the payment of that
 * charge stands. That result is read from the answers the service holds — the notification about the
 * ending and the invoice it names — so it is the same on the screen that ended the ride, after a
 * reload and in a tab that was never told anything; the history in the cabinet is where a ride older
 * than the last one is read. Without any of those the panel shows the day's allowance, which is not
 * something to infer from having no rental.
 */
export function ReservationPanel({
  resource,
  account,
  reservations,
  ride,
  notifications,
  onShowVehicle,
}: ReservationPanelProps) {
  const snapshot = loadedValue(resource);
  const { session, csrfToken } = identityOf(account);
  const paid = usePayment(session, csrfToken);
  const result = useCompletedRideResult({ ride, notifications, paid });
  if (snapshot === undefined) return null;

  const clock: ServerClock = { serverTime: snapshot.server_time, receivedAt: loadedMoment(resource) };
  return (
    <section className="reservation-panel" aria-label={PANEL_HEADING}>
      <h2 className="reservation-panel-heading">{PANEL_HEADING}</h2>
      <CurrentState
        rental={currentRental(snapshot)}
        result={result}
        paid={paid}
        reservations={reservations}
        ride={ride}
        clock={clock}
        onShowVehicle={onShowVehicle}
      />
      <p className="reservation-panel-limit">{limitText(snapshot)}</p>
      <UnknownCommand held={repeatableOf(reservations, ride)} onRepeat={repeatOf(reservations, ride)} />
    </section>
  );
}

/**
 * The local moment the answer on screen arrived at, which every interval is measured from. A reading
 * that is not on screen any more has no moment of its own, so the clock the panel holds is the one
 * that was taken when it was shown.
 */
function loadedMoment(resource: Resource<CurrentSnapshot | undefined>): Date {
  if (resource.phase === 'ready' || resource.phase === 'stale') return resource.loadedAt;

  return new Date();
}

/**
 * CurrentState is what the panel shows about the account: the rental in force, the ride that ended
 * last, or the statement that nothing is current. Which of them it is comes from the answer the
 * service gave rather than from what the interface last asked for.
 */
function CurrentState({
  rental,
  result,
  paid,
  reservations,
  ride,
  clock,
  onShowVehicle,
}: {
  rental: Rental | undefined;
  result: CompletedRideResult | undefined;
  paid: Payment;
  reservations: Reservations;
  ride: RideCommands;
  clock: ServerClock;
  onShowVehicle: (vehicleId: string) => void;
}) {
  if (rental !== undefined) {
    return (
      <CurrentRental
        rental={rental}
        reservations={reservations}
        ride={ride}
        clock={clock}
        onShowVehicle={onShowVehicle}
      />
    );
  }
  if (result !== undefined) return <FinishedRideView result={result} paid={paid} />;

  return <p className="reservation-panel-empty">{NOTHING_CURRENT}</p>;
}

/**
 * The rental in force, shown as what it currently is: a reservation that is waiting to be started, or
 * a ride. Which one it is comes from the answer the server gave, never from what the interface last
 * asked for; a rental that is over is not shown at all, because nothing about it is still current.
 */
function CurrentRental({
  rental,
  reservations,
  ride,
  clock,
  onShowVehicle,
}: {
  rental: Rental;
  reservations: Reservations;
  ride: RideCommands;
  clock: ServerClock;
  onShowVehicle: (vehicleId: string) => void;
}) {
  if (!isLiveRental(rental)) return null;
  if (rental.state !== 'reserved') {
    return <RideView rental={rental} ride={ride} clock={clock} onShowVehicle={onShowVehicle} />;
  }

  return (
    <ReservedRental
      rental={rental}
      reservations={reservations}
      ride={ride}
      clock={clock}
      onShowVehicle={onShowVehicle}
    />
  );
}

/** The command a reload left unanswered, whichever of the two senders holds it. */
function repeatableOf(reservations: Reservations, ride: RideCommands): UnfinishedCommand | undefined {
  return reservations.repeatable ?? ride.repeatable;
}

/** The way to settle that command, which is the sender that holds it. */
function repeatOf(reservations: Reservations, ride: RideCommands): () => void {
  return reservations.repeatable === undefined ? ride.repeat : reservations.repeat;
}

/**
 * UnknownCommand offers to settle a command whose answer never arrived. It is the only control that
 * sends a command with a key that was already used: a new booking is refused while the outcome is
 * unknown, because a new key would hide what the stored one already did.
 */
function UnknownCommand({ held, onRepeat }: { held: UnfinishedCommand | undefined; onRepeat: () => void }) {
  const now = useClockTick();
  if (held === undefined) return null;

  return (
    <div className="reservation-panel-unknown" role="status">
      <p className="reservation-panel-notice">{UNKNOWN_COMMAND}</p>
      <RepeatCommand held={held} now={now} onRepeat={onRepeat} />
    </div>
  );
}

/** The one control that settles an unknown outcome, or what is said instead of it past its window. */
function RepeatCommand({ held, now, onRepeat }: { held: UnfinishedCommand; now: Date; onRepeat: () => void }) {
  if (!withinRepeatWindow(held, now.getTime())) {
    return <p className="reservation-panel-notice">{REPEAT_EXPIRED}</p>;
  }

  return (
    <button className="action-button" type="button" onClick={onRepeat}>
      {REPEAT_ACTION}
    </button>
  );
}
