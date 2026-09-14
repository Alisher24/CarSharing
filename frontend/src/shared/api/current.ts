import { cancelRental, getCurrentRental, reserve } from './generated/sdk.gen';
import type {
  CurrentSnapshot,
  DailyLimitState,
  Rental,
  RentalCommandResult,
  ReserveResult,
} from './generated/types.gen';
import {
  answerOf,
  commandHeaders,
  sameOriginRequest,
  type CommandCredentials,
  type CommandResult,
} from './commands.ts';

export type {
  ActiveRental,
  ApiError,
  CurrentRental,
  CurrentSnapshot,
  DailyLimitState,
  NoCurrentRental,
  PausedRental,
  Progress,
  Rental,
  RentalCommandResult,
  ReserveResult,
  TariffSnapshot,
  Vehicle,
  VehicleReference,
} from './generated/types.gen';

export type { CommandCredentials, CommandResult } from './commands.ts';

/**
 * A rental that is still a live claim on a vehicle: one waiting to be started, a ride in motion, and
 * a ride that is held. What a person can do next differs between them, and none of them is over.
 */
export type LiveRental = Extract<Rental, { state: 'reserved' | 'active' | 'paused' }>;

/** A rental that is only a reservation, which is the one a person may give back before riding. */
export type ReservedRental = Extract<Rental, { state: 'reserved' }>;

/** Whether a rental is still live, which is what decides that the interface shows it at all. */
export function isLiveRental(rental: Rental): rental is LiveRental {
  return rental.state === 'reserved' || rental.state === 'active' || rental.state === 'paused';
}

/**
 * fetchCurrentRental reads what the account is doing now. It is a private resource: the browser
 * sends the session cookie, and the answer carries the current rental with the day's allowance.
 */
export async function fetchCurrentRental(signal: AbortSignal): Promise<CurrentSnapshot> {
  const { data } = await getCurrentRental({ ...sameOriginRequest, throwOnError: true, signal });
  return data;
}

/** Books one vehicle for the free period the service allows. */
export async function reserveVehicle(
  vehicleId: string,
  credentials: CommandCredentials,
): Promise<CommandResult<ReserveResult>> {
  return answerOf(() =>
    reserve({
      body: { vehicle_id: vehicleId },
      headers: commandHeaders(credentials),
      ...sameOriginRequest,
    }),
  );
}

/** Gives one reservation back, which frees the vehicle without returning the day's allowance. */
export async function cancelReservation(
  rentalId: string,
  credentials: CommandCredentials,
): Promise<CommandResult<RentalCommandResult>> {
  return answerOf(() =>
    cancelRental({
      path: { id: rentalId },
      headers: commandHeaders(credentials),
      ...sameOriginRequest,
    }),
  );
}

/** The allowance of free reservations, as the current read publishes it. */
export function dailyLimitOf(snapshot: CurrentSnapshot): DailyLimitState {
  return snapshot.daily_limit;
}
