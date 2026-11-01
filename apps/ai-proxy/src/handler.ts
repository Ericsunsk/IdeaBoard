import type {
  CanonicalImageRequest,
  DryRunResponse,
  Env,
  GenerateFailure,
  GenerateSuccess,
  ImageProvider,
  ProvidersResponse,
} from "./contracts.js";
import {
  listProviders,
  prepareRoute,
  receiptFromUpstream,
} from "./providers/index.js";
import { UnsupportedModelError, UpstreamError } from "./providers/shared.js";
import {
  fetchInputImage,
  InputFetchError,
  maskSecrets,
  putOutput,
  outputContentType,
  safeLog,
  UploadError,
} from "./io.js";

// provider 是 registry slug（任意非空，大小写不敏感）：channel 判定与收敛
// 由 providers provider declarations 拥有。generate 侧未知 slug 走 422
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
  // 所有 400 都在日志留痕（含 JSON 解析失败与字段校验失败），只记安全字段。
  safeLog({ task_id: taskId, code: "BAD_REQUEST", message });
  const body: GenerateFailure = {
    task_id: taskId,
    status: "failed",
    error: { code: "BAD_REQUEST", message },
  };
  return Response.json(body, { status: 400 });
}

function checkAuth(request: Request, env: Env): boolean {
  // WORKER_INTERNAL_SECRET 缺失即 fail-closed：任何请求都 401。
  if (!env.WORKER_INTERNAL_SECRET) return false;
  return (
    request.headers.get("Authorization") ===
    `Bearer ${env.WORKER_INTERNAL_SECRET}`
  );
}

function validateGenerate(req: CanonicalImageRequest): string | null {
  if (!req || typeof req !== "object") return "invalid body";
  if (!req.task_id || typeof req.task_id !== "string")
    return "task_id is required";
  if (!req.provider || typeof req.provider !== "string")
    return "provider is required";
  if (!req.model || typeof req.model !== "string") return "model is required";
  // prompt 与 images 二选一必填：文生图（无 images）prompt 必须非空；
  // 有 images（cutout/img2img/edit）时 prompt 可为空串透传上游。
  const promptOrImages = validatePromptOrImages(req.prompt, req.images);
  if (promptOrImages) return promptOrImages;
  if (!req.upload_url || typeof req.upload_url !== "string")
    return "upload_url is required";
  try {
    const u = new URL(req.upload_url);
    if (u.protocol !== "https:" && u.protocol !== "http:")
      return "upload_url must be http(s)";
  } catch {
    return "upload_url is invalid";
  }
  const invalidImages = validateImages(req);
  if (invalidImages) return invalidImages;
  return null;
}

// prompt 与 images 二选一必填（cutout/img2img/edit 允许空 prompt 透传上游）：
// generate 与 dry-run 共用，文案与顺序保持一致。
function validatePromptOrImages(
  prompt: unknown,
  images: unknown,
): string | null {
  if (typeof prompt !== "string") return "prompt is required";
  const hasImages = Array.isArray(images) && images.length > 0;
  if (!prompt && !hasImages) return "prompt or images is required";
  return null;
}

function validateImages(req: CanonicalImageRequest): string | null {
  if (req.images !== undefined) {
    if (!Array.isArray(req.images) || req.images.length === 0)
      return "images must be a non-empty array";
    if (req.images.length - (req.mask_index === undefined ? 0 : 1) > 4)
      return "images supports at most 4 references";
    for (const u of req.images) {
      if (typeof u !== "string" || u.length === 0)
        return "images must be URL strings";
    }
  }
  if (
    req.mask_index !== undefined &&
    (!Number.isInteger(req.mask_index) ||
      req.mask_index < 0 ||
      req.mask_index >= (req.images?.length ?? 0) ||
      (req.images?.length ?? 0) < 2)
  ) {
    return "mask_index must identify one mask alongside an input image";
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
    const body: unknown = await request.json();
    if (body === null || typeof body !== "object" || Array.isArray(body)) {
      return badRequest("unknown", "body must be a JSON object");
    }
    req = body as CanonicalImageRequest;
  } catch {
    return badRequest("unknown", "invalid JSON body");
  }
  const invalid = validateGenerate(req);
  if (invalid)
    return badRequest(
      typeof req.task_id === "string" ? req.task_id : "unknown",
      invalid,
    );

  try {
    // 1. presigned GET 拉输入（上限 + content-type，失败 fail-closed）。
    const inputs =
      req.images && req.images.length > 0
        ? await Promise.all(req.images.map((u) => fetchInputImage(u, fetchFn)))
        : [];

    // 2. 建上游目标（provider-specific slug 置顶常量）。
    const route = prepareRoute(req, env, inputs);
    const target = route.target;

    // 3. 调 AI Gateway（secret 只在 header，不进日志/body）。
    const upstreamRes = await fetchFn(target.url, {
      method: target.method,
      headers: target.headers,
      body: target.body,
    });
    const receipt = receiptFromUpstream(upstreamRes, req.provider, req.model);

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

    if (!upstreamRes.ok) {
      const text = await upstreamRes.text().catch(() => "");
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
    const { bytes, seed } = await route.extractImage(upstreamRes, fetchFn);

    const contentType = outputContentType(bytes);

    // 5. presigned PUT 直传输出（exact key，原样 PUT）。
    await putOutput(req.upload_url as string, bytes, contentType, fetchFn);

    const ok: GenerateSuccess = {
      task_id: req.task_id,
      status: "completed",
      provider: req.provider as ImageProvider,
      model: req.model,
      image_url: req.public_url || (req.upload_url as string).split("?")[0],
      content_type: contentType,
      ...((seed ?? req.seed) !== undefined ? { seed: seed ?? req.seed } : {}),
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
        message: "internal adapter error",
      },
    };
    return Response.json(fail, { status: 500 });
  }
}

export async function handleListProviders(
  request: Request,
  env: Env,
): Promise<Response> {
  // 与 generate/dry-run 一致：WORKER_INTERNAL_SECRET bearer 鉴权，缺失即 401。
  // 无 task 上下文，401 body 不带 task_id。
  if (!checkAuth(request, env)) {
    return Response.json(
      { error: { code: "UNAUTHORIZED", message: "unauthorized" } },
      { status: 401 },
    );
  }
  const body: ProvidersResponse = { providers: listProviders() };
  return Response.json(body, { status: 200 });
}

export async function handleDryRun(
  request: Request,
  env: Env,
): Promise<Response> {
  if (!checkAuth(request, env)) return unauthorized();
  let req: CanonicalImageRequest;
  try {
    const body: unknown = await request.json();
    if (body === null || typeof body !== "object" || Array.isArray(body)) {
      return badRequest("unknown", "body must be a JSON object");
    }
    req = body as CanonicalImageRequest;
  } catch {
    return badRequest("unknown", "invalid JSON body");
  }
  if (!req.task_id || typeof req.task_id !== "string")
    return badRequest("unknown", "task_id is required");
  if (!req.provider || typeof req.provider !== "string")
    return badRequest(req.task_id, "provider is required");
  if (!req.model || typeof req.model !== "string")
    return badRequest(req.task_id, "model is required");
  const promptOrImages = validatePromptOrImages(req.prompt, req.images);
  if (promptOrImages) return badRequest(req.task_id, promptOrImages);
  // upload_url 可选：仅校验格式，不外发、不拉取。
  if (req.upload_url !== undefined && typeof req.upload_url !== "string") {
    return badRequest(req.task_id, "upload_url must be a string");
  }

  const invalidImages = validateImages(req);
  if (invalidImages) return badRequest(req.task_id, invalidImages);

  // dry-run 不拉取 presigned GET、不调上游：image 字段统一占位，secret 全打码。
  const placeholderCount = req.images?.length ?? 0;
  const placeholders = Array.from({ length: placeholderCount }, (_, i) => ({
    bytes: new ArrayBuffer(0),
    contentType: "image/png",
    base64: `<input-image:${i}:redacted>`,
    dataUri: `<input-image:${i}:redacted>`,
  }));
  let target;
  try {
    target = prepareRoute(req, env, placeholders).target;
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
