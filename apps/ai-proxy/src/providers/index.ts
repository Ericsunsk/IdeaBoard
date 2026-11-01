// Provider 分发根：channel 决定 transport，vendor 只做 input 字段映射。
//
// channel 判定规则（写死，providers.test.ts 锁死）：
//   1. 池条目声明优先：以 CanonicalImageRequest.provider（pool provider slug，小写）为准。
//      - "workers-ai"（旧 slug，待迁移）一律进 airun，migrated=true，dry-run 标注。
//      - "google" 进 airun/google；"krea" 进 airun（通用 input）；
//      - "openai" 看裸 model：gpt-image 系进 airun/openai，其余进 gateway/openai；
//      - "fal" 进 gateway/fal。
//   2. 其次模型前缀回落（未知/custom-<slug> provider）：对 `${provider}/${model}`
//      小写全引用做前缀匹配：google/、openai/gpt-image、krea/ 进 airun；
//      fal/ 进 gateway；workers-ai/、@cf/ 进 airun（migrated）；openai/ 非 gpt-image
//      进 gateway/openai；都未命中直接 UnsupportedModelError（422）。
//
// ESM 约束：本文件顶层 import 子模块 builder；子模块回引本文件的共享符号仅限
// 函数体内运行时使用，求值期无顶层触碰。

import type {
  CanonicalImageRequest,
  Env,
  ImageProvider,
} from "../contracts.js";
import type { FetchedInput } from "../io.js";
import { buildAirunTarget, extractAirunImage } from "./airun/index.js";
import {
  buildFalTarget,
  buildOpenAiTarget,
  extractFalImage,
  extractOpenAiImage,
} from "./gateway/index.js";

export type Channel = "airun" | "gateway";

export interface ChannelResolution {
  channel: Channel;
  // channel 内 vendor：airun 下为 "google" | "openai" | "krea" | "migrated"，
  // gateway 下为 "fal" | "openai"。
  vendor: string;
  // airun envelope 用的完整 model id（author/model 或 @cf/... 原样）。
  model: string;
  // workers-ai 旧 slug 迁移标记：dry-run 必须标注。
  migrated?: boolean;
}

export class UpstreamError extends Error {
  upstreamStatus: number;
  constructor(message: string, upstreamStatus: number) {
    super(message);
    this.name = "UpstreamError";
    this.upstreamStatus = upstreamStatus;
  }
}

// queue 型一律 fail-closed 报 UNSUPPORTED_MODEL（绝不轮询）。
export class UnsupportedModelError extends Error {
  upstreamStatus?: number;
  constructor(message: string, upstreamStatus?: number) {
    super(message);
    this.name = "UnsupportedModelError";
    this.upstreamStatus = upstreamStatus;
  }
}

export interface UpstreamTarget {
  url: string;
  method: string;
  headers: Record<string, string>;
  // generate 用：string（JSON）或 FormData（gateway openai edits multipart）。
  body: string | FormData;
  // dry-run 用：可 JSON 序列化的脱敏描述（bytes 占位，不含密钥）。
  dryBody: unknown;
  path_source: string;
}

// vendor 返回：纯 input 映射结果，不含 host/auth/URL。
export interface VendorInput {
  // airun vendor 恒为可 JSON 序列化 record；gateway openai 可为 FormData。
  body: Record<string, unknown> | FormData;
  dryBody: unknown;
  path_source: string;
}

export interface ExtractedImage {
  bytes: ArrayBuffer;
  contentType: string;
  seed?: number;
}

type FetchFn = typeof fetch;

export function defaults(req: CanonicalImageRequest): {
  width: number;
  height: number;
} {
  return { width: req.width ?? 1024, height: req.height ?? 1024 };
}

export function isQueuePayload(data: unknown): boolean {
  if (!data || typeof data !== "object") return false;
  const d = data as Record<string, unknown>;
  // fal queue / 通用 queue 形态：有 request_id 但无 image，又带 queue/status 字段。
  if (typeof d["request_id"] === "string") {
    const hasImage =
      (d["images"] !== undefined && d["images"] !== null) ||
      (d["image"] !== undefined && d["image"] !== null) ||
      (d["output"] !== undefined && d["output"] !== null) ||
      (d["data"] !== undefined && d["data"] !== null);
    if (!hasImage) return true;
  }
  const status = d["status"];
  if (typeof status === "string") {
    const s = status.toUpperCase();
    if (
      s.includes("QUEUE") ||
      s === "IN_PROGRESS" ||
      s === "IN_QUEUE" ||
      s === "PENDING" ||
      s === "PROCESSING"
    ) {
      return true;
    }
  }
  if (d["queue_position"] !== undefined) return true;
  return false;
}

export function dryBodyWithImagePlaceholders(
  body: Record<string, unknown>,
  inputCount: number,
  isFal = false,
): unknown {
  if (inputCount === 0) return body;
  const clone: Record<string, unknown> = { ...body };
  // data URI / base64 绝不进 dry-run：统一占位。
  for (const k of [
    "image_b64",
    "mask_b64",
    "image_url",
    "mask_url",
    "image_urls",
    "image_input",
    "images",
  ]) {
    if (clone[k] !== undefined) {
      clone[k] = Array.isArray(clone[k])
        ? (clone[k] as unknown[]).map((_, i) => `<input-image:${i}:redacted>`)
        : `<input-image:redacted>`;
    }
  }
  void isFal;
  return clone;
}

// channel 判定：先 provider 声明，后模型前缀回落。规则见文件头。
export function resolveChannel(req: {
  provider: string;
  model: string;
}): ChannelResolution {
  const provider = req.provider.trim().toLowerCase();
  const model = req.model.trim();
  const lowerModel = model.toLowerCase();

  // 1. 池条目声明优先。
  if (provider === "workers-ai") {
    return { channel: "airun", vendor: "migrated", model, migrated: true };
  }
  if (provider === "google") {
    return {
      channel: "airun",
      vendor: "google",
      model: lowerModel.startsWith("google/") ? model : `google/${model}`,
    };
  }
  if (provider === "krea") {
    return {
      channel: "airun",
      vendor: "krea",
      model: lowerModel.startsWith("krea/") ? model : `krea/${model}`,
    };
  }
  if (provider === "openai") {
    const bare = lowerModel.startsWith("openai/")
      ? lowerModel.slice("openai/".length)
      : lowerModel;
    if (bare.startsWith("gpt-image")) {
      const full = lowerModel.startsWith("openai/") ? model : `openai/${model}`;
      return { channel: "airun", vendor: "openai", model: full };
    }
    return { channel: "gateway", vendor: "openai", model };
  }
  if (provider === "fal") {
    return { channel: "gateway", vendor: "fal", model };
  }

  // 2. 模型前缀回落（未知/custom-<slug> provider）：看全引用或裸 model 前缀。
  const ref = `${provider}/${lowerModel}`;
  const starts = (p: string): boolean =>
    lowerModel.startsWith(p) || ref.startsWith(p);
  if (starts("google/") || starts("krea/") || starts("openai/gpt-image")) {
    const vendor = starts("google/")
      ? "google"
      : starts("krea/")
        ? "krea"
        : "openai";
    // model 自带 author 前缀则原样用，否则拼全引用。
    const full = lowerModel.includes("/") ? model : `${provider}/${model}`;
    return { channel: "airun", vendor, model: full };
  }
  if (starts("fal/")) {
    return { channel: "gateway", vendor: "fal", model };
  }
  if (starts("openai/")) {
    return { channel: "gateway", vendor: "openai", model };
  }
  if (starts("workers-ai/") || starts("@cf/")) {
    return { channel: "airun", vendor: "migrated", model, migrated: true };
  }
  throw new UnsupportedModelError(
    `unsupported provider/model: ${req.provider}/${req.model}`,
  );
}

// 分发注册表：key = `${channel}:${vendor}`，value = 该组合的建目标 + 抽图实现。
// 查表发生在 resolveChannel 之后，因此“池声明优先、模型前缀回落”（含 fal 声明
// 压过模型前缀）的既有优先级全部由 resolveChannel 保持，本表只做精确命中；
// 未知组合无表项，一律 UnsupportedModelError（generate 侧 422 / dry-run 侧 400
// 的语义由 handler 映射，见 handler.ts，本层只抛异常类型与原文案）。
// airun 通道 transport/回包统一（buildAirunTarget/extractAirunImage），四个 vendor
// 表项共享同一实现；gateway 按 vendor 各自实现。表内只存函数引用，不含 host/auth/URL。
type BuildTargetFn = (
  req: CanonicalImageRequest,
  env: Env,
  inputs: FetchedInput[],
  resolved: ChannelResolution,
) => UpstreamTarget;
type ExtractImageFn = (
  res: Response,
  fetchFn?: FetchFn,
) => Promise<ExtractedImage>;
interface ProviderEntry {
  buildTarget: BuildTargetFn;
  extractImage: ExtractImageFn;
}
const PROVIDER_REGISTRY: Record<string, ProviderEntry> = {
  "airun:google": {
    buildTarget: (req, env, inputs, resolved) =>
      buildAirunTarget(req, env, inputs, resolved),
    extractImage: extractAirunImage,
  },
  "airun:openai": {
    buildTarget: (req, env, inputs, resolved) =>
      buildAirunTarget(req, env, inputs, resolved),
    extractImage: extractAirunImage,
  },
  "airun:krea": {
    buildTarget: (req, env, inputs, resolved) =>
      buildAirunTarget(req, env, inputs, resolved),
    extractImage: extractAirunImage,
  },
  "airun:migrated": {
    buildTarget: (req, env, inputs, resolved) =>
      buildAirunTarget(req, env, inputs, resolved),
    extractImage: extractAirunImage,
  },
  "gateway:fal": {
    buildTarget: buildFalTarget,
    extractImage: extractFalImage,
  },
  "gateway:openai": {
    buildTarget: buildOpenAiTarget,
    extractImage: extractOpenAiImage,
  },
};
function providerEntry(resolved: ChannelResolution): ProviderEntry | undefined {
  return PROVIDER_REGISTRY[`${resolved.channel}:${resolved.vendor}`];
}

export function buildTarget(
  req: CanonicalImageRequest,
  env: Env,
  inputs: FetchedInput[],
): UpstreamTarget {
  const resolved = resolveChannel(req);
  const entry = providerEntry(resolved);
  if (!entry) {
    throw new UnsupportedModelError(
      `unsupported provider/model: ${req.provider}/${req.model}`,
    );
  }
  return entry.buildTarget(req, env, inputs, resolved);
}

export async function extractImage(
  req: Pick<CanonicalImageRequest, "provider" | "model">,
  res: Response,
  fetchFn: FetchFn = fetch,
): Promise<ExtractedImage> {
  const resolved = resolveChannel(req);
  const entry = providerEntry(resolved);
  if (!entry) {
    throw new UnsupportedModelError(`unsupported provider: ${req.provider}`);
  }
  return entry.extractImage(res, fetchFn);
}

export function receiptFromUpstream(
  res: Response,
  provider: ImageProvider,
  model: string,
): {
  upstream_status: number;
  provider: ImageProvider;
  model: string;
  log_id?: string;
  cache_status?: string;
} {
  const logId =
    res.headers.get("cf-aig-log-id") ??
    res.headers.get("cf-aig-request-id") ??
    undefined;
  const cacheStatus =
    res.headers.get("cf-aig-cache-status") ??
    res.headers.get("cf-cache-status") ??
    undefined;
  return {
    upstream_status: res.status,
    provider,
    model,
    ...(logId ? { log_id: logId } : {}),
    ...(cacheStatus ? { cache_status: cacheStatus } : {}),
  };
}
