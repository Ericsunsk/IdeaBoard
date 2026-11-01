import type { Env } from "./contracts.js";
import {
  handleDryRun,
  handleGenerate,
  handleListProviders,
} from "./handler.js";

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    try {
      const url = new URL(request.url);
      if (request.method === "POST" && url.pathname === "/v1/generate") {
        return await handleGenerate(request, env);
      }
      if (request.method === "POST" && url.pathname === "/v1/dry-run") {
        return await handleDryRun(request, env);
      }
      if (request.method === "GET" && url.pathname === "/v1/providers") {
        return await handleListProviders(request, env);
      }
      if (request.method === "GET" && url.pathname === "/health") {
        return Response.json({ ok: true });
      }
      return Response.json({ error: "Not Found" }, { status: 404 });
    } catch {
      // 全局捕获：路由层之外的任何同步/异步抛错都收敛为 500，不裸奔。
      console.log(
        JSON.stringify({
          scope: "ai-proxy",
          code: "WORKER_UNHANDLED_ERROR",
        }),
      );
      return Response.json(
        {
          task_id: "unknown",
          status: "failed",
          error: {
            code: "ADAPTER_INTERNAL_ERROR",
            message: "internal worker error",
          },
        },
        { status: 500 },
      );
    }
  },
};
