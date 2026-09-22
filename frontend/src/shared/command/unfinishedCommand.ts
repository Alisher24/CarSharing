/**
 * What the browser remembers about a command it sent and could not confirm: who sent it, what it
 * asked for, the key it was sent with, and when it was first sent. Nothing else is kept — no token,
 * no price, no answer — because the point of the record is only to repeat that one command with the
 * key it already had.
 */

/**
 * The actions a stored record may name, which is what makes a record readable at all. The list is
 * the one declaration: the type is read from it, so an action that is added here is one the reader
 * accepts and the writer may store, and neither can be changed without the other.
 */
export const COMMAND_ACTIONS = ['reserve', 'cancel', 'start', 'pause', 'resume', 'finish', 'pay'] as const;

/** The action a stored command asked for. */
export type CommandAction = (typeof COMMAND_ACTIONS)[number];

/**
 * One command whose outcome the browser does not know. It is written before the request is sent, so
 * a reload while the answer is in flight still knows what to ask about.
 */
export type UnfinishedCommand = {
  /** The account the command belongs to, so it is never carried to another one. */
  owner: string;

  action: CommandAction;

  /** What the command named: the vehicle it asked for, the rental or the invoice it acted on. */
  parameters: { vehicleId?: string; rentalId?: string; invoiceId?: string };

  /** The key of the original attempt, which is the only key a repeat may present. */
  key: string;

  /** When the command was first sent, which is what its repeat window is measured from. */
  sentAt: number;
};

/** How long a stored command may still be repeated. Past it the state is read instead. */
export const REPEAT_WINDOW_MILLISECONDS = 86_400 * 1_000;

/** Where the record is kept: the storage of one tab, cleared when that tab is closed. */
const STORAGE_KEY = 'carsharing.unfinished-command';

/** The part of a storage this module uses, so a test can supply its own. */
export type CommandStorage = {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
  removeItem: (key: string) => void;
};

/**
 * storeUnfinished records the command before it is sent. A browser that cannot write it — a private
 * window with storage disabled — is not an error: the command is still sent, and only the ability to
 * repeat it after a reload is lost.
 */
export function storeUnfinished(command: UnfinishedCommand, storage = browserStorage()): void {
  try {
    storage?.setItem(STORAGE_KEY, JSON.stringify(command));
  } catch {
    // Storage that refuses the write costs the repeat, never the command.
  }
}

/** clearUnfinished forgets a command whose outcome is known. */
export function clearUnfinished(storage = browserStorage()): void {
  try {
    storage?.removeItem(STORAGE_KEY);
  } catch {
    // A record that cannot be removed is stale, not dangerous: it is refused below.
  }
}

/**
 * repeatableCommand answers the command one account may still repeat, or undefined when there is
 * none. A record of another account, a record past its window and a record that cannot be read are
 * all answered as no command at all: the interface reads the current state instead of offering a
 * repeat that the server would refuse or that belongs to somebody else.
 */
export function repeatableCommand(
  owner: string,
  now: number,
  storage = browserStorage(),
): UnfinishedCommand | undefined {
  const held = readUnfinished(storage);
  if (held === undefined) return undefined;
  if (held.owner !== owner) return undefined;
  if (now - held.sentAt >= REPEAT_WINDOW_MILLISECONDS) return undefined;

  return held;
}

/** Whether the moment a command was sent is still inside the window a repeat may be sent in. */
export function withinRepeatWindow(command: UnfinishedCommand, now: number): boolean {
  return now - command.sentAt < REPEAT_WINDOW_MILLISECONDS;
}

function readUnfinished(storage: CommandStorage | undefined): UnfinishedCommand | undefined {
  let raw: string | null = null;
  try {
    raw = storage?.getItem(STORAGE_KEY) ?? null;
  } catch {
    return undefined;
  }
  if (raw === null) return undefined;

  try {
    const parsed = JSON.parse(raw) as UnfinishedCommand;
    if (typeof parsed?.owner !== 'string' || typeof parsed.key !== 'string') return undefined;
    if (typeof parsed.sentAt !== 'number') return undefined;
    if (!COMMAND_ACTIONS.includes(parsed.action)) return undefined;

    return parsed;
  } catch {
    return undefined;
  }
}

function browserStorage(): CommandStorage | undefined {
  try {
    return window.sessionStorage;
  } catch {
    return undefined;
  }
}
