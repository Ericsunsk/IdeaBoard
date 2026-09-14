import { timingSafeEqual } from "node:crypto";
import type { FastifyReply, FastifyRequest } from "fastify";
import { getHeaderValue, sendError } from "../infrastructure/httpResponse.js";

/**
 * Shared service-to-service guard for internal auth routes: the caller must
 * present the configured `x-internal-key`. Comparison is timing-safe; empty
 * or mismatched keys are rejected before any handler runs.
 */
export function matchesInternalKey(provided: string, expected: string): boolean {
  const left = Buffer.from(provided.trim());
  const right = Buffer.from(expected.trim());
  return (
    left.length > 0 &&
    left.length === right.length &&
    timingSafeEqual(left, right)
  );
}

export function createInternalKeyPreHandler(internalKey: string) {
  return async (request: FastifyRequest, reply: FastifyReply) => {
    if (
      !matchesInternalKey(
        getHeaderValue(request.headers["x-internal-key"]),
        internalKey,
      )
    ) {
      sendError(reply, 401, "unauthorized internal request");
      return reply;
    }
  };
}
