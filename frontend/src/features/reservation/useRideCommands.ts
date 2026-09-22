import { useCallback, useState } from 'react';
import { identityOf } from '../../shared/account/identity.ts';
import type { Account } from '../../shared/account/session.ts';
import type { FinishResult } from '../../shared/api/current.ts';
import { finishRide, pauseRide, resumeRide, startRide } from '../../shared/api/rides.ts';
import { useCommandSender, type CommandAnswer } from '../../shared/command/commandSender.ts';
import type { CommandPhase } from '../../shared/command/commandPhase.ts';
import type { UnfinishedCommand } from '../../shared/command/unfinishedCommand.ts';
import type { CurrentRental } from './useCurrentRental.ts';

/** What the interface can do about the ride in force, and where its last command stands. */
export type RideCommands = {
  /** Where the ride command the interface last sent stands. */
  phase: CommandPhase;

  /** The command a reload left unanswered, while its repeat window is still open. */
  repeatable: UnfinishedCommand | undefined;

  /**
   * What the service answered the finish this tab sent. It is held in memory for the moment between
   * that answer and the report of the same ending arriving from the service, so a person reads the
   * truth this tab was already told rather than an empty panel. A reload in that moment reads the
   * report instead, which the same transaction wrote.
   */
  finished: FinishResult | undefined;

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

/** What this tab was told about an ending, kept under the account that rode. */
type AnsweredFinish = { owner: string | undefined; finished: FinishResult | undefined };

/**
 * useRideCommands sends the commands that move a rental through its ride. It is the sender the
 * reservation uses, told a different set of commands: the same key is kept before the request, the
 * same key is presented by a repeat, and the current rental is read again after every answer.
 *
 * What this tab was told about an ending belongs to the account that rode, and it is keyed by the
 * account rather than cleared by an effect: the next account starts with nothing of the previous one
 * on screen, from the first render it is shown in.
 *
 * Closing the tab changes nothing here. The ride lives in the database, and a client that comes back
 * reads it rather than sending a command about it.
 */
export function useRideCommands(account: Account, current: CurrentRental): RideCommands {
  const { session, csrfToken } = identityOf(account);
  const [answered, setAnswered] = useState<AnsweredFinish>(() => ({ owner: session, finished: undefined }));

  const finished = answered.owner === session ? answered.finished : undefined;
  const keep = useCallback((result: FinishResult) => setAnswered({ owner: session, finished: result }), [session]);

  const send = useCallback(
    (command: UnfinishedCommand, credentials: { csrfToken: string; key: string }) =>
      rideCommand(command, credentials, keep),
    [keep],
  );

  const { phase, repeatable, start, repeat, settle } = useCommandSender({
    owner: session,
    csrfToken,
    refresh: current.retry,
    send,
  });

  return {
    phase,
    repeatable,
    finished,
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
  keep: (finished: FinishResult) => void,
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
      return finishedRide(rentalId, credentials, keep);
    default:
      return { outcome: 'unknown' };
  }
}

/**
 * finishedRide ends the ride and keeps the answer when the service confirmed it. The answer is kept
 * by the handler that received it rather than by the render that shows it, because a render React
 * discards must not leave a value behind that no screen ever showed.
 */
async function finishedRide(
  rentalId: string,
  credentials: { csrfToken: string; key: string },
  keep: (finished: FinishResult) => void,
): Promise<CommandAnswer> {
  const answer = await finishRide(rentalId, credentials);
  if (answer.outcome === 'done') keep(answer.answer);

  return answer;
}
