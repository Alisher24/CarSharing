import { highestVersion, isNewerVersion } from './version.ts';
import type { ObservedVersions } from './changes.ts';
import type { AnswerHandlers, DocumentKind } from './readCycle.ts';

/** Every public resource the catalog reads, in the order the interface shows them. */
export const DOCUMENT_KINDS: readonly DocumentKind[] = ['vehicles', 'zones', 'tariffs'];

/** A read the coordinator has opened, and which answer is allowed to store. */
export type ReadTicket = { document: DocumentKind; session: string; generation: number };

/**
 * ReadCoordinator is what a client must remember between a change signal and the REST answer that
 * covers it, and it is what decides whether an answer may still be stored. It knows nothing about
 * fetching: a hook asks whether to read and opens the read, and hands the answer back with the
 * ticket that read was given.
 */
export type ReadCoordinator = {
  /** Whether there is anything to read: a change nobody has answered for yet, or a request. */
  due: (document: DocumentKind) => boolean;

  /** Opens a read of one document, and answers nothing when one is already running. */
  begin: (document: DocumentKind) => ReadTicket | undefined;

  /** Whether this ticket's answer may be stored, given what has been read meanwhile. */
  accepts: (ticket: ReadTicket) => boolean;

  /** Records that an accepted answer was stored, after which an earlier read's answer is stale. */
  stored: (ticket: ReadTicket) => void;

  /** That a read of this document has ended, whichever way it ended, so it may be read again. */
  settle: (document: DocumentKind) => void;

  /** Keeps the versions one answer covers. */
  cover: (document: DocumentKind, covered: ObservedVersions) => void;

  /** Remembers one change, which is read whether or not a read is already in flight. */
  observe: (document: DocumentKind, id: string, version: string) => void;

  /** Asks for the given documents to be read, which is how a ready frame is answered. */
  request: (documents: readonly DocumentKind[]) => void;

  /** Asks for one document to be read even if nothing has changed in it. */
  force: (document: DocumentKind) => void;

  /** How many reads of one document have been asked for, which is what its reader watches. */
  asked: (document: DocumentKind) => number;

  /** Calls the listener whenever a read of one document is asked for, and stops when told to. */
  watch: (document: DocumentKind, listener: () => void) => () => void;

  /** The handlers this document's answers are given to. */
  answer: (document: DocumentKind) => AnswerHandlers;
};

/** What a coordinator remembers for one document. */
type DocumentState = {
  readable: boolean;
  waiting: boolean;
  requested: boolean;
  observed: ObservedVersions;
  covered: ObservedVersions;
  generation: number;
  answeredBy: number;
  asked: number;
  listeners: Set<() => void>;
};

/**
 * createReadCoordinator builds the memory of one session. A new session starts with nothing
 * observed, covered or answered, so no answer of the previous one can be stored.
 */
export function createReadCoordinator(session: string, readers: Record<DocumentKind, AnswerHandlers>): ReadCoordinator {
  const remembered = new Map<DocumentKind, DocumentState>();

  const documentOf = (document: DocumentKind): DocumentState => {
    const state = remembered.get(document) ?? freshDocument();
    remembered.set(document, state);
    return state;
  };

  return {
    due: (document) => {
      const state = documentOf(document);
      if (state.waiting || !state.readable) return false;
      if (state.requested || holdsChanges(state)) return true;

      // A resource that has never answered has everything to read, which is what keeps a first
      // attempt that failed from being the last one.
      return state.answeredBy === 0;
    },
    begin: (document) => {
      const state = documentOf(document);
      if (state.waiting) return undefined;

      state.readable = true;
      state.waiting = true;
      state.generation += 1;
      state.requested = false;
      return { document, session, generation: state.generation };
    },
    accepts: (ticket) => {
      const state = documentOf(ticket.document);
      if (ticket.session !== session) return false;
      if (ticket.generation !== state.generation) return false;

      // A read answers once. A second answer to the same read is a duplicate, and an answer to a
      // read opened earlier answers a question a later read has already answered.
      return state.answeredBy !== ticket.generation;
    },
    stored: (ticket) => {
      // From here, an answer to a read opened before this one answers a question that has already
      // been answered, and storing it would put an older reading over a newer one.
      documentOf(ticket.document).answeredBy = ticket.generation;
    },
    settle: (document) => {
      documentOf(document).waiting = false;
    },
    cover: (document, covered) => {
      documentOf(document).covered = covered;
    },
    observe: (document, id, version) => {
      const state = documentOf(document);
      if (!isNewerVersion(version, state.observed.get(id) ?? '')) return;

      state.observed = new Map(state.observed).set(id, highestVersion(state.observed.get(id) ?? version, version));
    },
    request: (documents) => {
      for (const document of documents) askFor(documentOf(document));
    },
    force: (document) => {
      askFor(documentOf(document));
    },
    asked: (document) => documentOf(document).asked,
    watch: (document, listener) => {
      const state = documentOf(document);
      state.listeners.add(listener);
      return () => state.listeners.delete(listener);
    },
    answer: (document) => readers[document],
  };
}

/**
 * askFor asks for one document, and declares that the catalog reads it. Asking is what makes
 * reconciliation cover a resource: a ready frame answers for every resource the catalog holds,
 * whether or not a screen has asked for that resource yet.
 *
 * Asking is also what a reader is woken by: the reader watches how many reads have been asked for,
 * so a request that arrives while an answer is still on its way starts the read that answer will
 * not cover rather than waiting for the next one.
 */
function askFor(state: DocumentState): void {
  state.requested = true;
  state.readable = true;
  state.asked += 1;
  for (const listener of state.listeners) listener();
}

function freshDocument(): DocumentState {
  return {
    readable: false,
    waiting: false,
    requested: false,
    observed: new Map(),
    covered: new Map(),
    generation: 0,
    answeredBy: 0,
    asked: 0,
    listeners: new Set(),
  };
}

/**
 * holdsChanges says whether the document holds a change no answer has covered. A change is read
 * whether or not a read is already in flight, so a signal that arrives during a request is not
 * lost: the read that is running may not have been computed after the change it is asking about.
 */
function holdsChanges(state: DocumentState): boolean {
  return [...state.observed].some(([id, version]) => isNewerVersion(version, state.covered.get(id) ?? ''));
}
