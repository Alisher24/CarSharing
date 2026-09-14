import type { Account } from '../account/useAccount.ts';
import type { CurrentRental } from './useCurrentRental.ts';
import { pauseRide, resumeRide, startRide } from '../../shared/api/rides.ts';
import { useCommandSender, type CommandAnswer } from './commandSender.ts';
import type { UnfinishedCommand } from './unfinishedCommand.ts';
import type { CommandPhase } from './commandPhase.ts';

/** What the interface can do about the ride in force, and where its last command stands. */
export type RideCommands = {
  /** Where the ride command the interface last sent stands. */
  phase: CommandPhase;

  /** The command a reload left unanswered, while its repeat window is still open. */
  repeatable: UnfinishedCommand | undefined;

  /** Starts the ride the reservation is waiting for. */
  begin: (rentalId: string) => void;

  /** Holds the ride where it is. */
  hold: (rentalId: string) => void;

  /** Carries the ride on after a pause. */
  carryOn: (rentalId: string) => void;

  /** Sends the stored command again with the key it was first sent with. */
  repeat: () => void;

  /** Forgets what the last command answered, so a message is not shown twice. */
  settle: () => void;
};

/**
 * useRideCommands sends the three commands that move a rental through its ride. It is the sender the
 * reservation uses, told a different set of commands: the same key is kept before the request, the
 * same key is presented by a repeat, and the current rental is read again after every answer.
 *
 * Closing the tab changes nothing here. The ride lives in the database, and a client that comes back
 * reads it rather than sending a command about it.
 */
export function useRideCommands(account: Account, current: CurrentRental): RideCommands {
  const owner = account.state === 'signed-in' ? account.snapshot.user.id : undefined;
  const csrfToken = account.state === 'signed-in' ? account.snapshot.csrf_token : undefined;

  const { phase, repeatable, start, repeat, settle } = useCommandSender({
    owner,
    csrfToken,
    refresh: current.retry,
    send: rideCommand,
  });

  return {
    phase,
    repeatable,
    begin: (rentalId) => void start('start', { rentalId }),
    hold: (rentalId) => void start('pause', { rentalId }),
    carryOn: (rentalId) => void start('resume', { rentalId }),
    repeat,
    settle,
  };
}

/** One ride command, which names the ride it moves and asks for the transition it makes. */
function rideCommand(
  command: UnfinishedCommand,
  credentials: { csrfToken: string; key: string },
): Promise<CommandAnswer> {
  const rentalId = command.parameters.rentalId ?? '';
  switch (command.action) {
    case 'start':
      return startRide(rentalId, credentials);
    case 'pause':
      return pauseRide(rentalId, credentials);
    case 'resume':
      return resumeRide(rentalId, credentials);
    default:
      return Promise.resolve({ outcome: 'unknown' });
  }
}
