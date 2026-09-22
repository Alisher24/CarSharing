import { finishRental, pauseRental, resumeRental, startRental } from './generated/sdk.gen';
import type { FinishResult, RentalCommandResult } from './generated/types.gen';
import { answerOf, commandHeaders, type CommandCredentials, type CommandResult } from './commands.ts';
import { sameOriginRequest } from './request.ts';

/**
 * The commands that move a rental through its ride: a reservation becomes a ride, the ride is held,
 * the ride carries on, and the ride is ended. Each answers with the rental as the transition left it,
 * so the interface reads the state the server holds rather than the state it asked for; the ending
 * answers with the invoice it produced as well.
 */

/** Starts the ride a reservation is waiting for, which ends the free period and opens the ride. */
export async function startRide(
  rentalId: string,
  credentials: CommandCredentials,
): Promise<CommandResult<RentalCommandResult>> {
  return answerOf(() =>
    startRental({
      path: { id: rentalId },
      headers: commandHeaders(credentials),
      ...sameOriginRequest,
    }),
  );
}

/** Holds the ride where it is: the driving ends and the paused interval of the same ride opens. */
export async function pauseRide(
  rentalId: string,
  credentials: CommandCredentials,
): Promise<CommandResult<RentalCommandResult>> {
  return answerOf(() =>
    pauseRental({
      path: { id: rentalId },
      headers: commandHeaders(credentials),
      ...sameOriginRequest,
    }),
  );
}

/** Carries the ride on after a pause, opening the next driving interval where the pause ended. */
export async function resumeRide(
  rentalId: string,
  credentials: CommandCredentials,
): Promise<CommandResult<RentalCommandResult>> {
  return answerOf(() =>
    resumeRental({
      path: { id: rentalId },
      headers: commandHeaders(credentials),
      ...sameOriginRequest,
    }),
  );
}

/**
 * Ends the ride and answers with the invoice it produced. The ending is judged by the service from
 * confirmed telemetry, so nothing about where the vehicle stands is sent with the command.
 */
export async function finishRide(
  rentalId: string,
  credentials: CommandCredentials,
): Promise<CommandResult<FinishResult>> {
  return answerOf(() =>
    finishRental({
      path: { id: rentalId },
      headers: commandHeaders(credentials),
      ...sameOriginRequest,
    }),
  );
}
