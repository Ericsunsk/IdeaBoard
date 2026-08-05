import type { IncomingMessage, ServerResponse } from "node:http";
import type { AuthNodeHandler } from "./authEntryGuard.js";
import { getAuthSubpath } from "./authUtils.js";
import { sendNodeJSON } from "./httpResponse.js";

export function createAuthDispatchHandler(
  betterAuthHandler: AuthNodeHandler,
): AuthNodeHandler {
  return async (req: IncomingMessage, res: ServerResponse) => {
    if (getAuthSubpath(req.url || "") === "/register/username") {
      sendNodeJSON(res, 404, {
        code: 404,
        msg: "username registration is no longer supported",
      });
      return;
    }
    await betterAuthHandler(req, res);
  };
}
