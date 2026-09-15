import { useState } from 'react';
import { AppHeader } from './AppHeader';
import { AppRoutes } from './AppRoutes';
import { useApplication } from './useApplication';

/**
 * App is the whole application at one address: the header that every screen carries, and whichever
 * screen the address names. It holds one thing of its own — whether the entry panel on the map is
 * open — because the one account control in the header is what opens it.
 */
export function App() {
  const application = useApplication();
  const [entryOpen, setEntryOpen] = useState(false);
  const signedIn = application.account.state === 'signed-in';

  return (
    <div className="page">
      <AppHeader
        connection={application.connection}
        stream={application.stream}
        signedIn={signedIn}
        entryOpen={entryOpen}
        onToggleEntry={() => setEntryOpen((open) => !open)}
      />
      <AppRoutes application={application} entryOpen={entryOpen && !signedIn} />
    </div>
  );
}
