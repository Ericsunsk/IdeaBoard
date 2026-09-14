import type { FastifyInstance, FastifyRequest } from "fastify";
import { getHeaderValue, sendError, sendSuccess } from "../infrastructure/httpResponse.js";
import {
  SocialProviderSettingsError,
  type SocialProviderSettingsService,
} from "./socialProviderSettings.js";
import { authInternalRoutes } from "../generated/auth-internal/routes.gen.js";
import { normalizeOrigin } from "../infrastructure/origin.js";
import { createInternalKeyPreHandler } from "../admission/internalKeyGuard.js";

type SocialProviderRouteService = Pick<
  SocialProviderSettingsService,
  "getSummary" | "update"
>;

type PublicSocialProviderCapabilities = {
  social: Record<string, boolean>;
  providers?: Array<{ provider: string; label: string; available: boolean }>;
};

export function isSocialProviderCapabilitiesRoute(url: string): boolean {
  return String(url || "").split("?", 1)[0] === "/api/auth/capabilities";
}

export function registerSocialProviderRoutes(
  app: FastifyInstance,
  options: {
    internalKey: string;
    trustedOrigins?: string[];
    service: SocialProviderRouteService;
    getPublicCapabilities: () => Promise<PublicSocialProviderCapabilities>;
  },
) {
  const requireInternalKey = createInternalKeyPreHandler(options.internalKey);

  const requireTrustedOrigin = async (
    request: FastifyRequest,
    reply: import("fastify").FastifyReply,
  ) => {
    const rawOrigin = getHeaderValue(request.headers.origin).trim();
    if (!rawOrigin) return;
    const origin = normalizeOrigin(rawOrigin);
    const trustedOrigins = new Set(
      (options.trustedOrigins ?? []).map(normalizeOrigin).filter(Boolean),
    );
    if (!origin || !trustedOrigins.has(origin)) {
      sendError(reply, 403, "origin is not trusted");
      return reply;
    }
    reply.header("Access-Control-Allow-Origin", origin);
    reply.header("Access-Control-Allow-Credentials", "true");
    reply.header("Vary", "Origin");
  };

  app.get(
    "/api/auth/capabilities",
    { preHandler: requireTrustedOrigin },
    async (_request, reply) => {
      reply.header("Cache-Control", "no-store");
      sendSuccess(reply, await options.getPublicCapabilities());
    },
  );

  app.get(
    authInternalRoutes.getSocialProviderCapabilities.fastifyPath,
    { preHandler: requireInternalKey },
    async (_request, reply) => {
      reply.header("Cache-Control", "no-store");
      sendSuccess(reply, await options.getPublicCapabilities());
    },
  );

  app.get(
    authInternalRoutes.getSocialProvider.fastifyPath,
    { preHandler: requireInternalKey },
    async (request, reply) => {
      reply.header("Cache-Control", "no-store");
      try {
        const { provider } = request.params as { provider: string };
        sendSuccess(reply, serializeSummary(await options.service.getSummary(provider)));
      } catch (error) {
        sendProviderSettingsError(reply, error);
      }
    },
  );

  app.put(
    authInternalRoutes.updateSocialProvider.fastifyPath,
    { preHandler: requireInternalKey },
    async (request, reply) => {
      reply.header("Cache-Control", "no-store");
      try {
        const { provider } = request.params as { provider: string };
        const body = request.body as Record<string, unknown>;
        const summary = await options.service.update(provider, {
          enabled: body.enabled === true,
          clientId: stringValue(body.client_id),
          clientSecret: stringValue(body.client_secret) || undefined,
        });
        sendSuccess(reply, serializeSummary(summary));
      } catch (error) {
        sendProviderSettingsError(reply, error);
      }
    },
  );
}

function serializeSummary(
  summary: Awaited<ReturnType<SocialProviderSettingsService["getSummary"]>>,
) {
  return {
    provider: summary.provider,
    enabled: summary.enabled,
    configured: summary.configured,
    available: summary.available,
    client_id: summary.clientId,
    client_secret_configured: summary.clientSecretConfigured,
    scopes: summary.scopes,
    config_version: summary.configVersion,
    callback_url: summary.callbackURL,
    updated_at: summary.updatedAt,
  };
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function sendProviderSettingsError(
  reply: import("fastify").FastifyReply,
  error: unknown,
) {
  if (error instanceof SocialProviderSettingsError) {
    const status =
      error.code === "unsupported_provider"
        ? 404
        : error.code === "disable_before_client_id_change" ||
            error.code === "incomplete_configuration"
          ? 409
          : error.code === "invalid_input"
            ? 400
            : 503;
    sendError(reply, status, error.message);
    return;
  }
  sendError(reply, 500, "OAuth Provider 配置操作失败");
}
