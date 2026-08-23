import type { AuthEnvironment } from "../infrastructure/env.js";

export type LoginMetadata = {
  authUserId: string;
  ip: string;
  country: string | null;
};

export type LoginMetadataReporter = {
  report(metadata: LoginMetadata): Promise<void>;
};

const REPORT_TIMEOUT_MS = 2_000;

export function createLoginMetadataReporter(
  environment: AuthEnvironment,
): LoginMetadataReporter {
  const endpoint = new URL(
    "/internal/users/login-metadata",
    `${environment.auth.apiInternalURL.replace(/\/$/, "")}/`,
  ).toString();

  return {
    async report(metadata) {
      if (!metadata.authUserId || !metadata.ip) return;
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), REPORT_TIMEOUT_MS);
      try {
        const response = await fetch(endpoint, {
          method: "POST",
          headers: {
            "content-type": "application/json",
            "x-internal-key": environment.auth.internalKey,
          },
          body: JSON.stringify({
            auth_user_id: metadata.authUserId,
            ip: metadata.ip,
            country: metadata.country,
          }),
          signal: controller.signal,
        });
        if (!response.ok) {
          console.warn(
            `[auth] login metadata report rejected (${response.status})`,
          );
        }
      } catch (error) {
        console.warn("[auth] login metadata report failed", error);
      } finally {
        clearTimeout(timeout);
      }
    },
  };
}

