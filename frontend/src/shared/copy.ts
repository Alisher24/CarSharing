/**
 * The Russian wording that more than one feature shows, declared once so two screens cannot say the
 * same thing two ways. What one feature alone shows belongs in that feature's own copy module.
 */

/**
 * What the cabinet is called. The header control that leads there says the same word, because a
 * control is named by where it goes: two spellings of one destination would be one word too many.
 */
export const CABINET_HEADING = 'Кабинет';

/** What the header offers a person who is not signed in. */
export const SIGN_IN_ACTION = 'Вход';

/** What the link to one invoice offers, from the ride it charged and from the feed of invoices. */
export const OPEN_INVOICE = 'Открыть счёт';

/** What the link back from one invoice to the feed it belongs to offers. */
export const BACK_TO_INVOICES = 'К счетам';

/** What a control that asks for one more attempt offers, wherever a read may have failed. */
export const RETRY_ACTION = 'Повторить';

/** What a person who is not signed in is told where a control would otherwise book for them. */
export const SIGN_IN_TO_BOOK = 'Войдите, чтобы забронировать автомобиль';
/** What a control that opens the conditions of a booking offers. */
export const BOOK_ACTION = 'Забронировать на 15 минут';

/** The heading of the step that asks a person to agree to the conditions before they are taken. */
export const CONFIRM_HEADING = 'Подтверждение брони';

/** What a person is warned about, in the words the specification fixes. */
export const FREE_RESERVATION_WARNING =
  'Одна бесплатная бронь в день. После отмены или истечения лимит не восстанавливается';

/** The control that confirms the booking, in the words the specification fixes. */
export const CONFIRM_ACTION = 'Использовать бесплатную бронь';

/** What the confirmation says the reservation costs and how long it stands. */
export const FREE_PERIOD = '15 минут бесплатно';

/**
 * The control that keeps a booking a person asked about confirming. A booking is confirmed from the
 * card of a vehicle and given back from the panel above the map, so the one word for keeping it is
 * declared here rather than in either feature that offers it.
 */
export const KEEP_BOOKING_ACTION = 'Оставить бронь';

/** What the interface says about a command whose outcome it does not know yet. */
export const UNKNOWN_COMMAND = 'Результат команды неизвестен: ответ не получен';

/** What the interface offers to settle an unknown outcome with the key the command was sent with. */
export const REPEAT_ACTION = 'Повторить команду';

/** What the interface says instead of offering a repeat past its window. */
export const REPEAT_EXPIRED = 'Повторить команду больше нельзя: прошло больше суток';

/** What a command of an ended session is answered with, whichever command it was. */
export const SIGN_IN_TO_MANAGE = 'Войдите, чтобы управлять арендой';
