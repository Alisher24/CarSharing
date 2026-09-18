import { Link } from 'react-router';
import { ACCOUNT_ADDRESS, MAP_ADDRESS } from './addresses';
import { ConnectionIndicator } from '../features/connection/ConnectionIndicator';
import { StreamNotice } from '../features/connection/StreamNotice';
import type { Connection } from '../features/connection/useConnection';
import type { EventsConnection } from '../features/events/useEventStream';
import { CABINET_HEADING, SIGN_IN_ACTION } from '../features/cabinet/cabinetCopy';

type AppHeaderProps = {
  connection: Connection;
  stream: EventsConnection;

  /** Whether there is a session, which is the whole of what the one account control depends on. */
  signedIn: boolean;

  /** Whether the entry window is open, which the control opens and closes while there is no session. */
  entryOpen: boolean;
  onToggleEntry: () => void;
};

/**
 * AppHeader carries the one account control of the application. It states what a person can do next
 * rather than what they have: without a session it opens the entry window over whatever screen is on,
 * and with one it leads to the cabinet, which is where the account and the way out of it now live.
 */
export function AppHeader({ connection, stream, signedIn, entryOpen, onToggleEntry }: AppHeaderProps) {
  return (
    <header className="page-header">
      <Link className="brand" to={MAP_ADDRESS} aria-label="CarSharing — главная">
        <span className="brand-mark" aria-hidden="true">
          c↗
        </span>
        CarSharing
      </Link>
      <span className="location">
        <span className="location-mark" aria-hidden="true">
          ◉
        </span>
        Бишкек
      </span>
      <StreamNotice stream={stream} connection={connection} />
      <ConnectionIndicator connection={connection} />
      {signedIn ? (
        <Link className="header-action" to={ACCOUNT_ADDRESS}>
          {CABINET_HEADING}
        </Link>
      ) : (
        <button
          className="header-action"
          type="button"
          aria-haspopup="dialog"
          aria-expanded={entryOpen}
          onClick={onToggleEntry}
        >
          {SIGN_IN_ACTION}
        </button>
      )}
    </header>
  );
}
