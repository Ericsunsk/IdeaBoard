import Fastify, {
  type FastifyInstance,
  type FastifyServerOptions,
} from "fastify";
import {
  createAuthEntryGuard,
  isAuthRoute,
  type AuthNodeHandler,
} from "./authEntryGuard.js";
import type { AuthGuardSettings } from "./authGuardSettings.js";
import type {
  AuthInternalCommands,
  InternalAuthAPI,
} from "./authInternalCommands.js";
import { extractErrorMessage } from "./authUtils.js";
import { registerInternalRoutes } from "./internalRoutes.js";
import { registerMailConfigRoutes } from "./mailConfigRoutes.js";
import type { MailConfigService } from "./mailConfig.js";
import { getErrorStatus, sendError, sendSuccess } from "./httpResponse.js";
import { createAuthDispatchHandler } from "./publicRegistration.js";
import type { TrustedIdentityRepository } from "./trustedIdentity.js";

export type AuthAppConfig = {
  trustProxy: FastifyServerOptions["trustProxy"];
  internalKey: string;
  trustedOrigins: string[];
};

export type AuthAppDependencies = {
  config: AuthAppConfig;
  healthCheck: () => Promise<void>;
  authHandler: AuthNodeHandler;
  authApi: InternalAuthAPI;
  repository: TrustedIdentityRepository;
  internalCommands: AuthInternalCommands;
  getAuthGuardSettings: () => Promise<AuthGuardSettings>;
  mailConfigService?: Pick<MailConfigService, "getSummary" | "update" | "clear" | "sendTest">;
};

export function createAuthApp(deps: AuthAppDependencies): FastifyInstance {
  const app = Fastify({
    bodyLimit: 1_048_576,
    trustProxy: deps.config.trustProxy,
  });
  const authEntryGuard = createAuthEntryGuard({
    trustedOrigins: deps.config.trustedOrigins,
    getAuthGuardSettings: deps.getAuthGuardSettings,
    authHandler: createAuthDispatchHandler(deps.authHandler),
    getMailReadiness: deps.mailConfigService
      ? async () => (await deps.mailConfigService!.getSummary()).readiness
      : undefined,
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

  return app;
}
