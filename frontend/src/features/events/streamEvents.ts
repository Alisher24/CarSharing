import type { ExactInteger } from '../../shared/api/catalog.ts';
import type { Frame } from './frames.ts';
import type { DocumentKind } from './readCycle.ts';

/**
 * What one frame means to the client. The stream carries no replay and no identifiers, so a frame
 * is either the handshake that says the subscription is established, a change signal, or something
 * this contract does not declare and which is therefore ignored.
 */
export type StreamEvent =
  | { kind: 'ready'; serverTime: string }
  | { kind: 'changed'; resource: DocumentKind; id: string; version: ExactInteger }
  | { kind: 'ignored' };

/** The handshake a subscription starts with. */
const READY = 'ready';

/**
 * How each event the contract declares is read. The name of the event is the whole of the type: the
 * payload states only which resource moved and how far. A change to the account's own rental arrives
 * on the private stream, which is the only connection it is published to.
 */
const RESOURCES_BY_EVENT: Record<string, DocumentKind> = {
  'vehicle.changed': 'vehicles',
  'zone.changed': 'zones',
  'tariff.changed': 'tariffs',
  'rental.changed': 'current',
};

/** What one frame says, or that it says nothing this client knows how to use. */
export function streamEvent(frame: Frame): StreamEvent {
  if (frame.data === undefined) return { kind: 'ignored' };
  if (frame.event === READY) return readyEvent(frame.data);

  const resource = RESOURCES_BY_EVENT[frame.event];
  if (resource === undefined) return { kind: 'ignored' };

  return changedEvent(resource, frame.data);
}

function readyEvent(data: string): StreamEvent {
  const payload = payloadOf(data) as { server_time?: unknown };
  if (typeof payload?.server_time !== 'string') return { kind: 'ignored' };

  return { kind: 'ready', serverTime: payload.server_time };
}

function changedEvent(resource: DocumentKind, data: string): StreamEvent {
  const payload = payloadOf(data) as { id?: unknown; version?: unknown };
  if (typeof payload?.id !== 'string' || typeof payload.version !== 'string') return { kind: 'ignored' };

  return { kind: 'changed', resource, id: payload.id, version: payload.version };
}

// A frame this client cannot read is dropped rather than thrown: a stream is a signal, and the
// reconciliation reads the snapshots it is a signal about.
function payloadOf(data: string): unknown {
  try {
    return JSON.parse(data);
  } catch {
    return undefined;
  }
}
