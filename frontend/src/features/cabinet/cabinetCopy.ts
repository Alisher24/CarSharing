import type { AbsenceCopy } from '../fleet/ResourceNotice.tsx';

/**
 * The Russian wording of the cabinet: the two feeds a person reads their own history in, what each
 * of them says when it holds nothing, and what a page that could not be read offers.
 *
 * Nothing here computes a number, a moment or an amount. What a row states is built from the answer
 * the service gave, in the modules beside this one.
 */

/**
 * What the cabinet is called. The header control that leads there says the same word, because a
 * control is named by where it goes: two spellings of one destination would be one word too many.
 */
export const CABINET_HEADING = 'Кабинет';

/** What the header offers a person who is not signed in. */
export const SIGN_IN_ACTION = 'Вход';

/** What the tab of each feed is called. */
export const RIDES_TAB = 'Поездки';
export const INVOICES_TAB = 'Счета';

/** What the control that ends the session offers, which the cabinet is the one place for. */
export const LEAVE_ACTION = 'Выйти';

/** What the control that reads the page after the one on screen offers. */
export const READ_MORE_ACTION = 'Показать ещё';

/** What the link to one invoice offers, from the ride it charged and from the feed of invoices. */
export const OPEN_INVOICE = 'Открыть счёт';

/** What the link back from one invoice to the feed it belongs to offers. */
export const BACK_TO_INVOICES = 'К счетам';

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
