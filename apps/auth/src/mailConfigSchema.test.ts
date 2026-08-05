import assert from "node:assert/strict";
import { describe, it } from "node:test";
import type { Pool } from "pg";
import {
  configureMailConfigRuntimePrivileges,
  ensureMailConfigSchema,
  verifyMailConfigRuntimePrivileges,
} from "./mailConfigSchema.js";

describe("mail config schema and privileges", () => {
  it("creates singleton encrypted settings and append-only audit tables", async () => {
    const queries: string[] = [];
    const pool = {
      query: async (sql: string) => {
        queries.push(sql);
        return { rows: [] };
      },
    } as unknown as Pool;

    await ensureMailConfigSchema(pool);

    const sql = queries.join("\n");
    assert.match(sql, /CREATE TABLE IF NOT EXISTS auth_mail_settings/);
    assert.match(sql, /CHECK \(id = 1\)/);
    assert.match(sql, /encrypted_api_key TEXT NOT NULL/);
    assert.match(sql, /config_version BIGINT NOT NULL/);
    assert.match(sql, /auth_mail_settings_audit_append_only/);
  });

  it("grants runtime writes only to settings and audit inserts", async () => {
    const queries: string[] = [];
    const pool = {
      query: async (sql: string) => {
        queries.push(sql);
        return { rows: [] };
      },
    } as unknown as Pool;

    await configureMailConfigRuntimePrivileges(pool, "ideaboard_auth");

    assert.match(
      queries[0] || "",
      /SELECT, INSERT, UPDATE, DELETE ON auth_mail_settings/,
    );
    assert.match(
      queries[1] || "",
      /SELECT, INSERT ON auth_mail_settings_audit/,
    );
    assert.doesNotMatch(queries[1] || "", /UPDATE|DELETE|TRUNCATE/);
  });

  it("rejects runtime access that can mutate audit history", async () => {
    const pool = {
      query: async () => ({
        rows: [
          {
            can_manage_settings: true,
            can_insert_audit: true,
            can_mutate_audit: true,
          },
        ],
      }),
    } as unknown as Pool;

    await assert.rejects(
      () => verifyMailConfigRuntimePrivileges(pool),
      /Unsafe Auth mail config privileges/,
    );
  });
});
