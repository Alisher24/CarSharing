import type { ApiError } from '../api/generated/types.gen.ts';
import type { CommandAction } from './unfinishedCommand.ts';
import { refusalText } from '../refusal.ts';
import { SIGN_IN_TO_MANAGE, UNKNOWN_COMMAND } from '../copy.ts';

/**
 * Where one command of the service stands, whichever command it was and whoever sent it. The
 * outcomes are kept apart because a person's next step differs: a refusal is answered by reading the
 * state again, and an answer that never arrived leaves the outcome unknown until the command is
 * repeated with the key it was sent with.
 */
export type CommandPhase =
  | { state: 'idle' }
  | { state: 'sending'; action: CommandAction }
  | { state: 'done'; action: CommandAction; replayed: boolean }
  | { state: 'refused'; action: CommandAction; code: ApiError['code'] }
  | { state: 'unknown'; action: CommandAction }
  | { state: 'signed-out' };

/** What each command says while it is on its way to the server. */
const PENDING_TEXT: Record<CommandAction, string> = {
  reserve: 'Отправляем запрос…',
  cancel: 'Отменяем бронь…',
  start: 'Начинаем поездку…',
  pause: 'Ставим поездку на паузу…',
  resume: 'Продолжаем поездку…',
  finish: 'Завершаем поездку…',
  pay: 'Оплачиваем счёт…',
};

/**
 * The sentence the last command is reported with, or undefined while there is nothing to report. A
 * command still on its way says so; one whose answer never arrived says that instead of pretending to
 * know; a refusal is named by its contract code, so a person reads what the server objected to.
 */
export function commandText(phase: CommandPhase): string | undefined {
  switch (phase.state) {
    case 'sending':
      return PENDING_TEXT[phase.action];
    case 'unknown':
      return UNKNOWN_COMMAND;
    case 'refused':
      return refusalText(phase.code);
    case 'signed-out':
      return SIGN_IN_TO_MANAGE;
    default:
      return undefined;
  }
}
