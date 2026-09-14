import type { FastifyInstance, FastifyRequest } from "fastify";
import { getHeaderValue, sendError, sendSuccess } from "../infrastructure/httpResponse.js";
import { MailConfigError, type MailConfigService } from "./mailConfig.js";
import { authInternalRoutes } from "../generated/auth-internal/routes.gen.js";
import { createInternalKeyPreHandler } from "../admission/internalKeyGuard.js";

export function registerMailConfigRoutes(
  app: FastifyInstance,
  options: {
    internalKey: string;
    service: Pick<
      MailConfigService,
      "getSummary" | "update" | "clear" | "sendTest"
    >;
  },
) {
  const preHandler = createInternalKeyPreHandler(options.internalKey);

  app.get(authInternalRoutes.getMailConfig.fastifyPath, { preHandler }, async (_request, reply) => {
    reply.header("Cache-Control", "no-store");
    sendSuccess(reply, serializeSummary(await options.service.getSummary()));
  });

  app.put(authInternalRoutes.updateMailConfig.fastifyPath, { preHandler }, async (request, reply) => {
    reply.header("Cache-Control", "no-store");
    try {
      const body = request.body as Record<string, unknown>;
      const summary = await options.service.update({
        apiKey: stringValue(body.api_key) || undefined,
        fromName: stringValue(body.from_name),
        fromEmail: stringValue(body.from_email),
        replyTo: stringValue(body.reply_to) || undefined,
        actorId: numberValue(body.actor_id),
      });
      sendSuccess(reply, serializeSummary(summary));
    } catch (error) {
      sendMailConfigError(reply, error);
    }
  });

  app.delete(
    authInternalRoutes.clearMailConfig.fastifyPath,
    { preHandler },
    async (request, reply) => {
      reply.header("Cache-Control", "no-store");
      try {
        await options.service.clear(
          numberValue((request.body as Record<string, unknown>)?.actor_id),
        );
        sendSuccess(reply, { deleted: true });
      } catch (error) {
        sendMailConfigError(reply, error);
      }
    },
  );

  app.post(
    authInternalRoutes.sendMailConfigTest.fastifyPath,
    { preHandler },
    async (request, reply) => {
      reply.header("Cache-Control", "no-store");
      try {
        const body = request.body as Record<string, unknown>;
        await options.service.sendTest({
          recipient: stringValue(body.recipient),
          actorId: numberValue(body.actor_id),
        });
        sendSuccess(reply, { sent: true });
      } catch (error) {
        sendMailConfigError(reply, error);
      }
    },
  );
}

function serializeSummary(
  summary: Awaited<ReturnType<MailConfigService["getSummary"]>>,
) {
  return {
    configured: summary.configured,
    api_key_configured: summary.apiKeyConfigured,
    from_name: summary.fromName,
    from_email: summary.fromEmail,
    reply_to: summary.replyTo,
    config_version: summary.configVersion,
    readiness: summary.readiness,
    updated_at: summary.updatedAt,
    last_tested_at: summary.lastTestedAt,
    last_test_error_code: summary.lastTestErrorCode,
  };
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}
function numberValue(value: unknown): number {
  return typeof value === "number" && Number.isSafeInteger(value) ? value : 0;
}

function sendMailConfigError(
  reply: import("fastify").FastifyReply,
  error: unknown,
) {
  if (error instanceof MailConfigError) {
    const status =
      error.code === "invalid_input"
        ? 400
        : error.code === "not_configured"
          ? 409
          : 502;
    sendError(reply, status, error.message);
    return;
  }
  sendError(reply, 500, "邮件配置操作失败");
}
