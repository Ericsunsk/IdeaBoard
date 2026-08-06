import type { Pool, PoolClient } from "pg";
import { createSecretCipher } from "../infrastructure/secretCipher.js";
import {
  MailConfigService,
  type MailConfigRepository,
  type MailReadiness,
  type MailTransport,
  type StoredMailConfig,
} from "./mailConfig.js";

export type MailDeliveryErrorCode = "timeout" | "non_2xx" | "unavailable";

export class MailDeliveryError extends Error {
  constructor(
    readonly code: MailDeliveryErrorCode,
    message: string,
    options?: ErrorOptions,
  ) {
    super(message, options);
    this.name = "MailDeliveryError";
  }
}

export function createRuntimeMailConfigService(
  pool: Pool,
  encryptionKey: string,
  fetchImpl: typeof fetch = fetch,
): MailConfigService {
  return new MailConfigService(
    createPgMailConfigRepository(pool),
    createResendTransport(fetchImpl),
    createSecretCipher(encryptionKey),
  );
}

export function createResendTransport(
  fetchImpl: typeof fetch = fetch,
  timeoutMs = 5_000,
): MailTransport {
  return {
    async send(input, apiKey) {
      const controller = new AbortController();
      let didTimeout = false;
      const timeout = setTimeout(() => {
        didTimeout = true;
        controller.abort();
      }, timeoutMs);
      try {
        const response = await fetchImpl("https://api.resend.com/emails", {
          method: "POST",
          signal: controller.signal,
          headers: {
            authorization: `Bearer ${apiKey}`,
            "content-type": "application/json",
          },
          body: JSON.stringify({
            from: input.from,
            to: [input.to],
            subject: input.content.subject,
            html: input.content.html,
            text: input.content.text,
            ...(input.replyTo ? { reply_to: input.replyTo } : {}),
          }),
        });
        if (!response.ok) {
          throw new MailDeliveryError(
            "non_2xx",
            `Resend rejected email with status ${response.status}`,
          );
        }
      } catch (error) {
        if (error instanceof MailDeliveryError) throw error;
        throw new MailDeliveryError(
          didTimeout ? "timeout" : "unavailable",
          didTimeout ? "Resend request timed out" : "Resend request failed",
          { cause: error },
        );
      } finally {
        clearTimeout(timeout);
      }
    },
  };
}

export function createPgMailConfigRepository(pool: Pool): MailConfigRepository {
  return {
    async get() {
      const result = await pool.query<MailConfigRow>(
        "SELECT encrypted_api_key, from_name, from_email, reply_to, config_version, readiness, updated_at, last_tested_at, last_test_error_code FROM auth_mail_settings WHERE id = 1",
      );
      return result.rows[0] ? fromRow(result.rows[0]) : null;
    },

    async put(value, expectedConfigVersion, actorId, action) {
      return withMailConfigTransaction(pool, async (client) => {
        const values = [
          value.encryptedApiKey,
          value.fromName,
          value.fromEmail,
          value.replyTo,
          value.readiness,
        ];
        const result =
          expectedConfigVersion === 0
            ? await client.query<MailConfigRow>(
                "INSERT INTO auth_mail_settings (id, encrypted_api_key, from_name, from_email, reply_to, config_version, readiness, updated_at, last_tested_at, last_test_error_code) VALUES (1, $1, $2, $3, $4, nextval('auth_mail_settings_config_version_seq'), $5, clock_timestamp(), NULL, NULL) ON CONFLICT (id) DO NOTHING RETURNING encrypted_api_key, from_name, from_email, reply_to, config_version, readiness, updated_at, last_tested_at, last_test_error_code",
                values,
              )
            : await client.query<MailConfigRow>(
                "UPDATE auth_mail_settings SET encrypted_api_key = $1, from_name = $2, from_email = $3, reply_to = $4, readiness = $5, config_version = nextval('auth_mail_settings_config_version_seq'), updated_at = clock_timestamp(), last_tested_at = NULL, last_test_error_code = NULL WHERE id = 1 AND config_version = $6 RETURNING encrypted_api_key, from_name, from_email, reply_to, config_version, readiness, updated_at, last_tested_at, last_test_error_code",
                [...values, expectedConfigVersion],
              );
        const row = result.rows[0];
        if (!row) return null;

        await client.query(
          "INSERT INTO auth_mail_settings_audit (actor_id, action, metadata) VALUES ($1, $2, $3)",
          [actorId, action, JSON.stringify({ readiness: value.readiness })],
        );
        return fromRow(row);
      });
    },

    async markTest(readiness, errorCode, actorId, configVersion) {
      return withMailConfigTransaction(pool, async (client) => {
        const result = await client.query(
          "UPDATE auth_mail_settings SET readiness = $1, last_tested_at = clock_timestamp(), last_test_error_code = $2, updated_at = clock_timestamp() WHERE id = 1 AND config_version = $3",
          [readiness, errorCode, configVersion],
        );
        const applied = (result.rowCount ?? 0) > 0;
        await client.query(
          "INSERT INTO auth_mail_settings_audit (actor_id, action, metadata) VALUES ($1, $2, $3)",
          [
            actorId,
            applied && readiness === "ready"
              ? "test_mail_success"
              : "test_mail_failure",
            JSON.stringify({
              readiness: applied ? readiness : "not_tested",
              error_code: applied ? errorCode : "config_changed",
            }),
          ],
        );
        return applied;
      });
    },

    async clear(expectedConfigVersion, actorId) {
      return withMailConfigTransaction(pool, async (client) => {
        const result = await client.query(
          "DELETE FROM auth_mail_settings WHERE id = 1 AND config_version = $1",
          [expectedConfigVersion],
        );
        if ((result.rowCount ?? 0) === 0) return false;
        await client.query(
          "INSERT INTO auth_mail_settings_audit (actor_id, action, metadata) VALUES ($1, $2, $3)",
          [actorId, "delete_mail_config", JSON.stringify({})],
        );
        return true;
      });
    },
  };
}

type MailConfigRow = {
  encrypted_api_key: string;
  from_name: string;
  from_email: string;
  reply_to: string;
  config_version: number | string;
  readiness: MailReadiness;
  updated_at: Date;
  last_tested_at: Date | null;
  last_test_error_code: string | null;
};

function fromRow(row: MailConfigRow): StoredMailConfig {
  return {
    encryptedApiKey: row.encrypted_api_key,
    fromName: row.from_name,
    fromEmail: row.from_email,
    replyTo: row.reply_to,
    configVersion: Number(row.config_version),
    readiness: row.readiness,
    updatedAt: row.updated_at,
    lastTestedAt: row.last_tested_at,
    lastTestErrorCode: row.last_test_error_code,
  };
}

async function withMailConfigTransaction<T>(
  pool: Pool,
  run: (client: PoolClient) => Promise<T>,
): Promise<T> {
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    const result = await run(client);
    await client.query("COMMIT");
    return result;
  } catch (error) {
    try {
      await client.query("ROLLBACK");
    } catch (rollbackError) {
      throw new AggregateError(
        [error, rollbackError],
        "mail config transaction failed",
      );
    }
    throw error;
  } finally {
    client.release();
  }
}
