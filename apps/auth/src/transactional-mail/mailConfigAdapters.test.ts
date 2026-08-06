import assert from "node:assert/strict";
import { describe, it } from "node:test";
import type { Pool } from "pg";
import { createPgMailConfigRepository } from "./mailConfigAdapters.js";

const candidate = {
  encryptedApiKey: "encrypted",
  fromName: "IdeaBoard",
  fromEmail: "auth@example.com",
  replyTo: "",
  readiness: "not_tested" as const,
};

describe("PostgreSQL mail config adapter", () => {
  it("updates with compare-and-swap and allocates the version in PostgreSQL", async () => {
    const queries: Array<{ sql: string; values?: unknown[] }> = [];
    let released = 0;
    const client = {
      async query(sql: string, values?: unknown[]) {
        queries.push({ sql, values });
        if (sql.includes("UPDATE auth_mail_settings")) {
          return { rows: [mailRow(9)], rowCount: 1 };
        }
        return { rows: [], rowCount: 1 };
      },
      release() {
        released += 1;
      },
    };
    const pool = { connect: async () => client } as unknown as Pool;

    const saved = await createPgMailConfigRepository(pool).put(
      candidate,
      8,
      7,
      "update_mail_config",
    );

    const mutation = queries.find(({ sql }) =>
      sql.includes("UPDATE auth_mail_settings"),
    );
    assert.match(mutation?.sql ?? "", /nextval\('auth_mail_settings_config_version_seq'/);
    assert.match(mutation?.sql ?? "", /config_version = \$6/);
    assert.equal(mutation?.values?.at(-1), 8);
    assert.equal(saved?.configVersion, 9);
    assert.equal(queries[0]?.sql, "BEGIN");
    assert.equal(queries.at(-1)?.sql, "COMMIT");
    assert.equal(released, 1);
  });

  it("returns a conflict without writing audit history", async () => {
    const queries: string[] = [];
    const client = {
      async query(sql: string) {
        queries.push(sql);
        if (sql.includes("UPDATE auth_mail_settings")) {
          return { rows: [], rowCount: 0 };
        }
        return { rows: [], rowCount: 1 };
      },
      release() {},
    };
    const pool = { connect: async () => client } as unknown as Pool;

    const saved = await createPgMailConfigRepository(pool).put(
      candidate,
      8,
      7,
      "update_mail_config",
    );

    assert.equal(saved, null);
    assert.equal(
      queries.some((sql) => sql.includes("auth_mail_settings_audit")),
      false,
    );
  });

  it("rolls back a failed transaction and always releases the client", async () => {
    const failure = new Error("write failed");
    const queries: string[] = [];
    let released = 0;
    const client = {
      async query(sql: string) {
        queries.push(sql);
        if (sql.includes("INSERT INTO auth_mail_settings ")) throw failure;
        return { rows: [], rowCount: 1 };
      },
      release() {
        released += 1;
      },
    };
    const pool = { connect: async () => client } as unknown as Pool;

    await assert.rejects(
      () =>
        createPgMailConfigRepository(pool).put(
          candidate,
          0,
          7,
          "create_mail_config",
        ),
      failure,
    );

    assert.equal(queries.at(-1), "ROLLBACK");
    assert.equal(released, 1);
  });

  it("preserves both the operation and rollback failures", async () => {
    const failure = new Error("write failed");
    const rollbackFailure = new Error("rollback failed");
    let released = 0;
    const client = {
      async query(sql: string) {
        if (sql === "ROLLBACK") throw rollbackFailure;
        if (sql.includes("INSERT INTO auth_mail_settings ")) throw failure;
        return { rows: [], rowCount: 1 };
      },
      release() {
        released += 1;
      },
    };
    const pool = { connect: async () => client } as unknown as Pool;

    await assert.rejects(
      () =>
        createPgMailConfigRepository(pool).put(
          candidate,
          0,
          7,
          "create_mail_config",
        ),
      (error) =>
        error instanceof AggregateError &&
        error.errors.includes(failure) &&
        error.errors.includes(rollbackFailure),
    );
    assert.equal(released, 1);
  });
});

function mailRow(configVersion: number) {
  return {
    encrypted_api_key: candidate.encryptedApiKey,
    from_name: candidate.fromName,
    from_email: candidate.fromEmail,
    reply_to: candidate.replyTo,
    config_version: configVersion,
    readiness: candidate.readiness,
    updated_at: new Date("2026-08-05T10:00:00Z"),
    last_tested_at: null,
    last_test_error_code: null,
  };
}
