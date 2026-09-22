import { useCallback, useMemo, useState } from 'react';
import { cancelReservation, reserveVehicle } from '../../shared/api/current.ts';
import { identityOf } from '../../shared/account/identity.ts';
import type { Account } from '../../shared/account/session.ts';
import { useCommandSender, type CommandAnswer } from '../../shared/command/commandSender.ts';
import type { CommandPhase } from '../../shared/command/commandPhase.ts';
import type { UnfinishedCommand } from '../../shared/command/unfinishedCommand.ts';
import { loadedValue } from '../../shared/read/Resource.ts';
import { currentRental } from '../../shared/ride/serverClock.ts';
import { rateTextOf, sameRates, type RateText } from '../../shared/ride/fares.ts';
import type { CurrentRental } from './useCurrentRental.ts';

/** What the interface can do about the account's reservation, and where its last command stands. */
export type Reservations = {
  phase: CommandPhase;

  /** The command a reload left unanswered, while its repeat window is still open. */
  repeatable: UnfinishedCommand | undefined;

  /**
   * Whether the reservation in force was made under conditions other than the ones a person agreed
   * to. The interface says so, shows what the reservation actually costs and offers the
   * cancellation, which costs nothing.
   */
  ratesChanged: boolean;

  /** Books one vehicle under a new command key, having shown the rates the person agreed to. */
  book: (vehicleId: string, shownRates: RateText) => void;

  /** Gives one reservation back under a new command key. */
  cancel: (rentalId: string) => void;

  /** Sends the stored command again with the key it was first sent with. */
  repeat: () => void;

  /** Forgets what the last command answered, so a message is not shown twice. */
  settle: () => void;
};

/**
 * useReservations sends the two commands of a reservation and keeps what the browser knows about
 * them. It answers with the phase and the stored command the sender holds, and adds what only a
 * reservation has: the conditions a person agreed to, which the command that follows is compared
 * with.
 *
 * The two controls it offers are declared once, so a render that changes nothing else does not hand
 * a new control to every button below it; what the sender itself states is read from the sender.
 */
export function useReservations(account: Account, current: CurrentRental): Reservations {
  const { session, csrfToken } = identityOf(account);
  const [confirmedRates, setConfirmedRates] = useState<RateText | undefined>(undefined);

  const commands = useCommandSender({ owner: session, csrfToken, refresh: current.retry, send: reservationCommand });
  const start = commands.start;

  const book = useCallback(
    (vehicleId: string, shownRates: RateText) => {
      // What the person agreed to is remembered, so the reservation that comes back can be compared
      // with it: a catalogue that moved between the two is said out loud rather than shown as if
      // nothing had happened.
      setConfirmedRates(shownRates);
      void start('reserve', { vehicleId });
    },
    [start],
  );

  const cancel = useCallback((rentalId: string) => void start('cancel', { rentalId }), [start]);

  // The conditions the reservation was made under are the ones it stores, so they are compared
  // against what a person agreed to rather than against the catalogue as it stands now.
  const ratesChanged = useMemo(() => {
    const rental = currentRental(loadedValue(current.resource));
    if (rental === undefined || confirmedRates === undefined) return false;

    return !sameRates(confirmedRates, rateTextOf(rental.tariff_snapshot));
  }, [confirmedRates, current.resource]);

  return {
    phase: commands.phase,
    repeatable: commands.repeatable,
    ratesChanged,
    book,
    cancel,
    repeat: commands.repeat,
    settle: commands.settle,
  };
}

/** One reservation command, which names the vehicle to book or the reservation to give back. */
function reservationCommand(
  command: UnfinishedCommand,
  credentials: { csrfToken: string; key: string },
): Promise<CommandAnswer> {
  if (command.action === 'reserve') return reserveVehicle(command.parameters.vehicleId ?? '', credentials);

  return cancelReservation(command.parameters.rentalId ?? '', credentials);
}
