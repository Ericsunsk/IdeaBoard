import Fastify, {
  type FastifyInstance,
  type FastifyServerOptions,
} from "fastify";
import {
  createAuthEntryGuard,
  isAuthRoute,
  type AuthNodeHandler,
} from "../admission/authEntryGuard.js";
import type { AuthAdmissionPolicy } from "../admission/authAdmissionPolicy.js";
import type {
  AuthSessionAPI,
  AuthInternalCommands,
} from "../trusted-identity/authInternalCommands.js";
import { extractErrorMessage } from "../infrastructure/authUtils.js";
import { registerInternalRoutes } from "../trusted-identity/internalRoutes.js";
import { registerMailConfigRoutes } from "../transactional-mail/mailConfigRoutes.js";
import type { MailConfigService } from "../transactional-mail/mailConfig.js";
import { getErrorStatus, sendError, sendSuccess } from "../infrastructure/httpResponse.js";
import {
  isSocialProviderCapabilitiesRoute,
  registerSocialProviderRoutes,
} from "../social-identity/socialProviderRoutes.js";
import type { SocialProviderSettingsService } from "../social-identity/socialProviderSettings.js";
import { socialProviderCapabilities } from "../social-identity/catalog.js";

export type AuthAppConfig = {
  trustProxy: FastifyServerOptions["trustProxy"];
  internalKey: string;
  trustedOrigins: string[];
};

export type AuthAppDependencies = {
  config: AuthAppConfig;
  healthCheck: () => Promise<void>;
  authHandler: AuthNodeHandler;
  authApi: AuthSessionAPI;
  internalCommands: AuthInternalCommands;
  admissionPolicy: AuthAdmissionPolicy;
  mailConfigService?: Pick<MailConfigService, "getSummary" | "update" | "clear" | "sendTest">;
  socialProviderSettingsService?: Pick<
    SocialProviderSettingsService,
    "getSummary" | "update" | "getCapabilities"
  >;
};

export function createAuthApp(deps: AuthAppDependencies): FastifyInstance {
  const app = Fastify({
    bodyLimit: 1_048_576,
    trustProxy: deps.config.trustProxy,
  });
  const authEntryGuard = createAuthEntryGuard({
    admissionPolicy: deps.admissionPolicy,
    authHandler: deps.authHandler,
  });

  app.setErrorHandler((error, _request, reply) => {
    console.error("[auth] Unhandled request error", error);
    sendError(
      reply,
      getErrorStatus(error),
      extractErrorMessage(error, "internal server error"),
    );
  });

  app.addHook("onRequest", async (request, reply) => {
    if (isSocialProviderCapabilitiesRoute(request.raw.url || "")) {
      return;
    }
    if (!isAuthRoute(request.raw.url || "")) {
      return;
    }

    reply.hijack();
    await authEntryGuard.handle(request.raw, reply.raw, request.ip);
  });

  app.get("/api/health", async (_request, reply) => {
    try {
      await deps.healthCheck();
      sendSuccess(reply, { status: "ok", database: "ok" });
    } catch (error) {
      sendError(
        reply,
        503,
        extractErrorMessage(error, "database is not ready"),
      );
    }
  });

  registerInternalRoutes(app, {
    internalKey: deps.config.internalKey,
    authApi: deps.authApi,
    commands: deps.internalCommands,
  });

  if (deps.mailConfigService) {
    registerMailConfigRoutes(app, {
      internalKey: deps.config.internalKey,
      service: deps.mailConfigService,
    });
  }

  if (deps.socialProviderSettingsService) {
    registerSocialProviderRoutes(app, {
      internalKey: deps.config.internalKey,
      trustedOrigins: deps.config.trustedOrigins,
      service: deps.socialProviderSettingsService,
      getPublicCapabilities: async () => {
        const capabilities = await deps.socialProviderSettingsService!.getCapabilities();
        return {
          ...capabilities,
          providers: socialProviderCapabilities(capabilities.social),
        };
      },
    });
  }

  return app;
}
