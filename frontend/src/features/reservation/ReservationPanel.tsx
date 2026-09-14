import { useState } from 'react';
import { isLiveRental, type CurrentSnapshot, type Rental, type ReservedRental } from '../../shared/api/current.ts';
import type { Resource } from '../../shared/api/Resource.ts';
import { loadedValue } from '../../shared/api/Resource.ts';
import { commandText } from './commandPhase.ts';
import type { Countdown, ServerClock } from './countdown.ts';
import {
  CANCEL_ACTION,
  CANCEL_CONFIRMED,
  CANCEL_QUESTION,
  CANCEL_WARNING,
  currentRental,
  EXPIRY_PENDING,
  GO_TO_VEHICLE,
  KEEP_ACTION,
  limitText,
  NOTHING_CURRENT,
  PANEL_HEADING,
  rateTextOf,
  REPEAT_ACTION,
  REPEAT_EXPIRED,
  RIDE_RUNNING,
  TARIFF_CHANGED,
  UNKNOWN_COMMAND,
  vehicleName,
} from './reservationCopy.ts';
import { RideView } from './RideView.tsx';
import { START_ACTION } from './rideCopy.ts';
import { TariffRates } from './TariffRates.tsx';
import { withinRepeatWindow, type UnfinishedCommand } from './unfinishedCommand.ts';
import { useCountdown } from './useCountdown.ts';
import type { Reservations } from './useReservations.ts';
import type { RideCommands } from './useRideCommands.ts';

type ReservationPanelProps = {
  /** What the private read answered. */
  resource: Resource<CurrentSnapshot | undefined>;

  reservations: Reservations;

  ride: RideCommands;

  /** What the panel asks the application to do with a vehicle that is held. */
  onShowVehicle: (vehicleId: string) => void;
};

/**
 * ReservationPanel is what a person reads about their own rental above the map. It is permanent while
 * somebody is signed in: a reservation shows the vehicle, the time left, the frozen rates, the
 * cancellation and the control that starts the ride, and a ride that has started shows its mode, its
 * durations and what it has cost so far. Without a rental it shows the day's allowance, which is not
 * something to infer from having no rental.
 */
export function ReservationPanel({ resource, reservations, ride, onShowVehicle }: ReservationPanelProps) {
  const snapshot = loadedValue(resource);
  if (snapshot === undefined) return null;

  const rental = currentRental(snapshot);
  const receivedAt = resource.phase === 'ready' || resource.phase === 'stale' ? resource.loadedAt : new Date();
  const clock: ServerClock = { serverTime: snapshot.server_time, receivedAt };
  return (
    <section className="reservation-panel" aria-label={PANEL_HEADING}>
      <h2 className="reservation-panel-heading">{PANEL_HEADING}</h2>
      {rental === undefined ? (
        <p className="reservation-panel-empty">{NOTHING_CURRENT}</p>
      ) : (
        <CurrentRental
          rental={rental}
          reservations={reservations}
          ride={ride}
          clock={clock}
          onShowVehicle={onShowVehicle}
        />
      )}
      <p className="reservation-panel-limit">{limitText(snapshot)}</p>
      <UnknownCommand held={repeatableOf(reservations, ride)} onRepeat={repeatOf(reservations, ride)} />
    </section>
  );
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
  if (rental.state === 'reserved') {
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

  return <RideView rental={rental} ride={ride} clock={clock} onShowVehicle={onShowVehicle} />;
}

function ReservedRental({
  rental,
  reservations,
  ride,
  clock,
  onShowVehicle,
}: {
  rental: ReservedRental;
  reservations: Reservations;
  ride: RideCommands;
  clock: ServerClock;
  onShowVehicle: (vehicleId: string) => void;
}) {
  const [asking, setAsking] = useState(false);
  const countdown = useCountdown({ expiresAt: rental.expires_at, ...clock });
  const sending = reservations.phase.state === 'sending' && reservations.phase.action === 'cancel';
  const starting = ride.phase.state === 'sending' && ride.phase.action === 'start';
  const notice = cancellingNotice(reservations) ?? startingNotice(ride);

  return (
    <div className="reservation-panel-current">
      <p className="reservation-panel-vehicle">{vehicleName(rental)}</p>
      <p className="reservation-panel-time" role="status">
        {countdown === undefined ? RIDE_RUNNING : countdownText(countdown)}
      </p>

      <TariffRates rates={rateTextOf(rental.tariff_snapshot)} />
      {reservations.ratesChanged && <p className="reservation-panel-notice">{TARIFF_CHANGED}</p>}

      <div className="reservation-panel-actions">
        <button
          className="action-button"
          type="button"
          disabled={starting || sending}
          onClick={() => ride.begin(rental.id)}
        >
          {START_ACTION}
        </button>
        <button className="action-button" type="button" onClick={() => onShowVehicle(rental.vehicle.id)}>
          {GO_TO_VEHICLE}
        </button>
        <button className="action-button" type="button" disabled={sending || starting} onClick={() => setAsking(true)}>
          {CANCEL_ACTION}
        </button>
      </div>

      {asking && (
        <div className="reservation-panel-confirm" role="group" aria-label={CANCEL_QUESTION}>
          <p className="reservation-panel-question">{CANCEL_QUESTION}</p>
          <p className="reservation-panel-warning">{CANCEL_WARNING}</p>
          <button
            className="action-button"
            type="button"
            onClick={() => {
              setAsking(false);
              reservations.cancel(rental.id);
            }}
          >
            {CANCEL_ACTION}
          </button>
          <button className="action-button" type="button" onClick={() => setAsking(false)}>
            {KEEP_ACTION}
          </button>
        </div>
      )}

      {notice !== undefined && <p className="reservation-panel-notice">{notice}</p>}
    </div>
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

/** What the panel says about the last cancellation, if it was the last command. */
function cancellingNotice(reservations: Reservations): string | undefined {
  const { phase } = reservations;
  if (phase.state === 'done' && phase.action === 'cancel') return CANCEL_CONFIRMED;
  if (phase.state === 'refused' && phase.action === 'cancel') return commandText(phase);

  return undefined;
}

/**
 * What the panel says about a refused start, which is the one ride command a reservation can send: why
 * the ride could not begin — an unfitting vehicle, a reservation that has run out — is what the person
 * has to read before deciding what to do with the reservation.
 */
function startingNotice(ride: RideCommands): string | undefined {
  const { phase } = ride;
  if (phase.state === 'refused' && phase.action === 'start') return commandText(phase);

  return undefined;
}

/**
 * UnknownCommand offers to settle a command whose answer never arrived. It is the only control that
 * sends a command with a key that was already used: a new booking is refused while the outcome is
 * unknown, because a new key would hide what the stored one already did.
 */
function UnknownCommand({ held, onRepeat }: { held: UnfinishedCommand | undefined; onRepeat: () => void }) {
  if (held === undefined) return null;

  return (
    <div className="reservation-panel-unknown" role="status">
      <p className="reservation-panel-notice">{UNKNOWN_COMMAND}</p>
      {withinRepeatWindow(held, Date.now()) ? (
        <button className="action-button" type="button" onClick={onRepeat}>
          {REPEAT_ACTION}
        </button>
      ) : (
        <p className="reservation-panel-notice">{REPEAT_EXPIRED}</p>
      )}
    </div>
  );
}

function countdownText(countdown: Countdown): string {
  if (countdown.state === 'left') return `Осталось ${countdown.text}`;
  return EXPIRY_PENDING;
}
