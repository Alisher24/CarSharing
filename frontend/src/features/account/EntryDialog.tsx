import { useState } from 'react';
import { ModalWindow } from './ModalWindow';
import { CredentialsForm } from './CredentialsForm';
import { ENTRY_TAB_TITLES, SESSION_CHECK_NOTICE } from './accountCopy';
import type { AccountIntent } from './accountIntent';
import type { Account, Submission } from './useAccount';
import type { Credentials } from '../../shared/api/session';

/** What a window that has just been opened offers: a person who opens it is usually coming back. */
const FIRST_INTENT: AccountIntent = 'sign-in';

type EntryDialogProps = {
  open: boolean;
  account: Account;
  submission: Submission;
  onSubmit: (intent: AccountIntent, credentials: Credentials) => Promise<void>;
  onForgetRefusal: () => void;
  onClose: () => void;
};

/**
 * EntryDialog is the one way into the application: a window over whatever address the person is on,
 * holding the form that registers them or signs them in. The map and the cabinet both show it, so
 * there is one entry rather than one per frame, and the address stays what it was either way.
 *
 * Whether the session has been read yet is the account's business, so the window says that the
 * session is still being checked rather than offering a form that may already be pointless.
 * Closing it forgets what was typed and which tab was chosen: an entry that was abandoned is
 * answered from the beginning when it is opened again.
 */
export function EntryDialog({ open, account, submission, onSubmit, onForgetRefusal, onClose }: EntryDialogProps) {
  const [intent, setIntent] = useState<AccountIntent>(FIRST_INTENT);

  const checking = account.state === 'checking';
  const title = checking ? SESSION_CHECK_NOTICE : ENTRY_TAB_TITLES[intent];

  function close() {
    setIntent(FIRST_INTENT);
    onClose();
  }

  return (
    <ModalWindow open={open} title={title} onClose={close}>
      {!checking && (
        <CredentialsForm
          intent={intent}
          submission={submission}
          onSubmit={onSubmit}
          onChooseIntent={setIntent}
          onForgetRefusal={onForgetRefusal}
        />
      )}
    </ModalWindow>
  );
}
