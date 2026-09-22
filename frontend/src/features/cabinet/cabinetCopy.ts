import type { AbsenceCopy } from '../../shared/components/absence.ts';

/**
 * The Russian wording of the cabinet: the two feeds a person reads their own history in, and what
 * each of them says when it holds nothing.
 *
 * Nothing here computes a number, a moment or an amount. What a row states is built from the answer
 * the service gave, in the modules beside this one. What the cabinet shares with the header — the
 * words for the cabinet, for a session and for a control that opens an invoice — belongs to the
 * shell and the ride that lead into it, and is declared in `shared/copy.ts`.
 */

/** What the tab of each feed is called. */
export const RIDES_TAB = 'Поездки';

export const INVOICES_TAB = 'Счета';

/** What the control that ends the session offers, which the cabinet is the one place for. */
export const LEAVE_ACTION = 'Выйти';

/** What the control that reads the page after the one on screen offers. */
export const READ_MORE_ACTION = 'Показать ещё';

/** What a person who is not signed in is told above the form that lets them in. */
export const SIGN_IN_TO_READ = 'Войдите, чтобы открыть историю поездок и счетов';

/** What each feed says while it has nothing to show, which is not the same as being unable to read. */
export const RIDES_ABSENCE: AbsenceCopy = {
  loading: 'Загружаем поездки…',
  none: 'Вы ещё не совершали поездок',
  unreachable: 'Не удалось загрузить поездки',
};

export const INVOICES_ABSENCE: AbsenceCopy = {
  loading: 'Загружаем счета…',
  none: 'Счетов пока нет',
  unreachable: 'Не удалось загрузить счета',
};

/** What the reading of one invoice says while it has nothing to show. */
export const INVOICE_ABSENCE: AbsenceCopy = {
  loading: 'Загружаем счёт…',
  // An invoice of another account and one that never existed are one answer: saying that this one
  // belongs to somebody else would be telling a stranger that it exists.
  none: 'Такого счёта нет',
  unreachable: 'Не удалось загрузить счёт',
};
