import { useEffect, useState } from 'react';
import { COUNTDOWN_TICK_MILLISECONDS } from './countdown.ts';

/**
 * useClockTick is the moment a view last looked at the clock, redrawn on an interval while that view
 * is on screen. Nothing is counted here: the value is the local clock, and what a view derives from
 * it is derived again on every redraw.
 */
export function useClockTick(): Date {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const repeat = window.setInterval(() => setNow(new Date()), COUNTDOWN_TICK_MILLISECONDS);
    return () => window.clearInterval(repeat);
  }, []);

  return now;
}
