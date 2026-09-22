import { useCallback, useState } from 'react';
import {
  clearUnfinished,
  repeatableCommand,
  storeUnfinished,
  type CommandAction,
  type UnfinishedCommand,
} from './unfinishedCommand.ts';
import type { CommandPhase } from './commandPhase.ts';
import type { CommandResult } from '../api/current.ts';

/** What one command attempt answers, whichever command was sent. */
export type CommandAnswer = CommandResult<unknown>;

/** What the browser knows about one command it has sent, and the way to send the next one. */
export type CommandSender = {
  /** Where the command the interface last sent stands. */
  phase: CommandPhase;

  /** The command a reload left unanswered, while its repeat window is still open. */
  repeatable: UnfinishedCommand | undefined;

  /** Sends one command with a key drawn now. Nothing is sent by the sender itself. */
  start: (action: CommandAction, parameters: UnfinishedCommand['parameters']) => void;

  /** Sends the stored command again with the key it was first sent with. */
  repeat: () => void;

  /** Forgets what the last command answered, so a message is not shown twice. */
  settle: () => void;
};

/**
 * What one command attempt is sent with, and how its answer becomes the state the interface holds.
 * The sender is told the session it may use, the account the command belongs to, the way to read the
 * state again, and how to send one attempt; everything else about a command is the same whether it
 * reserves a vehicle or moves a ride.
 */
export type CommandSenderOptions = {
  owner: string | undefined;
  csrfToken: string | undefined;
  refresh: () => void;
  send: (command: UnfinishedCommand, credentials: { csrfToken: string; key: string }) => Promise<CommandAnswer>;
};

/**
 * What one account's command was answered with, kept under the account it belongs to. The record of
 * a command is there only while an answer left one to send again: an answer that settled the command
 * forgets it, which is what the missing field says.
 */
type Answered = { owner: string | undefined; phase: CommandPhase; repeatable?: UnfinishedCommand };

/** What a reload of one account left to settle: the stored command, or nothing to settle at all. */
function answeredFor(owner: string | undefined): Answered {
  return {
    owner,
    phase: { state: 'idle' },
    repeatable: owner === undefined ? undefined : repeatableCommand(owner, Date.now()),
  };
}

/**
 * The state one answered command leaves behind. A command the server carried out or refused is
 * settled and forgotten; one it never answered — a lost response, an ended session — is kept so that
 * a person can send it again with the key it was first sent with.
 */
function answeredBy(owner: string | undefined, command: UnfinishedCommand, answer: CommandAnswer): Answered {
  switch (answer.outcome) {
    case 'done':
      clearUnfinished();
      return { owner, phase: { state: 'done', action: command.action, replayed: answer.replayed } };
    case 'refused':
      clearUnfinished();
      return { owner, phase: { state: 'refused', action: command.action, code: answer.code } };
    case 'signed-out':
      return { owner, phase: { state: 'signed-out' }, repeatable: command };
    default:
      return { owner, phase: { state: 'unknown', action: command.action }, repeatable: command };
  }
}

/** Whether an answer is one the interface reads the state again for, which a repeat never is. */
function asksForAnotherRead(answer: CommandAnswer): boolean {
  return answer.outcome === 'done' || answer.outcome === 'refused';
}

/**
 * useCommandSender sends one command at a time and keeps what the browser knows about it. A command
 * is stored before it is sent and forgotten the moment its outcome is known, so a reload or a lost
 * answer leaves an outcome that can still be settled rather than a guess.
 *
 * Nothing here is sent on its own: a reconnection, a sign-in, a new day and a stored command all wait
 * for a person to ask. What a command did is read again from the server afterwards, because the
 * interface shows what the server holds rather than what it hoped the command did.
 *
 * A command is sent once it is asked for and its answer is applied where it arrives, so the asker
 * waits for nothing: every control of the interface stays responsive while its command is on its way.
 *
 * What the browser remembers belongs to one account, and it is keyed by the account rather than
 * cleared by an effect: the state of the account on screen is the state of the account it was
 * answered for, so a render after a change of account never shows the previous one's phase.
 */
export function useCommandSender(options: CommandSenderOptions): CommandSender {
  const { owner, csrfToken, refresh, send: attempt } = options;
  const [held, setHeld] = useState<Answered>(() => answeredFor(owner));

  const current = held.owner === owner ? held : answeredFor(owner);
  const { phase, repeatable } = current;

  const send = useCallback(
    async (command: UnfinishedCommand) => {
      if (csrfToken === undefined) {
        setHeld({ ...current, phase: { state: 'signed-out' } });
        return;
      }

      setHeld({ ...current, phase: { state: 'sending', action: command.action } });
      storeUnfinished(command);

      const answer = await attempt(command, { csrfToken, key: command.key });
      setHeld(answeredBy(owner, command, answer));

      // Whatever the answer was, the interface reads the state again rather than reasoning about it:
      // a repeat carries the moment of the original command, which is not the current one.
      if (asksForAnotherRead(answer)) refresh();
    },
    [attempt, csrfToken, owner, refresh, current],
  );

  const start = useCallback(
    (action: CommandAction, parameters: UnfinishedCommand['parameters']) => {
      if (owner === undefined) {
        setHeld({ ...current, phase: { state: 'signed-out' } });
        return;
      }
      void send({ owner, action, parameters, key: crypto.randomUUID(), sentAt: Date.now() });
    },
    [owner, send, current],
  );

  const repeat = useCallback(() => {
    if (repeatable !== undefined) void send(repeatable);
  }, [repeatable, send]);

  const settle = useCallback(() => setHeld({ ...current, phase: { state: 'idle' } }), [current]);

  return { phase, repeatable, start, repeat, settle };
}
