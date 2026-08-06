import {
  mailConfigurationTest,
  type RenderedEmail,
} from "./emailTemplates/index.js";

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

export type MailSecretCipher = {
  encrypt(plaintext: string): string;
  decrypt(encoded: string): string;
};

export type MailConfigRepository = {
  get: () => Promise<StoredMailConfig | null>;
  put: (
    value: Omit<
      StoredMailConfig,
      "configVersion" | "updatedAt" | "lastTestedAt" | "lastTestErrorCode"
    > & { readiness: MailReadiness },
    expectedConfigVersion: number,
    actorId: number,
    action: string,
  ) => Promise<StoredMailConfig | null>;
  markTest: (
    readiness: Extract<MailReadiness, "ready" | "unavailable">,
    errorCode: string | null,
    actorId: number,
    configVersion: number,
  ) => Promise<boolean>;
  clear: (expectedConfigVersion: number, actorId: number) => Promise<boolean>;
};

const MAX_CONFIG_MUTATION_ATTEMPTS = 3;

export class MailConfigService {
  constructor(
    private readonly repository: MailConfigRepository,
    private readonly transport: MailTransport,
    private readonly secretCipher: MailSecretCipher,
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
    const nextAPIKey = input.apiKey?.trim() ?? "";
    const rotatedAPIKey = Boolean(nextAPIKey);
    const rotatedEncryptedAPIKey = rotatedAPIKey
      ? encryptSecret(nextAPIKey, this.secretCipher)
      : "";

    for (let attempt = 0; attempt < MAX_CONFIG_MUTATION_ATTEMPTS; attempt += 1) {
      const current = await this.repository.get();
      const encryptedApiKey = rotatedAPIKey
        ? rotatedEncryptedAPIKey
        : (current?.encryptedApiKey ?? "");
      const changed =
        !current ||
        rotatedAPIKey ||
        current.fromName !== fromName ||
        current.fromEmail !== fromEmail ||
        current.replyTo !== replyTo;
      if (!changed && current) return summarize(current);

      const readiness: MailReadiness =
        !encryptedApiKey || !fromName || !fromEmail
          ? "not_configured"
          : "not_tested";
      const saved = await this.repository.put(
        {
          encryptedApiKey,
          fromName,
          fromEmail,
          replyTo,
          readiness,
        },
        current?.configVersion ?? 0,
        input.actorId,
        current ? "update_mail_config" : "create_mail_config",
      );
      if (saved) return summarize(saved);
    }

    throw new MailConfigError("config_changed", "邮件配置已变化，请重试");
  }

  async clear(actorId: number): Promise<void> {
    if (actorId <= 0) throw new MailConfigError("invalid_input", "操作者无效");
    for (let attempt = 0; attempt < MAX_CONFIG_MUTATION_ATTEMPTS; attempt += 1) {
      const current = await this.repository.get();
      if (!current) return;
      if (await this.repository.clear(current.configVersion, actorId)) return;
    }
    throw new MailConfigError("config_changed", "邮件配置已变化，请重试");
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
      apiKey = this.secretCipher.decrypt(current.encryptedApiKey);
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
    } catch (deliveryError) {
      try {
        await this.repository.markTest(
          "unavailable",
          normalizeProviderError(deliveryError),
          input.actorId,
          current.configVersion,
        );
      } catch (persistenceError) {
        throw new AggregateError(
          [deliveryError, persistenceError],
          "mail test delivery and readiness persistence failed",
        );
      }
      throw new MailConfigError("provider_error", "测试邮件发送失败", {
        cause: deliveryError,
      });
    }

    let applied: boolean;
    try {
      applied = await this.repository.markTest(
        "ready",
        null,
        input.actorId,
        current.configVersion,
      );
    } catch (error) {
      throw new MailConfigError("repository_error", "邮件测试状态保存失败", {
        cause: error,
      });
    }
    if (!applied) {
      throw new MailConfigError(
        "config_changed",
        "邮件配置已变化，请重新测试",
      );
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
      apiKey = this.secretCipher.decrypt(current.encryptedApiKey);
    } catch {
      throw new MailConfigError("unavailable", "邮件服务密钥不可用");
    }
    try {
      await this.transport.send(
        {
          ...input,
          from: input.from ?? `${current.fromName} <${current.fromEmail}>`,
          replyTo: input.replyTo ?? current.replyTo ?? undefined,
        },
        apiKey,
      );
    } catch (error) {
      throw new MailConfigError("provider_error", "邮件发送失败", {
        cause: error,
      });
    }
  }
}

export class MailConfigError extends Error {
  constructor(
    readonly code: string,
    message: string,
    options?: ErrorOptions,
  ) {
    super(message, options);
    this.name = "MailConfigError";
  }
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
  if (
    error instanceof Error &&
    "code" in error &&
    (error.code === "timeout" ||
      error.code === "non_2xx" ||
      error.code === "unavailable")
  ) {
    return `provider_${error.code}`;
  }
  return "provider_unavailable";
}
function encryptSecret(plaintext: string, secretCipher: MailSecretCipher): string {
  try {
    return secretCipher.encrypt(plaintext);
  } catch {
    throw new MailConfigError(
      "encryption_unavailable",
      "邮件配置加密密钥未配置",
    );
  }
}
