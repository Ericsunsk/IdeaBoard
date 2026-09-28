import assert from "node:assert/strict";
import { describe, it } from "node:test";
import type { FastifyReply, FastifyRequest } from "fastify";
import {
  createInternalKeyPreHandler,
  matchesInternalKey,
} from "./internalKeyGuard.js";

describe("matchesInternalKey", () => {
  it("accepts the configured key and trims surrounding whitespace", () => {
    assert.equal(matchesInternalKey("secret-key", "secret-key"), true);
    assert.equal(matchesInternalKey("  secret-key  ", "secret-key"), true);
  });

  it("rejects empty or mismatched keys", () => {
    assert.equal(matchesInternalKey("", "secret-key"), false);
    assert.equal(matchesInternalKey("wrong-key", "secret-key"), false);
    assert.equal(matchesInternalKey("short", "a-much-longer-key"), false);
    assert.equal(matchesInternalKey("secret-key", ""), false);
  });
});

describe("createInternalKeyPreHandler", () => {
  it("rejects requests without the internal key", async () => {
    const preHandler = createInternalKeyPreHandler("secret-key");
    const reply = createReply();
    const result = await preHandler(
      { headers: {} } as FastifyRequest,
      reply as unknown as FastifyReply,
    );

    assert.equal(result, reply);
    assert.equal(reply.statusCode, 401);
    assert.deepEqual(reply.payload, {
      code: 401,
      msg: "unauthorized internal request",
    });
  });

  it("lets requests with the internal key through", async () => {
    const preHandler = createInternalKeyPreHandler("secret-key");
    const reply = createReply();
    const result = await preHandler(
      { headers: { "x-internal-key": "secret-key" } } as unknown as FastifyRequest,
      reply as unknown as FastifyReply,
    );

    assert.equal(result, undefined);
    assert.equal(reply.statusCode, undefined);
  });
});

function createReply() {
  return {
    statusCode: undefined as number | undefined,
    payload: undefined as unknown,
    status(code: number) {
      this.statusCode = code;
      return this;
    },
    send(payload: unknown) {
      this.payload = payload;
      return this;
    },
  };
}
