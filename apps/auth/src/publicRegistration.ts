import type { IncomingMessage, ServerResponse } from "node:http";
import type { AuthNodeHandler } from "./authEntryGuard.js";

export function createAuthDispatchHandler(
  betterAuthHandler: AuthNodeHandler,
): AuthNodeHandler {
  return (req: IncomingMessage, res: ServerResponse) => betterAuthHandler(req, res);
}
