import { useEffect, useState } from 'react';
import type { CurrentSnapshot, Rental } from '../../shared/api/current.ts';
import type { Resource } from '../../shared/api/Resource.ts';
import { loadedValue } from '../../shared/api/Resource.ts';
import { countdownAt, type Countdown, type Deadline } from './countdown.ts';
import {
  CANCEL_ACTION,
  CANCEL_CONFIRMED,
  CANCEL_PENDING,
  CANCEL_QUESTION,
  CANCEL_WARNING,
  commandNotice,
  currentRental,
  EXPIRY_PENDING,
  GO_TO_VEHICLE,
  limitText,
  NOTHING_CURRENT,
  PANEL_HEADING,
  rateTextOf,
  KEEP_ACTION,
  REPEAT_ACTION,
  REPEAT_EXPIRED,
  RIDE_RUNNING,
  TARIFF_CHANGED,
  UNKNOWN_COMMAND,
  vehicleName,
} from './reservationCopy.ts';
import { withinRepeatWindow } from './unfinishedCommand.ts';
import type { Reservations } from './useReservations.ts';

/**
 * How often the remaining time is recomputed. It is a redraw rather than a count: the value comes
 * from the deadline, the moment the server computed its answer at and the moment that answer
 * arrived, so a tick that never happened costs a late redraw and nothing else.
 */
const TICK_MILLISECONDS = 1_000;

type ReservationPanelProps = {
  /** What the private read answered. */
  resource: Resource<CurrentSnapshot | undefined>;

  reservations: Reservations;

  /** What the panel asks the application to do with a vehicle that is held. */
  onShowVehicle: (vehicleId: string) => void;
};

/**
 * ReservationPanel is what a person reads about their own reservation above the map. It is permanent
 * while somebody is signed in: with a reservation it shows the vehicle, the time left, the frozen
 * rates and the cancellation, and without one it shows the day's allowance, which is not something
 * to infer from having no reservation.
 */
export function ReservationPanel({ resource, reservations, onShowVehicle }: ReservationPanelProps) {
  const snapshot = loadedValue(resource);
  if (snapshot === undefined) return null;

  const rental = currentRental(snapshot);
  const receivedAt = resource.phase === 'ready' || resource.phase === 'stale' ? resource.loadedAt : new Date();
  return (
    <section className="reservation-panel" aria-label={PANEL_HEADING}>
      <h2 className="reservation-panel-heading">{PANEL_HEADING}</h2>
      {rental === undefined ? (
        <p className="reservation-panel-empty">{NOTHING_CURRENT}</p>
      ) : (
        <ReservedRental
          rental={rental}
          reservations={reservations}
          serverTime={snapshot.server_time}
          receivedAt={receivedAt}
          onShowVehicle={onShowVehicle}
        />
      )}
      <p className="reservation-panel-limit">{limitText(snapshot)}</p>
      <UnknownCommand reservations={reservations} />
    </section>
  );
}

function ReservedRental({
  rental,
  reservations,
  serverTime,
  receivedAt,
  onShowVehicle,
}: {
  rental: Rental;
  reservations: Reservations;
  serverTime: string;
  receivedAt: Date;
  onShowVehicle: (vehicleId: string) => void;
}) {
  const [asking, setAsking] = useState(false);
  const deadline = reservationDeadline(rental, serverTime, receivedAt);
  const countdown = useCountdown(deadline);
  const sending = reservations.phase.state === 'sending' && reservations.phase.action === 'cancel';
  const notice = cancellingNotice(reservations);

  const rates = rateTextOf(rental.tariff_snapshot);
  return (
    <div className="reservation-panel-current">
      <p className="reservation-panel-vehicle">{vehicleName(rental)}</p>
      <p className="reservation-panel-time" role="status">
        {countdown === undefined ? RIDE_RUNNING : countdownText(countdown)}
      </p>
      <dl className="reservation-panel-rates">
        <dt>Движение</dt>
        <dd>{rates.driving ?? '—'} за начатую минуту</dd>
        <dt>Пауза</dt>
        <dd>{rates.paused ?? '—'} за начатую минуту</dd>
      </dl>
      {reservations.ratesChanged && <p className="reservation-panel-notice">{TARIFF_CHANGED}</p>}

      <div className="reservation-panel-actions">
        <button className="action-button" type="button" onClick={() => onShowVehicle(rental.vehicle.id)}>
          {GO_TO_VEHICLE}
        </button>
        <button className="action-button" type="button" disabled={sending} onClick={() => setAsking(true)}>
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

      {sending && <p className="reservation-panel-notice">{CANCEL_PENDING}</p>}
      {notice !== undefined && <p className="reservation-panel-notice">{notice}</p>}
    </div>
  );
}

/** What the panel says about the last cancellation, if it was the last command. */
function cancellingNotice(reservations: Reservations): string | undefined {
  const { phase } = reservations;
  if (phase.state === 'done' && phase.action === 'cancel') return CANCEL_CONFIRMED;
  if (phase.state === 'refused' && phase.action === 'cancel') return commandNotice(phase);

  return undefined;
}

/**
 * UnknownCommand offers to settle a command whose answer never arrived. It is the only control that
 * sends a command with a key that was already used: a new booking is refused while the outcome is
 * unknown, because a new key would hide what the stored one already did.
 */
function UnknownCommand({ reservations }: { reservations: Reservations }) {
  const held = reservations.repeatable;
  if (held === undefined) return null;

  return (
    <div className="reservation-panel-unknown" role="status">
      <p className="reservation-panel-notice">{UNKNOWN_COMMAND}</p>
      {withinRepeatWindow(held, Date.now()) ? (
        <button className="action-button" type="button" onClick={reservations.repeat}>
          {REPEAT_ACTION}
        </button>
      ) : (
        <p className="reservation-panel-notice">{REPEAT_EXPIRED}</p>
      )}
    </div>
  );
}

function useCountdown(deadline: Deadline | undefined): Countdown | undefined {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const repeat = window.setInterval(() => setNow(new Date()), TICK_MILLISECONDS);
    return () => window.clearInterval(repeat);
  }, []);

  return deadline === undefined ? undefined : countdownAt(deadline, now);
}

/**
 * The deadline a rental is counted down to, or nothing when it does not have one. Only a
 * reservation runs out: a ride that has started is counted in the tasks that own its commands, and
 * the panel says so rather than counting down to a moment that does not exist.
 */
function reservationDeadline(rental: Rental, serverTime: string, receivedAt: Date): Deadline | undefined {
  if (rental.state !== 'reserved') return undefined;
  return { expiresAt: rental.expires_at, serverTime, receivedAt };
}

function countdownText(countdown: Countdown): string {
  if (countdown.state === 'left') return `Осталось ${countdown.text}`;
  return EXPIRY_PENDING;
}
