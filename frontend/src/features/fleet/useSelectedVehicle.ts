import { useEffect, useRef } from 'react';
import type { Vehicle } from '../../shared/api/catalog';

/**
 * useSelectedVehicle answers what an open card is about. It follows the newest reading of the
 * selected vehicle, and keeps the last one it saw if that vehicle leaves the fleet entirely, so a
 * reload never empties a card a person is still reading. Only closing the card clears it.
 */
export function useSelectedVehicle(vehicles: readonly Vehicle[], selectedId: string | undefined): Vehicle | undefined {
  const current = vehicles.find((vehicle) => vehicle.id === selectedId);
  const lastSeen = useRef<Vehicle>(undefined);

  // The reading is remembered after the render that saw it rather than during it: a render React
  // discards must not leave a value behind that no screen ever showed.
  useEffect(() => {
    if (current !== undefined) lastSeen.current = current;
  }, [current]);

  if (selectedId === undefined) return undefined;
  if (current !== undefined) return current;
  return lastSeen.current?.id === selectedId ? lastSeen.current : undefined;
}
