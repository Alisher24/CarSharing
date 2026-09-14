import type { Account } from '../account/useAccount.ts';
import type { CurrentRental } from './useCurrentRental.ts';
import { finishRide, pauseRide, resumeRide, startRide } from '../../shared/api/rides.ts';
import { useCommandSender, type CommandAnswer } from './commandSender.ts';
import { storeCompletedRide } from './completedRide.ts';
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

  /** Ends the ride and issues the invoice for it. */
  finish: (rentalId: string) => void;

  /** Sends the stored command again with the key it was first sent with. */
  repeat: () => void;

  /** Forgets what the last command answered, so a message is not shown twice. */
  settle: () => void;
};

/**
 * useRideCommands sends the commands that move a rental through its ride. It is the sender the
 * reservation uses, told a different set of commands: the same key is kept before the request, the
 * same key is presented by a repeat, and the current rental is read again after every answer.
 *
 * A ride that is ended is no longer current, so the answer to that one command is also what the panel
 * shows about it afterwards: the ending is written down by the handler that received it, because a
 * render that React discards must not be what a person's finished ride was remembered by.
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
    send: (command, credentials) => rideCommand(command, credentials, owner),
  });

  return {
    phase,
    repeatable,
    begin: (rentalId) => void start('start', { rentalId }),
    hold: (rentalId) => void start('pause', { rentalId }),
    carryOn: (rentalId) => void start('resume', { rentalId }),
    finish: (rentalId) => void start('finish', { rentalId }),
    repeat,
    settle,
  };
}

/** One ride command, which names the ride it moves and asks for the transition it makes. */
async function rideCommand(
  command: UnfinishedCommand,
  credentials: { csrfToken: string; key: string },
  owner: string | undefined,
): Promise<CommandAnswer> {
  const rentalId = command.parameters.rentalId ?? '';
  switch (command.action) {
    case 'start':
      return startRide(rentalId, credentials);
    case 'pause':
      return pauseRide(rentalId, credentials);
    case 'resume':
      return resumeRide(rentalId, credentials);
    case 'finish':
      return finishedRide(rentalId, credentials, owner);
    default:
      return { outcome: 'unknown' };
  }
}

/**
 * finishedRide ends the ride and keeps the answer when the service confirmed it. The record is written
 * here, where the answer arrived, rather than by the render that shows it: what a person sees after a
 * reload has to be something a handler wrote down.
 */
async function finishedRide(
  rentalId: string,
  credentials: { csrfToken: string; key: string },
  owner: string | undefined,
): Promise<CommandAnswer> {
  const answer = await finishRide(rentalId, credentials);
  if (answer.outcome === 'done' && owner !== undefined) {
    storeCompletedRide(owner, answer.answer, Date.now());
  }

  return answer;
}
