import type {
  CanonicalImageRequest,
  DryRunResponse,
  Env,
  GenerateFailure,
  GenerateSuccess,
  ImageProvider,
} from "./contracts.js";
import {
  buildTarget,
  extractImage,
  receiptFromUpstream,
  UnsupportedModelError,
  UpstreamError,
} from "./providers/index.js";
import {
  fetchInputImage,
  InputFetchError,
  maskSecrets,
  putOutput,
  safeLog,
  UploadError,
} from "./io.js";

// provider 是 registry slug（任意非空，大小写不敏感）：channel 判定与收敛
// 由 providers resolveChannel 拥有。generate 侧未知 slug 走 422
// UNSUPPORTED_MODEL；dry-run 侧收敛为 400（见 handleDryRun）。

type FetchFn = typeof fetch;

function unauthorized(taskId?: string): Response {
  const body: GenerateFailure = {
    task_id: taskId ?? "unknown",
    status: "failed",
    error: { code: "UNAUTHORIZED", message: "unauthorized" },
  };
  return Response.json(body, { status: 401 });
}

function badRequest(taskId: string, message: string): Response {
  const body: GenerateFailure = {
    task_id: taskId,
    status: "failed",
    error: { code: "BAD_REQUEST", message },
  };
  return Response.json(body, { status: 400 });
}

function checkAuth(request: Request, env: Env): boolean {
  // INTERNAL_SECRET 缺失即 fail-closed：任何请求都 401。
  if (!env.INTERNAL_SECRET) return false;
  return (
    request.headers.get("Authorization") === `Bearer ${env.INTERNAL_SECRET}`
  );
}

function validateGenerate(req: CanonicalImageRequest): string | null {
  if (!req || typeof req !== "object") return "invalid body";
  if (!req.task_id || typeof req.task_id !== "string")
    return "task_id is required";
  if (!req.provider || typeof req.provider !== "string")
    return "provider is required";
  if (!req.model || typeof req.model !== "string") return "model is required";
  if (!req.prompt || typeof req.prompt !== "string")
    return "prompt is required";
  if (!req.upload_url || typeof req.upload_url !== "string")
    return "upload_url is required";
  try {
    const u = new URL(req.upload_url);
    if (u.protocol !== "https:" && u.protocol !== "http:")
      return "upload_url must be http(s)";
  } catch {
    return "upload_url is invalid";
  }
  if (req.images !== undefined) {
    if (!Array.isArray(req.images) || req.images.length === 0)
      return "images must be a non-empty array";
    if (req.images.length > 4) return "images supports at most 4 inputs";
    for (const u of req.images) {
      if (typeof u !== "string" || u.length === 0)
        return "images must be URL strings";
    }
  }
  return null;
}

export async function handleGenerate(
  request: Request,
  env: Env,
  fetchFn: FetchFn = fetch,
): Promise<Response> {
  const start = Date.now();
  if (!checkAuth(request, env)) return unauthorized();
  let req: CanonicalImageRequest;
  try {
    req = (await request.json()) as CanonicalImageRequest;
  } catch {
    return badRequest("unknown", "invalid JSON body");
  }
  if (!checkAuth(request, env)) return unauthorized(req.task_id);
  const invalid = validateGenerate(req);
  if (invalid) return badRequest(req.task_id ?? "unknown", invalid);

  try {
    // 1. presigned GET 拉输入（上限 + content-type，失败 fail-closed）。
    const inputs =
      req.images && req.images.length > 0
        ? await Promise.all(req.images.map((u) => fetchInputImage(u, fetchFn)))
        : [];

    // 2. 建上游目标（provider-specific slug 置顶常量）。
    const target = buildTarget(req, env, inputs);

    // 3. 调 AI Gateway（secret 只在 header，不进日志/body）。
    const upstreamRes = await fetchFn(target.url, {
      method: target.method,
      headers: target.headers,
      body: target.body,
    });
    const receipt = receiptFromUpstream(upstreamRes, req.provider, req.model);

    if (!upstreamRes.ok) {
      const text = await upstreamRes.text().catch(() => "");
      // queue 型（202 / queue payload）fail-closed 报 UNSUPPORTED_MODEL。
      if (upstreamRes.status === 202) {
        safeLog({
          task_id: req.task_id,
          provider: req.provider,
          model: req.model,
          code: "UNSUPPORTED_MODEL",
          upstream_status: upstreamRes.status,
          path_source: target.path_source,
          duration_ms: Date.now() - start,
        });
        const fail: GenerateFailure = {
          task_id: req.task_id,
          status: "failed",
          error: {
            code: "UNSUPPORTED_MODEL",
            message: "queue/async model is not supported (sync only)",
            upstream_status: upstreamRes.status,
          },
        };
        return Response.json(fail, { status: 422 });
      }
      safeLog({
        task_id: req.task_id,
        provider: req.provider,
        model: req.model,
        code: "UPSTREAM_ERROR",
        upstream_status: upstreamRes.status,
        path_source: target.path_source,
        duration_ms: Date.now() - start,
      });
      const fail: GenerateFailure = {
        task_id: req.task_id,
        status: "failed",
        error: {
          code: "UPSTREAM_ERROR",
          message:
            text.slice(0, 1000) || `upstream failed: ${upstreamRes.status}`,
          upstream_status: upstreamRes.status,
        },
      };
      return Response.json(fail, {
        status:
          upstreamRes.status >= 400 && upstreamRes.status < 600
            ? upstreamRes.status
            : 502,
      });
    }

    // 4. 统一抽图（二进制 <1ms 级不动 JS 大循环；base64 解码走原生层）。
    const { bytes, contentType, seed } = await extractImage(
      req,
      upstreamRes,
      fetchFn,
    );

    // 5. presigned PUT 直传输出（exact key，原样 PUT）。
    await putOutput(req.upload_url as string, bytes, contentType, fetchFn);

    const ok: GenerateSuccess = {
      task_id: req.task_id,
      status: "completed",
      provider: req.provider as ImageProvider,
      model: req.model,
      image_url: req.public_url || (req.upload_url as string).split("?")[0],
      content_type: contentType,
      ...((seed ?? req.seed !== undefined) ? { seed: seed ?? req.seed } : {}),
      duration_ms: Date.now() - start,
      receipt,
    };
    safeLog({
      task_id: req.task_id,
      provider: req.provider,
      model: req.model,
      ok: 1,
      upstream_status: receipt.upstream_status,
      path_source: target.path_source,
      duration_ms: ok.duration_ms,
    });
    return Response.json(ok, { status: 200 });
  } catch (err) {
    if (err instanceof UnsupportedModelError) {
      safeLog({
        task_id: req.task_id,
        provider: req.provider,
        model: req.model,
        code: "UNSUPPORTED_MODEL",
        upstream_status: err.upstreamStatus,
        duration_ms: Date.now() - start,
      });
      const fail: GenerateFailure = {
        task_id: req.task_id,
        status: "failed",
        error: {
          code: "UNSUPPORTED_MODEL",
          message: err.message,
          ...(err.upstreamStatus !== undefined
            ? { upstream_status: err.upstreamStatus }
            : {}),
        },
      };
      return Response.json(fail, { status: 422 });
    }
    if (err instanceof InputFetchError) {
      safeLog({
        task_id: req.task_id,
        provider: req.provider,
        model: req.model,
        code: "INPUT_FETCH_FAILED",
        duration_ms: Date.now() - start,
      });
      const fail: GenerateFailure = {
        task_id: req.task_id,
        status: "failed",
        error: { code: "INPUT_FETCH_FAILED", message: err.message },
      };
      return Response.json(fail, { status: 400 });
    }
    if (err instanceof UploadError) {
      safeLog({
        task_id: req.task_id,
        provider: req.provider,
        model: req.model,
        code: "UPLOAD_FAILED",
        upstream_status: err.upstreamStatus,
        duration_ms: Date.now() - start,
      });
      const fail: GenerateFailure = {
        task_id: req.task_id,
        status: "failed",
        error: {
          code: "UPLOAD_FAILED",
          message: err.message,
          ...(err.upstreamStatus !== undefined
            ? { upstream_status: err.upstreamStatus }
            : {}),
        },
      };
      return Response.json(fail, { status: 502 });
    }
    if (err instanceof UpstreamError) {
      safeLog({
        task_id: req.task_id,
        provider: req.provider,
        model: req.model,
        code: "UPSTREAM_ERROR",
        upstream_status: err.upstreamStatus,
        duration_ms: Date.now() - start,
      });
      const fail: GenerateFailure = {
        task_id: req.task_id,
        status: "failed",
        error: {
          code: "UPSTREAM_ERROR",
          message: err.message,
          upstream_status: err.upstreamStatus,
        },
      };
      return Response.json(fail, {
        status:
          err.upstreamStatus >= 400 && err.upstreamStatus < 600
            ? err.upstreamStatus
            : 502,
      });
    }
    safeLog({
      task_id: req.task_id,
      provider: req.provider,
      model: req.model,
      code: "ADAPTER_INTERNAL_ERROR",
      duration_ms: Date.now() - start,
    });
    const fail: GenerateFailure = {
      task_id: req.task_id,
      status: "failed",
      error: {
        code: "ADAPTER_INTERNAL_ERROR",
        message: (err as Error)?.message || "unknown adapter error",
      },
    };
    return Response.json(fail, { status: 500 });
  }
}

export async function handleDryRun(
  request: Request,
  env: Env,
): Promise<Response> {
  if (!checkAuth(request, env)) return unauthorized();
  let req: CanonicalImageRequest;
  try {
    req = (await request.json()) as CanonicalImageRequest;
  } catch {
    return badRequest("unknown", "invalid JSON body");
  }
  if (!req.task_id || typeof req.task_id !== "string")
    return badRequest("unknown", "task_id is required");
  if (!req.provider || typeof req.provider !== "string")
    return badRequest(req.task_id, "provider is required");
  if (!req.model || typeof req.model !== "string")
    return badRequest(req.task_id, "model is required");
  if (!req.prompt || typeof req.prompt !== "string")
    return badRequest(req.task_id, "prompt is required");
  // upload_url 可选：仅校验格式，不外发、不拉取。
  if (req.upload_url !== undefined && typeof req.upload_url !== "string") {
    return badRequest(req.task_id, "upload_url must be a string");
  }

  // dry-run 不拉取 presigned GET、不调上游：image 字段统一占位，secret 全打码。
  const placeholderCount = req.images?.length ?? 0;
  const placeholders = Array.from(
    { length: Math.min(placeholderCount, 4) },
    (_, i) => ({
      bytes: new ArrayBuffer(0),
      contentType: "image/png",
      base64: `<input-image:${i}:redacted>`,
      dataUri: `<input-image:${i}:redacted>`,
    }),
  );
  let target;
  try {
    target = buildTarget(req, env, placeholders);
  } catch (err) {
    if (err instanceof UnsupportedModelError) {
      return badRequest(req.task_id, (err as Error).message);
    }
    throw err;
  }
  const body: DryRunResponse = {
    provider: req.provider as ImageProvider,
    model: req.model,
    target: {
      url: target.url,
      method: target.method,
      // headers 永不返回；body 脱敏（provider_options 内的 token/key/secret 全打码）。
      body: maskSecrets(target.dryBody),
    },
    path_source: target.path_source,
  };
  return Response.json(body, { status: 200 });
}
