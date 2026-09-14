import { useState } from 'react';
import type { LiveRental } from '../../shared/api/current.ts';
import type { ServerClock } from './countdown.ts';
import { commandText, type CommandPhase } from './commandPhase.ts';
import { GO_TO_VEHICLE, rateTextOf, vehicleName } from './reservationCopy.ts';
import {
  amountText,
  DRIVING_TOTAL,
  ESTIMATED_AMOUNT,
  FINISH_ACTION,
  FINISH_QUESTION,
  FINISH_WARNING,
  IN_MODE,
  KEEP_RIDING_ACTION,
  PAUSE_ACTION,
  PAUSED_TOTAL,
  RESUME_ACTION,
  UNREADABLE_VALUE,
} from './rideCopy.ts';
import {
  drivingDuration,
  durationText,
  elapsedInMode,
  hasStarted,
  pausedDuration,
  rideModeOf,
  rideModeText,
} from './ridePace.ts';
import { TariffRates } from './TariffRates.tsx';
import { useClockTick } from './useClockTick.ts';
import type { RideCommands } from './useRideCommands.ts';
import type { CommandAction } from './unfinishedCommand.ts';

type RideViewProps = {
  rental: LiveRental;

  ride: RideCommands;

  /** The moment the answer that carries this rental was computed at, and when it arrived here. */
  clock: ServerClock;

  onShowVehicle: (vehicleId: string) => void;
};

/**
 * RideView is what a person reads about the ride they are on: the vehicle, the mode it is in and how
 * long it has been in it, what the ride has cost so far at the rates of its own snapshot, and the
 * controls that hold the ride, carry it on and end it. A rental that has not started a ride has
 * nothing to show here, and says so by showing nothing.
 *
 * Nothing here remembers the ride. The mode and the durations come from the answer the server gave,
 * the running value is measured against the server's own moment, and a client that comes back to a
 * closed tab therefore shows the ride as the database holds it rather than as it was left.
 */
export function RideView({ rental, ride, clock, onShowVehicle }: RideViewProps) {
  const [ending, setEnding] = useState(false);
  const now = useClockTick();
  if (!hasStarted(rental)) return null;

  const mode = rideModeOf(rental);
  const notice = commandText(ride.phase);
  return (
    <div className="reservation-panel-current">
      <p className="reservation-panel-vehicle">{vehicleName(rental)}</p>
      <p className="reservation-panel-time" role="status">
        {`${IN_MODE}: ${rideModeText(mode)} · ${modeTime(rental.mode_started_at, clock, now)}`}
      </p>

      <dl className="ride-progress">
        <dt>{DRIVING_TOTAL}</dt>
        <dd>{elapsedText(drivingDuration(rental.progress))}</dd>
        <dt>{PAUSED_TOTAL}</dt>
        <dd>{elapsedText(pausedDuration(rental.progress))}</dd>
        <dt>{ESTIMATED_AMOUNT}</dt>
        <dd>{amountText(rental.progress)}</dd>
      </dl>

      <TariffRates rates={rateTextOf(rental.tariff_snapshot)} />

      <div className="reservation-panel-actions">
        <button className="action-button" type="button" onClick={() => onShowVehicle(rental.vehicle.id)}>
          {GO_TO_VEHICLE}
        </button>
        {mode === 'driving' ? (
          <button
            className="action-button"
            type="button"
            disabled={running(ride.phase, 'pause')}
            onClick={() => ride.hold(rental.id)}
          >
            {PAUSE_ACTION}
          </button>
        ) : (
          <button
            className="action-button"
            type="button"
            disabled={running(ride.phase, 'resume')}
            onClick={() => ride.carryOn(rental.id)}
          >
            {RESUME_ACTION}
          </button>
        )}
        <button
          className="action-button"
          type="button"
          disabled={running(ride.phase, 'finish')}
          onClick={() => setEnding(true)}
        >
          {FINISH_ACTION}
        </button>
      </div>

      {ending && (
        <div className="reservation-panel-confirm" role="group" aria-label={FINISH_QUESTION}>
          <p className="reservation-panel-question">{FINISH_QUESTION}</p>
          <p className="reservation-panel-warning">{FINISH_WARNING}</p>
          <button
            className="action-button"
            type="button"
            onClick={() => {
              setEnding(false);
              ride.finish(rental.id);
            }}
          >
            {FINISH_ACTION}
          </button>
          <button className="action-button" type="button" onClick={() => setEnding(false)}>
            {KEEP_RIDING_ACTION}
          </button>
        </div>
      )}

      {notice !== undefined && <p className="reservation-panel-notice">{notice}</p>}
    </div>
  );
}

/** Whether one ride command is on its way to the server right now. */
function running(phase: CommandPhase, action: CommandAction): boolean {
  return phase.state === 'sending' && phase.action === action;
}

/** How long the ride has been in its mode, or that the moments it is measured between cannot be read. */
function modeTime(modeStartedAt: string, clock: ServerClock, now: Date): string {
  const elapsed = elapsedInMode({ modeStartedAt, ...clock }, now);

  return elapsed === undefined ? UNREADABLE_VALUE : elapsedText(elapsed.toString());
}

/** One published duration as minutes and seconds, or as missing when it cannot be read. */
function elapsedText(microseconds: string): string {
  return durationText(microseconds) ?? UNREADABLE_VALUE;
}
