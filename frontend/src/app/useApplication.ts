import { useEffect, useRef } from 'react';
import { sessionOf, useAccount, type Account, type Submission } from '../features/account/useAccount';
import type { AccountIntent } from '../features/account/accountIntent';
import { useConnection, type Connection } from '../features/connection/useConnection';
import type { EventsConnection } from '../features/events/useEventStream';
import { useEvents } from '../features/events/useEvents';
import { usePrivateEvents, type PrivateFeed } from '../features/events/usePrivateEvents';
import { useCatalog, type Catalog } from '../features/fleet/useCatalog';
import { useNotifications, type Notifications } from '../features/notifications/useNotifications';
import { useCurrentRental, type CurrentRental } from '../features/reservation/useCurrentRental';
import { useReservations, type Reservations } from '../features/reservation/useReservations';
import { useRideCommands, type RideCommands } from '../features/reservation/useRideCommands';
import type { Credentials } from '../shared/api/session';

/**
 * Everything the application reads and can do, whichever address is on screen. The session, the two
 * change streams and the readers of what the signed-in person is doing now all live here rather than
 * in a screen, because the stream belongs to the session: opening the cabinet must not reopen it,
 * and leaving the cabinet must not stop it.
 */
export type Application = {
  connection: Connection;
  stream: EventsConnection;

  account: Account;
  submission: Submission;
  submit: (intent: AccountIntent, credentials: Credentials) => Promise<void>;

  /** Drops the refusal of a submission the person has moved on from, such as by changing the tab. */
  forgetRefusal: () => void;

  leave: () => Promise<void>;

  /** The private stream, which every reader of the account's own records takes its signals from. */
  privateEvents: PrivateFeed;

  catalog: Catalog;
  current: CurrentRental;
  reservations: Reservations;
  ride: RideCommands;
  notifications: Notifications;
};

/** useApplication assembles what every address of the application is shown from. */
export function useApplication(): Application {
  const connection = useConnection();
  const { account, submission, submit, forgetRefusal, leave, recheck } = useAccount();
  const events = useEvents();
  const catalog = useCatalog(events);

  const privateEvents = usePrivateEvents(sessionOf(account), recheck);
  useSessionCheckOnRecovery(events.connection, recheck);

  // What the person is doing now, and the commands that change it, are read and held here rather
  // than by a panel: the panel above the map and the card that books a vehicle are two views of one
  // reservation, and the private stream keeps both of them current.
  const current = useCurrentRental(account, privateEvents);
  const reservations = useReservations(account, current);
  const ride = useRideCommands(account, current);
  const notifications = useNotifications(account, privateEvents);

  return {
    connection,
    stream: events.connection,
    account,
    submission,
    submit,
    forgetRefusal,
    leave,
    privateEvents,
    catalog,
    current,
    reservations,
    ride,
    notifications,
  };
}

/**
 * A connection that has come back is the moment to ask the server who the caller is now: the
 * session may have been revoked while the browser could not reach anything, and a session that is
 * still live lets the private stream open again. Reading the snapshots again is the handshake's
 * business, and a handshake follows this recovery.
 */
function useSessionCheckOnRecovery(stream: EventsConnection, recheck: () => void): void {
  const previous = useRef(stream);

  useEffect(() => {
    const recovered = stream === 'connected' && previous.current !== 'connected';
    previous.current = stream;
    if (recovered) recheck();
  }, [stream, recheck]);
}
