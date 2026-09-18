import { useEffect, useState } from 'react';
import { useLocation } from 'react-router';
import { AppHeader } from './AppHeader';
import { AppRoutes } from './AppRoutes';
import { useApplication } from './useApplication';
import { sectionOf } from './addresses';
import { EntryDialog } from '../features/account/EntryDialog';

/**
 * App is the whole application at one address: the header every screen carries, whichever screen the
 * address names, and the entry window over both.
 *
 * It holds one thing of its own — whether the entry window was asked for — because the one account
 * control in the header asks for it, and the cabinet asks for it by being opened without a session.
 * The window is closed by signing in, so it never stays over a screen whose person is already known.
 */
export function App() {
  const application = useApplication();
  const { pathname } = useLocation();
  const [entryRequested, setEntryRequested] = useState(false);

  const signedIn = application.account.state === 'signed-in';
  const entryOpen = entryRequested && !signedIn;

  // A cabinet address without a session opens the window over it: a person who followed a link to
  // their own history answers the form where they were going rather than being sent back to the map.
  // Opening it here, rather than by the cabinet, is what keeps a window the person closed closed —
  // the address already on screen does not ask for it a second time.
  useEffect(() => {
    if (sectionOf(pathname) !== undefined && !signedIn) setEntryRequested(true);
  }, [pathname, signedIn]);

  return (
    <div className="page">
      <AppHeader
        connection={application.connection}
        stream={application.stream}
        signedIn={signedIn}
        entryOpen={entryOpen}
        onToggleEntry={() => setEntryRequested((asked) => !asked)}
      />
      <AppRoutes application={application} />
      <EntryDialog
        open={entryOpen}
        account={application.account}
        submission={application.submission}
        onSubmit={application.submit}
        onForgetRefusal={application.forgetRefusal}
        onClose={() => setEntryRequested(false)}
      />
    </div>
  );
}
