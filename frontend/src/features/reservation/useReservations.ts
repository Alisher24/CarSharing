import { useCallback, useEffect, useMemo, useState } from 'react';
import type { Account } from '../account/useAccount.ts';
import type { ApiError } from '../../shared/api/current.ts';
import { cancelReservation, reserveVehicle } from '../../shared/api/current.ts';
import { loadedValue } from '../../shared/api/Resource.ts';
import { currentRental, rateTextOf, sameRates, type RateText } from './reservationCopy.ts';
import {
  clearUnfinished,
  repeatableCommand,
  storeUnfinished,
  type CommandAction,
  type UnfinishedCommand,
} from './unfinishedCommand.ts';
import type { CurrentRental } from './useCurrentRental.ts';

/**
 * Where one command stands. The outcomes are kept apart because a person's next step differs: a
 * refusal is answered by reading the state again, and an answer that never arrived leaves the
 * outcome unknown until the command is repeated with the key it was sent with.
 */
export type CommandPhase =
  | { state: 'idle' }
  | { state: 'sending'; action: CommandAction }
  | { state: 'done'; action: CommandAction; replayed: boolean }
  | { state: 'refused'; action: CommandAction; code: ApiError['code'] }
  | { state: 'unknown'; action: CommandAction }
  | { state: 'signed-out' };

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
 * them. A command is stored before it is sent and forgotten the moment its outcome is known, so a
 * reload or a lost answer leaves an outcome that can still be settled rather than a guess.
 *
 * Nothing here is sent on its own: a reconnection, a sign-in, a new day and a stored command all
 * wait for a person to ask. What the commands did is read again from the server afterwards, because
 * the interface shows what the server holds rather than what it hoped the command did.
 */
export function useReservations(account: Account, current: CurrentRental): Reservations {
  const [phase, setPhase] = useState<CommandPhase>({ state: 'idle' });
  const [repeatable, setRepeatable] = useState<UnfinishedCommand | undefined>(undefined);
  const [confirmedRates, setConfirmedRates] = useState<RateText | undefined>(undefined);

  const owner = account.state === 'signed-in' ? account.snapshot.user.id : undefined;
  const csrfToken = account.state === 'signed-in' ? account.snapshot.csrf_token : undefined;
  const refresh = current.retry;

  // What the browser remembers belongs to one account: another one starts with nothing to settle,
  // and the record of the previous account is neither shown nor sent.
  useEffect(() => {
    setPhase({ state: 'idle' });
    setRepeatable(owner === undefined ? undefined : repeatableCommand(owner, Date.now()));
  }, [owner]);

  const send = useCallback(
    async (command: UnfinishedCommand) => {
      if (csrfToken === undefined) {
        setPhase({ state: 'signed-out' });
        return;
      }

      setPhase({ state: 'sending', action: command.action });
      storeUnfinished(command);

      const credentials = { csrfToken, key: command.key };
      const answer =
        command.action === 'reserve'
          ? await reserveVehicle(command.parameters.vehicleId ?? '', credentials)
          : await cancelReservation(command.parameters.rentalId ?? '', credentials);

      // Whatever the answer was, the interface reads the reservation again rather than reasoning
      // about it: a repeat carries the moment of the original command, which is not the current one.
      switch (answer.outcome) {
        case 'done':
          forget();
          setPhase({ state: 'done', action: command.action, replayed: answer.replayed });
          refresh();
          return;
        case 'refused':
          forget();
          setPhase({ state: 'refused', action: command.action, code: answer.code });
          refresh();
          return;
        case 'signed-out':
          setRepeatable(command);
          setPhase({ state: 'signed-out' });
          return;
        default:
          setRepeatable(command);
          setPhase({ state: 'unknown', action: command.action });
      }

      function forget(): void {
        clearUnfinished();
        setRepeatable(undefined);
      }
    },
    [csrfToken, refresh],
  );

  const book = useCallback(
    (vehicleId: string, shownRates: RateText) => {
      if (owner === undefined) {
        setPhase({ state: 'signed-out' });
        return;
      }
      // What the person agreed to is remembered, so the reservation that comes back can be compared
      // with it: a catalogue that moved between the two is said out loud rather than shown as if
      // nothing had happened.
      setConfirmedRates(shownRates);
      void send(freshCommand('reserve', owner, { vehicleId }));
    },
    [owner, send],
  );

  const cancel = useCallback(
    (rentalId: string) => {
      if (owner === undefined) {
        setPhase({ state: 'signed-out' });
        return;
      }
      void send(freshCommand('cancel', owner, { rentalId }));
    },
    [owner, send],
  );

  const repeat = useCallback(() => {
    if (repeatable !== undefined) void send(repeatable);
  }, [repeatable, send]);

  const settle = useCallback(() => setPhase({ state: 'idle' }), []);

  // The conditions the reservation was made under are the ones it stores, so they are compared
  // against what a person agreed to rather than against the catalogue as it stands now.
  const ratesChanged = useMemo(() => {
    const rental = currentRental(loadedValue(current.resource));
    if (rental === undefined || confirmedRates === undefined) return false;

    return !sameRates(confirmedRates, rateTextOf(rental.tariff_snapshot));
  }, [confirmedRates, current.resource]);

  return useMemo(
    () => ({ phase, repeatable, ratesChanged, book, cancel, repeat, settle }),
    [phase, repeatable, ratesChanged, book, cancel, repeat, settle],
  );
}

/**
 * freshCommand is one new command: the key is drawn here, once, and the record keeps it so that a
 * repeat presents the key of the attempt that was actually made.
 */
function freshCommand(
  action: CommandAction,
  owner: string,
  parameters: UnfinishedCommand['parameters'],
): UnfinishedCommand {
  return { owner, action, parameters, key: crypto.randomUUID(), sentAt: Date.now() };
}

/** Whether one command's outcome is still unknown and may be settled by repeating it. */
export function awaitsRepeat(phase: CommandPhase): boolean {
  return phase.state === 'unknown';
}
