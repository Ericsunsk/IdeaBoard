import type { IncomingMessage, ServerResponse } from "node:http";
import type {
  AuthAdmissionDecision,
  AuthAdmissionPolicy,
} from "./authAdmissionPolicy.js";
import { getAuthSubpath, normalizeURLPath } from "../infrastructure/authUtils.js";
import {
  appendVaryHeader,
  getHeaderValue,
  sendNodeJSON,
  sendNodeStatus,
} from "../infrastructure/httpResponse.js";

export type AuthMiddlewareRequest = IncomingMessage & {
  socket: IncomingMessage["socket"];
};

export type AuthNodeHandler = (
  req: IncomingMessage,
  res: ServerResponse,
) => Promise<void> | void;

export type AuthEntryGuardOptions = {
  admissionPolicy: AuthAdmissionPolicy;
  authHandler: AuthNodeHandler;
};

export type AuthEntryGuard = {
  handle(
    req: AuthMiddlewareRequest,
    res: ServerResponse,
    requestIP: string,
  ): Promise<void>;
};

export function createAuthEntryGuard(
  options: AuthEntryGuardOptions,
): AuthEntryGuard {
  return {
    async handle(req, res, requestIP) {
      const decision = await options.admissionPolicy.admitRequest({
        method: req.method || "GET",
        path: getAuthSubpath(req.url || ""),
        origin: getHeaderValue(req.headers.origin),
        requestIP,
        fallbackIP: String(req.socket?.remoteAddress || ""),
      });

      applyCORSHeaders(req, res, decision);
      if (decision.allowed) {
        if (decision.preflight) {
          sendNodeStatus(res, 204);
          return;
        }
        await options.authHandler(req, res);
        return;
      }

      sendAdmissionDenial(res, decision);
    },
  };
}

export function isAuthRoute(url: string): boolean {
  const path = normalizeURLPath(url);
  return path === "/api/auth" || path.startsWith("/api/auth/");
}

function applyCORSHeaders(
  req: IncomingMessage,
  res: ServerResponse,
  decision: AuthAdmissionDecision,
): void {
  if (!decision.corsOrigin) return;
  res.setHeader("Access-Control-Allow-Origin", decision.corsOrigin);
  res.setHeader("Access-Control-Allow-Credentials", "true");
  res.setHeader(
    "Access-Control-Allow-Methods",
    "GET,POST,PUT,PATCH,DELETE,OPTIONS",
  );
  res.setHeader(
    "Access-Control-Allow-Headers",
    getHeaderValue(req.headers["access-control-request-headers"]) ||
      "Content-Type, Authorization, Cookie, X-Requested-With",
  );
  appendVaryHeader(res, "Origin");
}

function sendAdmissionDenial(
  res: ServerResponse,
  decision: Extract<AuthAdmissionDecision, { allowed: false }>,
): void {
  switch (decision.reason) {
    case "origin_not_allowed":
      sendNodeStatus(res, 403);
      return;
    case "registration_disabled":
      sendNodeJSON(res, 403, { code: 403, msg: "注册已关闭" });
      return;
    case "mail_unavailable":
      sendNodeJSON(res, 503, { code: 503, msg: "邮件服务暂不可用" });
      return;
    case "mail_not_ready":
      sendNodeJSON(res, 503, { code: 503, msg: "邮件服务尚未就绪" });
      return;
    case "rate_limited":
      res.setHeader("Retry-After", String(decision.retryAfterSeconds));
      sendNodeJSON(res, 429, {
        code: 429,
        msg: "请求过于频繁，请稍后重试",
      });
      return;
    case "provider_not_ready":
    case "admission_unavailable":
      sendNodeJSON(res, 503, { code: 503, msg: "认证服务暂不可用" });
  }
}
