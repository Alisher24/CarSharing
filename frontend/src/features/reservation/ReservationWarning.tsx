import type { CurrentSnapshot } from '../../shared/api/current.ts';
import { loadedValue, type Resource } from '../../shared/read/Resource.ts';
import type { Deadline } from '../../shared/ride/countdown.ts';
import { useCountdown } from '../../shared/ride/useCountdown.ts';
import { currentWarning, type CurrentWarning } from '../../shared/account/currentWarning.ts';
import type { Notifications } from '../../shared/account/notifications.ts';
import { READ_ACTION, READ_FAILED, READ_PENDING, WARNING_HEADING, warningText } from './reservationCopy.ts';

/** The identifier the heading is named by, so the section is announced with its own words. */
const WARNING_TITLE_ID = 'reservation-warning-heading';

type ReservationWarningProps = {
  /** What the private read answered about the reservation in force. */
  current: Resource<CurrentSnapshot | undefined>;

  /** The notifications addressed to the account, and the action that marks one read. */
  notifications: Notifications;
};

/**
 * ReservationWarning is the one warning a person sees while their reservation is running out: which
 * vehicle is held for them, when the reservation ends and how long is left. It stands beside the
 * reservation panel rather than inside it, so the panel keeps its own countdown and its cancellation,
 * and the warning states what is about to happen. Its one action marks the warning read on the
 * server, where the read state lives, so a reload never shows a read warning as new.
 */
export function ReservationWarning({ current, notifications }: ReservationWarningProps) {
  const reading = notifications.reading;
  const warning = currentWarning(reading?.collection, loadedValue(current));
  const countdown = useCountdown(deadlineOf(warning, reading?.receivedAt));
  if (warning === undefined) return null;

  const text = warningText(warning, countdown);
  if (text === undefined) return null;

  const sending = notifications.read.state === 'sending';
  return (
    <section className="reservation-warning" aria-labelledby={WARNING_TITLE_ID}>
      <h2 className="reservation-warning-heading" id={WARNING_TITLE_ID}>
        {WARNING_HEADING}
      </h2>
      <p className="reservation-warning-vehicle">{text.vehicle}</p>
      <p className="reservation-warning-deadline">{text.deadline}</p>
      <p className="reservation-warning-left" role="status">
        {text.remaining}
      </p>

      <div className="reservation-warning-actions">
        <button
          className="action-button"
          type="button"
          disabled={sending}
          onClick={() => notifications.markRead(warning.notificationId)}
        >
          {READ_ACTION}
        </button>
        {sending && <p className="reservation-warning-notice">{READ_PENDING}</p>}
        {notifications.read.state === 'failed' && <p className="reservation-warning-notice">{READ_FAILED}</p>}
      </div>
    </section>
  );
}

/**
 * The deadline the warning counts down to: the moment the collection published and the moment that
 * answer was computed at, which is what says how much of the reservation is left, anchored to the
 * local moment the answer arrived. Nothing is counted down to until both are known.
 */
function deadlineOf(warning: CurrentWarning | undefined, receivedAt: Date | undefined): Deadline | undefined {
  if (warning === undefined || receivedAt === undefined) return undefined;

  return { expiresAt: warning.expiresAt, serverTime: warning.serverTime, receivedAt };
}
