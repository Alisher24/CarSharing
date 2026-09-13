import { useEffect, useState } from 'react';
import { COUNTDOWN_TICK_MILLISECONDS, countdownAt, type Countdown, type Deadline } from './countdown.ts';

/**
 * useCountdown shows how much of one deadline is left, and redraws it while the component that shows
 * it is on screen. A deadline that is not there has no remaining time: a rental that is not running
 * out is not counted down to.
 */
export function useCountdown(deadline: Deadline | undefined): Countdown | undefined {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const repeat = window.setInterval(() => setNow(new Date()), COUNTDOWN_TICK_MILLISECONDS);
    return () => window.clearInterval(repeat);
  }, []);

  return deadline === undefined ? undefined : countdownAt(deadline, now);
}
