import type { AccountIntent } from './accountIntent';

/**
 * The Russian wording of the entry window: what each of its two tabs is called, what the one action
 * of the form offers, and what the window says while there is no session to enter yet.
 *
 * What a field is told when it does not hold a usable value is not here: that wording states the
 * rule it broke and lives beside the rules, in `credentialRules`.
 */

/** The two tabs in the order they are offered, which is the order the window can be reached in. */
export const ENTRY_INTENTS: readonly AccountIntent[] = ['sign-in', 'register'];

/**
 * What each tab is called. A tab says what the person is doing, and the window is titled with it:
 * one word for the operation, declared once, rather than a heading that can disagree with the tab
 * the form is actually on.
 */
export const ENTRY_TAB_TITLES: Record<AccountIntent, string> = {
  'sign-in': 'Вход',
  register: 'Регистрация',
};

/** What the one action of the form offers, which is the operation the chosen tab names. */
export const ENTRY_ACTIONS: Record<AccountIntent, string> = {
  'sign-in': 'Войти',
  register: 'Зарегистрироваться',
};

/** What the two tabs are offered as a group of, which is what a screen reader reads them by. */
export const ENTRY_TABS_LABEL = 'Вход или регистрация';

/** What the window is titled while the session is still being checked, so it is not a form yet. */
export const SESSION_CHECK_NOTICE = 'Проверяем сессию…';

/** What the control that closes the window offers; the mark it shows is decoration beside it. */
export const CLOSE_ACTION = 'Закрыть';
