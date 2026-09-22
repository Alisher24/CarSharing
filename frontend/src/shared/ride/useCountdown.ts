import { countdownAt, type Countdown, type Deadline } from './countdown.ts';
import { useClockTick } from './useClockTick.ts';

/**
 * useCountdown shows how much of one deadline is left, and redraws it while the component that shows
 * it is on screen. A deadline that is not there has no remaining time: a rental that is not running
 * out is not counted down to.
 */
export function useCountdown(deadline: Deadline | undefined): Countdown | undefined {
  const now = useClockTick();

  return deadline === undefined ? undefined : countdownAt(deadline, now);
}
