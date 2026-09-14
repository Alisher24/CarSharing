import { useCallback, useEffect, useState } from 'react';
import {
  clearUnfinished,
  repeatableCommand,
  storeUnfinished,
  type CommandAction,
  type UnfinishedCommand,
} from './unfinishedCommand.ts';
import type { CommandPhase } from './commandPhase.ts';
import type { CommandResult } from '../../shared/api/current.ts';

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
 */
export function useCommandSender(options: CommandSenderOptions): CommandSender {
  const [phase, setPhase] = useState<CommandPhase>({ state: 'idle' });
  const [repeatable, setRepeatable] = useState<UnfinishedCommand | undefined>(undefined);

  const { owner, csrfToken, refresh, send: attempt } = options;

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

      const answer = await attempt(command, { csrfToken, key: command.key });

      // Whatever the answer was, the interface reads the state again rather than reasoning about it:
      // a repeat carries the moment of the original command, which is not the current one.
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
    [attempt, csrfToken, refresh],
  );

  const start = useCallback(
    (action: CommandAction, parameters: UnfinishedCommand['parameters']) => {
      if (owner === undefined) {
        setPhase({ state: 'signed-out' });
        return;
      }
      void send(freshCommand(action, owner, parameters));
    },
    [owner, send],
  );

  const repeat = useCallback(() => {
    if (repeatable !== undefined) void send(repeatable);
  }, [repeatable, send]);

  const settle = useCallback(() => setPhase({ state: 'idle' }), []);

  return { phase, repeatable, start, repeat, settle };
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
