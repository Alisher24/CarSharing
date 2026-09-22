import type { ReactNode } from 'react';
import { Link, useLocation } from 'react-router';
import { INVOICES_ADDRESS, RIDES_ADDRESS, sectionOf, type CabinetSection } from '../../app/addresses.ts';
import type { Account } from '../../shared/account/session.ts';
import type { Submission } from '../../shared/account/session.ts';
import { CABINET_HEADING } from '../../shared/copy.ts';
import { INVOICES_TAB, LEAVE_ACTION, RIDES_TAB, SIGN_IN_TO_READ } from './cabinetCopy.ts';

// One section holds the cabinet, so the heading it is labelled by is declared once.
const CABINET_TITLE_ID = 'cabinet-title';

type AccountScreenProps = {
  account: Account;

  /** How the last submission ended, which is what the control that ends the session waits on. */
  submission: Submission;
  onLeave: () => Promise<void>;

  /** The feed the address leads to, which is shown once there is a session to read it with. */
  children: ReactNode;
};

/**
 * AccountScreen is the cabinet: who is signed in, the way out, the two feeds of the account's own
 * history, and whichever of them the address names.
 *
 * Without a session it says what the address is for and lets the entry window, which is shown over
 * it, do the asking. The address stays what it was, so somebody who followed a link to one of their
 * invoices reads that invoice the moment they are let in.
 */
export function AccountScreen({ account, submission, onLeave, children }: AccountScreenProps) {
  const section = sectionOf(useLocation().pathname);

  return (
    <section className="cabinet" aria-labelledby={CABINET_TITLE_ID}>
      <h2 className="cabinet-title" id={CABINET_TITLE_ID}>
        {CABINET_HEADING}
      </h2>

      <CabinetBody account={account} submission={submission} section={section} onLeave={onLeave}>
        {children}
      </CabinetBody>
    </section>
  );
}

/** What the cabinet holds for one account: the two feeds, or what the address is for without one. */
function CabinetBody({
  account,
  submission,
  section,
  onLeave,
  children,
}: {
  account: Account;
  submission: Submission;
  section: CabinetSection | undefined;
  onLeave: () => Promise<void>;
  children: ReactNode;
}) {
  if (account.state !== 'signed-in') return <p className="cabinet-invitation">{SIGN_IN_TO_READ}</p>;

  return (
    <>
      <div className="cabinet-account">
        <span className="cabinet-email" data-testid="account-email">
          {account.snapshot.user.email}
        </span>
        <button
          className="header-action"
          type="button"
          disabled={submission.state === 'sending'}
          onClick={() => void onLeave()}
        >
          {LEAVE_ACTION}
        </button>
      </div>

      <CabinetTabs section={section} />
      {children}
    </>
  );
}

/**
 * The two feeds, each at an address of its own. A tab without an address would not survive a reload
 * and could not be sent to anybody, which is what this cabinet exists for.
 */
function CabinetTabs({ section }: { section: CabinetSection | undefined }) {
  return (
    <nav className="cabinet-tabs" aria-label={CABINET_HEADING}>
      <CabinetTab address={RIDES_ADDRESS} name={RIDES_TAB} current={section === 'rides'} />
      <CabinetTab address={INVOICES_ADDRESS} name={INVOICES_TAB} current={section === 'invoices'} />
    </nav>
  );
}

function CabinetTab({ address, name, current }: { address: string; name: string; current: boolean }) {
  return (
    <Link className="cabinet-tab" to={address} aria-current={current ? 'page' : undefined}>
      {name}
    </Link>
  );
}
