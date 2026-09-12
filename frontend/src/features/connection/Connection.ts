import type { ReadyStatus } from '../../shared/api/health';

/** What the interface knows about the service behind it, as one state the card can render. */
export type Connection = { state: 'loading' } | { state: 'ready'; status: ReadyStatus } | { state: 'error' };
