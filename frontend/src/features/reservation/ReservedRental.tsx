import { useState } from 'react';
import type { ReservedRental as Reserved } from '../../shared/api/current.ts';
import { KEEP_BOOKING_ACTION } from '../../shared/copy.ts';
import { commandText } from '../../shared/command/commandPhase.ts';
import type { ServerClock } from '../../shared/ride/serverClock.ts';
import { rateTextOf } from '../../shared/ride/fares.ts';
import { START_ACTION, TIME_LEFT, vehicleName } from '../../shared/ride/spell.ts';
import { TariffRates } from '../../shared/ride/TariffRates.tsx';
import type { Countdown } from '../../shared/ride/countdown.ts';
import { useCountdown } from '../../shared/ride/useCountdown.ts';
import {
  CANCEL_ACTION,
  CANCEL_CONFIRMED,
  CANCEL_QUESTION,
  CANCEL_WARNING,
  EXPIRY_PENDING,
  GO_TO_VEHICLE,
  TARIFF_CHANGED,
} from './reservationCopy.ts';
import type { Reservations } from './useReservations.ts';
import type { RideCommands } from './useRideCommands.ts';

type ReservedRentalProps = {
  rental: Reserved;
  reservations: Reservations;
  ride: RideCommands;
  clock: ServerClock;
  onShowVehicle: (vehicleId: string) => void;
};

/**
 * ReservedRental is the reservation in force: the vehicle it holds, the time left of it, the rates
 * it was made under, and the three things a person can do about it — start the ride, find the
 * vehicle, or give the reservation back. Giving it back is asked before it is sent, because the one
 * free reservation of the day is not returned by cancelling.
 */
export function ReservedRental({ rental, reservations, ride, clock, onShowVehicle }: ReservedRentalProps) {
  const [asking, setAsking] = useState(false);
  const countdown = useCountdown({ expiresAt: rental.expires_at, ...clock });
  const sending = reservations.phase.state === 'sending' && reservations.phase.action === 'cancel';
  const starting = ride.phase.state === 'sending' && ride.phase.action === 'start';
  const notice = cancellingNotice(reservations) ?? startingNotice(ride);

  return (
    <div className="reservation-panel-current">
      <p className="reservation-panel-vehicle">{vehicleName(rental)}</p>
      <p className="reservation-panel-time" role="status">
        {countdownText(countdown)}
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
        <ConfirmCancellation
          rentalId={rental.id}
          onCancel={() => {
            setAsking(false);
            reservations.cancel(rental.id);
          }}
          onKeep={() => setAsking(false)}
        />
      )}

      {notice !== undefined && <p className="reservation-panel-notice">{notice}</p>}
    </div>
  );
}

/** What giving a reservation back asks, and the two answers to it. */
function ConfirmCancellation({
  rentalId,
  onCancel,
  onKeep,
}: {
  rentalId: string;
  onCancel: () => void;
  onKeep: () => void;
}) {
  return (
    <div className="reservation-panel-confirm" role="group" aria-label={CANCEL_QUESTION}>
      <p className="reservation-panel-question">{CANCEL_QUESTION}</p>
      <p className="reservation-panel-warning">{CANCEL_WARNING}</p>
      <button className="action-button" type="button" onClick={onCancel} data-rental={rentalId}>
        {CANCEL_ACTION}
      </button>
      <button className="action-button" type="button" onClick={onKeep}>
        {KEEP_BOOKING_ACTION}
      </button>
    </div>
  );
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
 * What the panel says about the time the reservation has left. A deadline that cannot be read is not
 * counted down to: the panel then says nothing rather than a time it made up.
 */
function countdownText(countdown: Countdown | undefined): string | undefined {
  if (countdown === undefined || countdown.state === 'unreadable') return undefined;
  if (countdown.state === 'due') return EXPIRY_PENDING;

  return `${TIME_LEFT} ${countdown.text}`;
}
