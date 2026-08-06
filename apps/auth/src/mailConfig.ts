import type { Pool } from "pg";
import {
  mailConfigurationTest,
  type RenderedEmail,
} from "./emailTemplates/index.js";
import {
  decryptSecret as decryptRuntimeSecret,
  encryptSecret as encryptRuntimeSecret,
} from "./secretCipher.js";

export type MailReadiness =
  "not_configured" | "not_tested" | "ready" | "unavailable";

export type MailConfigSummary = {
  configured: boolean;
  apiKeyConfigured: boolean;
  fromName: string;
  fromEmail: string;
  replyTo: string;
  configVersion: number;
  readiness: MailReadiness;
  updatedAt: string | null;
  lastTestedAt: string | null;
  lastTestErrorCode: string | null;
};

export type MailConfigInput = {
  apiKey?: string;
  fromName: string;
  fromEmail: string;
  replyTo?: string;
  actorId: number;
};

export type MailTestInput = { recipient: string; actorId: number };

export type StoredMailConfig = {
  encryptedApiKey: string;
  fromName: string;
  fromEmail: string;
  replyTo: string;
  configVersion: number;
  readiness: MailReadiness;
  updatedAt: Date;
  lastTestedAt: Date | null;
  lastTestErrorCode: string | null;
};

export type MailSendInput = {
  to: string;
  content: RenderedEmail;
  from?: string;
  replyTo?: string;
};

export type MailTransport = {
  send: (input: MailSendInput, apiKey: string) => Promise<void>;
};

export type MailConfigRepository = {
  get: () => Promise<StoredMailConfig | null>;
  put: (
    value: Omit<
      StoredMailConfig,
      "updatedAt" | "lastTestedAt" | "lastTestErrorCode"
    > & { readiness: MailReadiness },
    actorId: number,
    action: string,
  ) => Promise<StoredMailConfig>;
  markTest: (
    readiness: Extract<MailReadiness, "ready" | "unavailable">,
    errorCode: string | null,
    actorId: number,
    configVersion: number,
  ) => Promise<boolean>;
  clear: (actorId: number) => Promise<void>;
};

export const MAIL_SETTINGS_ENCRYPTION_KEY_ENV = "APP_SECRETS_ENCRYPTION_KEY";

export function createRuntimeMailConfigService(
  pool: Pool,
  source: Record<string, string | undefined> = process.env,
  fetchImpl: typeof fetch = fetch,
) {
  return new MailConfigService(
    createPgMailConfigRepository(pool),
    createResendTransport(fetchImpl),
    String(source[MAIL_SETTINGS_ENCRYPTION_KEY_ENV] || "").trim(),
  );
}

export class MailConfigService {
  constructor(
    private readonly repository: MailConfigRepository,
    private readonly transport: MailTransport,
    private readonly encryptionKey: string,
  ) {}

  async getSummary(): Promise<MailConfigSummary> {
    const value = await this.repository.get();
    if (!value) return emptySummary();
    return summarize(value);
  }

  async update(input: MailConfigInput): Promise<MailConfigSummary> {
    const fromName = input.fromName.trim();
    const fromEmail = canonicalEmail(input.fromEmail);
    const replyTo = input.replyTo ? canonicalEmail(input.replyTo) : "";
    if (
      input.actorId <= 0 ||
      !fromName ||
      !isEmail(fromEmail) ||
      (replyTo && !isEmail(replyTo))
    ) {
      throw new MailConfigError("invalid_input", "发件人配置无效");
    }
    const current = await this.repository.get();
    let encryptedApiKey = current?.encryptedApiKey ?? "";
    const nextAPIKey = input.apiKey?.trim() ?? "";
    const rotatedAPIKey = Boolean(nextAPIKey);
    if (rotatedAPIKey)
      encryptedApiKey = encryptSecret(nextAPIKey, this.encryptionKey);
    const changed =
      !current ||
      rotatedAPIKey ||
      current.fromName !== fromName ||
      current.fromEmail !== fromEmail ||
      current.replyTo !== replyTo;
    const readiness: MailReadiness =
      !encryptedApiKey || !fromName || !fromEmail
        ? "not_configured"
        : changed
          ? "not_tested"
          : current.readiness;
    const configVersion = changed
      ? (current?.configVersion ?? 0) + 1
      : current.configVersion;
    const saved = await this.repository.put(
      {
        encryptedApiKey,
        fromName,
        fromEmail,
        replyTo,
        configVersion,
        readiness,
      },
      input.actorId,
      current ? "update_mail_config" : "create_mail_config",
    );
    return summarize(saved);
  }

  async clear(actorId: number): Promise<void> {
    if (actorId <= 0) throw new MailConfigError("invalid_input", "操作者无效");
    await this.repository.clear(actorId);
  }

  async sendTest(input: MailTestInput): Promise<void> {
    const recipient = canonicalEmail(input.recipient);
    if (input.actorId <= 0 || !isEmail(recipient)) {
      throw new MailConfigError("invalid_input", "测试收件地址无效");
    }
    const current = await this.repository.get();
    if (!current?.encryptedApiKey || !current.fromName || !current.fromEmail) {
      throw new MailConfigError("not_configured", "邮件服务尚未配置");
    }
    let apiKey: string;
    try {
      apiKey = decryptSecret(current.encryptedApiKey, this.encryptionKey);
    } catch {
      throw new MailConfigError("unavailable", "邮件服务密钥不可用");
    }
    try {
      await this.transport.send(
        {
          to: recipient,
          from: `${current.fromName} <${current.fromEmail}>`,
          replyTo: current.replyTo || undefined,
          content: mailConfigurationTest(),
        },
        apiKey,
      );
      const applied = await this.repository.markTest(
        "ready",
        null,
        input.actorId,
        current.configVersion,
      );
      if (!applied) {
        throw new MailConfigError(
          "config_changed",
          "邮件配置已变化，请重新测试",
        );
      }
    } catch (error) {
      if (error instanceof MailConfigError && error.code === "config_changed") {
        throw error;
      }
      const code = normalizeProviderError(error);
      await this.repository.markTest(
        "unavailable",
        code,
        input.actorId,
        current.configVersion,
      );
      throw new MailConfigError("provider_error", "测试邮件发送失败");
    }
  }

  async send(input: MailSendInput): Promise<void> {
    const current = await this.repository.get();
    if (!current?.encryptedApiKey || !current.fromName || !current.fromEmail) {
      throw new MailConfigError("not_configured", "邮件服务尚未配置");
    }
    if (current.readiness !== "ready") {
      throw new MailConfigError(
        current.readiness === "not_configured" ? "not_configured" : "unavailable",
        "邮件服务尚未通过测试",
      );
    }
    let apiKey: string;
    try {
      apiKey = decryptSecret(current.encryptedApiKey, this.encryptionKey);
    } catch {
      throw new MailConfigError("unavailable", "邮件服务密钥不可用");
    }
    await this.transport.send(
      {
        ...input,
        from: input.from ?? `${current.fromName} <${current.fromEmail}>`,
        replyTo: input.replyTo ?? current.replyTo ?? undefined,
      },
      apiKey,
    );
  }
}

export class MailConfigError extends Error {
  constructor(
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "MailConfigError";
  }
}

export function createResendTransport(
  fetchImpl: typeof fetch = fetch,
  timeoutMs = 5_000,
): MailTransport {
  return {
    async send(input, apiKey) {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), timeoutMs);
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
        if (!response.ok) throw new Error(`resend_${response.status}`);
      } finally {
        clearTimeout(timeout);
      }
    },
  };
}

function emptySummary(): MailConfigSummary {
  return {
    configured: false,
    apiKeyConfigured: false,
    fromName: "",
    fromEmail: "",
    replyTo: "",
    configVersion: 0,
    readiness: "not_configured",
    updatedAt: null,
    lastTestedAt: null,
    lastTestErrorCode: null,
  };
}

function summarize(value: StoredMailConfig): MailConfigSummary {
  return {
    configured: Boolean(
      value.encryptedApiKey && value.fromName && value.fromEmail,
    ),
    apiKeyConfigured: Boolean(value.encryptedApiKey),
    fromName: value.fromName,
    fromEmail: value.fromEmail,
    replyTo: value.replyTo,
    configVersion: value.configVersion,
    readiness: value.readiness,
    updatedAt: value.updatedAt.toISOString(),
    lastTestedAt: value.lastTestedAt?.toISOString() ?? null,
    lastTestErrorCode: value.lastTestErrorCode,
  };
}

function canonicalEmail(value: string): string {
  return value.trim().toLowerCase();
}
function isEmail(value: string): boolean {
  return value.length <= 320 && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
}
function normalizeProviderError(error: unknown): string {
  return error instanceof Error && /^resend_\d+$/.test(error.message)
    ? error.message
    : "provider_unavailable";
}
function encryptSecret(plaintext: string, key: string): string {
  try {
    return encryptRuntimeSecret(plaintext, key);
  } catch {
    throw new MailConfigError(
      "encryption_unavailable",
      "邮件配置加密密钥未配置",
    );
  }
}

function decryptSecret(encoded: string, key: string): string {
  return decryptRuntimeSecret(encoded, key);
}

export function createPgMailConfigRepository(pool: Pool): MailConfigRepository {
  return {
    async get() {
      const result = await pool.query<MailConfigRow>(
        "SELECT encrypted_api_key, from_name, from_email, reply_to, config_version, readiness, updated_at, last_tested_at, last_test_error_code FROM auth_mail_settings WHERE id = 1",
      );
      const row = result.rows[0];
      return row
        ? {
            encryptedApiKey: row.encrypted_api_key,
            fromName: row.from_name,
            fromEmail: row.from_email,
            replyTo: row.reply_to,
            configVersion: Number(row.config_version),
            readiness: row.readiness,
            updatedAt: row.updated_at,
            lastTestedAt: row.last_tested_at,
            lastTestErrorCode: row.last_test_error_code,
          }
        : null;
    },
    async put(value, actorId, action) {
      return withMailConfigTransaction(pool, async (client) => {
        const result = await client.query<MailConfigRow>(
          "INSERT INTO auth_mail_settings (id, encrypted_api_key, from_name, from_email, reply_to, config_version, readiness, updated_at, last_tested_at, last_test_error_code) VALUES (1, $1, $2, $3, $4, $5, $6, clock_timestamp(), NULL, NULL) ON CONFLICT (id) DO UPDATE SET encrypted_api_key = EXCLUDED.encrypted_api_key, from_name = EXCLUDED.from_name, from_email = EXCLUDED.from_email, reply_to = EXCLUDED.reply_to, config_version = EXCLUDED.config_version, readiness = EXCLUDED.readiness, updated_at = clock_timestamp(), last_tested_at = CASE WHEN auth_mail_settings.config_version = EXCLUDED.config_version THEN auth_mail_settings.last_tested_at ELSE NULL END, last_test_error_code = CASE WHEN auth_mail_settings.config_version = EXCLUDED.config_version THEN auth_mail_settings.last_test_error_code ELSE NULL END RETURNING encrypted_api_key, from_name, from_email, reply_to, config_version, readiness, updated_at, last_tested_at, last_test_error_code",
          [
            value.encryptedApiKey,
            value.fromName,
            value.fromEmail,
            value.replyTo,
            value.configVersion,
            value.readiness,
          ],
        );
        await client.query(
          "INSERT INTO auth_mail_settings_audit (actor_id, action, metadata) VALUES ($1, $2, $3)",
          [actorId, action, JSON.stringify({ readiness: value.readiness })],
        );
        const row = result.rows[0];
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
    async clear(actorId) {
      await withMailConfigTransaction(pool, async (client) => {
        await client.query("DELETE FROM auth_mail_settings WHERE id = 1");
        await client.query(
          "INSERT INTO auth_mail_settings_audit (actor_id, action, metadata) VALUES ($1, $2, $3)",
          [actorId, "delete_mail_config", JSON.stringify({})],
        );
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

async function withMailConfigTransaction<T>(
  pool: Pool,
  run: (client: import("pg").PoolClient) => Promise<T>,
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
