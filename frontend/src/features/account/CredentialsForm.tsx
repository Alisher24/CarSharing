import type { FormEvent } from 'react';
import { ENTRY_ACTIONS, ENTRY_INTENTS, ENTRY_TAB_TITLES, ENTRY_TABS_LABEL } from './accountCopy';
import { CredentialInput } from './CredentialInput';
import { CREDENTIAL_FIELDS } from './credentialRules';
import { refusalText } from './refusalText';
import { useCredentialChecks } from './useCredentialChecks';
import type { AccountIntent } from './accountIntent';
import type { Submission } from './useAccount';
import type { Credentials } from '../../shared/api/session';

type CredentialsFormProps = {
  /** Which operation the form is asking for, which is the tab the person chose. */
  intent: AccountIntent;
  submission: Submission;
  onSubmit: (intent: AccountIntent, credentials: Credentials) => Promise<void>;

  /** Which of the two operations the person chose, which decides the action and the window's title. */
  onChooseIntent: (intent: AccountIntent) => void;

  /** Drops the answer of the submission the person is no longer asking about. */
  onForgetRefusal: () => void;
};

/**
 * CredentialsForm is the one form of the entry window: the tab naming the operation, the two fields,
 * and the single action that operation is asked with.
 *
 * What stops the form from being sent is the first field that is wrong, and the cursor is put on it,
 * so nobody has to look for what the server would have said anyway. None of it decides anything for
 * the server: every rule is one the contract already states, and a refusal the server answers with
 * is shown exactly as it was.
 */
export function CredentialsForm({
  intent,
  submission,
  onSubmit,
  onChooseIntent,
  onForgetRefusal,
}: CredentialsFormProps) {
  const checks = useCredentialChecks();
  const sending = submission.state === 'sending';
  const failure = refusalText(submission);

  function send(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const stopped = checks.checkAll();

    if (stopped === null) {
      void onSubmit(intent, checks.values);
      return;
    }

    checks.focus(stopped.field);
  }

  // Choosing the tab that is already chosen is not a change, so pressing it again takes nothing from
  // the person. Changing it is not a fault either: the marks of the tab being left and the refusal it
  // brought are dropped with it, and what was typed stays where it was.
  function choose(next: AccountIntent) {
    if (next === intent) return;

    checks.forgetChecks();
    onChooseIntent(next);
    onForgetRefusal();
  }

  return (
    <form className="account-form" noValidate onSubmit={send}>
      <EntryTabs intent={intent} onChoose={choose} />
      {CREDENTIAL_FIELDS.map((field) => (
        <CredentialInput
          key={field}
          field={field}
          intent={intent}
          value={checks.values[field]}
          error={checks.errors[field]}
          input={checks.inputs[field]}
          onChange={(value) => checks.change(field, value)}
          onBlur={() => checks.markChecked(field)}
        />
      ))}
      {failure && (
        <p className="account-error" role="alert" data-testid="account-error">
          {failure}
        </p>
      )}
      <div className="account-actions">
        <button className="action-button" type="submit" disabled={sending}>
          {ENTRY_ACTIONS[intent]}
        </button>
      </div>
    </form>
  );
}

/** The two tabs, which are how the person says what they are doing before they do it. */
function EntryTabs({ intent, onChoose }: { intent: AccountIntent; onChoose: (intent: AccountIntent) => void }) {
  return (
    <div className="account-tabs" role="group" aria-label={ENTRY_TABS_LABEL}>
      {ENTRY_INTENTS.map((offered) => (
        <button
          key={offered}
          className="account-tab"
          type="button"
          aria-pressed={offered === intent}
          onClick={() => onChoose(offered)}
        >
          {ENTRY_TAB_TITLES[offered]}
        </button>
      ))}
    </div>
  );
}
