// Provider 分发根：channel 决定 transport，vendor 只做 input 字段映射。
//
// channel 判定规则（写死，providers.test.ts 锁死）：
//   1. 池条目声明优先：以 CanonicalImageRequest.provider（pool provider slug，小写）为准。
//      - "workers-ai"（旧 slug，待迁移）例外：model 形态优先——`@cf/` 开头仍进
//      airun migrated（migrated=true，dry-run 标注）；model 含 `/` 且 vendor 段为已知
//      airun/gateway 映射（google/krea/openai/fal/google-vertex-ai）时按 vendor 名直分
//      到对应 channel/vendor（等价于 provider 即该 vendor）；其余裸模型一律 fail-closed
//      报 UNSUPPORTED_MODEL 并指引改 provider（真 CF 400 根因：workers-ai +
//      google/nano-banana 系曾误进 migrated 旧 `@cf` 形 body）。
//      - "google" 进 airun/google；"krea" 进 airun（通用 input）；
//      - "openai" 看裸 model：gpt-image 系进 airun/openai，其余进 gateway/openai；
//      - "fal" 进 gateway/fal；"google-vertex-ai" 精确匹配进 gateway。
//      - "gwllm" 进 Gateway Custom Provider 的 OpenAI Images API（兼容旧 custom-gwllm）。
//   2. 其次模型前缀回落（未知/custom-<slug> provider）：对 `${provider}/${model}`
//      小写全引用做前缀匹配：google/、openai/gpt-image、krea/ 进 airun；
//      fal/、google-vertex-ai/ 进 gateway；workers-ai/、@cf/ 进 airun（migrated）；openai/ 非 gpt-image
//      进 gateway/openai；都未命中直接 UnsupportedModelError（422）。
//
// 每次 prepareRoute 只选择一次 channel/vendor，target 与 decoder 保持配对。
// shared 只含纯类型和策略；channel/vendor 不反向依赖本分发根。

import type {
  CanonicalImageRequest,
  Env,
  ImageProvider,
} from "../contracts.js";
import type { FetchedInput } from "../io.js";
import { buildAirunTarget, extractAirunImage } from "./airun/index.js";
import {
  buildFalTarget,
  buildImagesTarget,
  buildVertexAiTarget,
  extractFalImage,
  extractOpenAiImage,
  extractVertexAiImage,
} from "./gateway/index.js";

import {
  UnsupportedModelError,
  type ChannelResolution,
  type ExtractedImage,
  type UpstreamTarget,
} from "./shared.js";

type FetchFn = typeof fetch;

// Gateway vendor APIs accept native model IDs, not the routing vendor prefix.
// fal-ai/ is a model namespace, so it must survive stripping fal/.
function gatewayResolution(vendor: string, model: string): ChannelResolution {
  const prefix = `${vendor}/`;
  return {
    channel: "gateway",
    vendor,
    model: model.toLowerCase().startsWith(prefix)
      ? model.slice(prefix.length)
      : model,
  };
}

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

interface ProviderDeclaration {
  readonly provider: string;
  readonly prefixes: readonly string[];
  readonly acceptsModel?: (model: string) => boolean;
  readonly resolveModel: (
    model: string,
    fallbackProvider?: string,
  ) => ChannelResolution;
  readonly buildTarget: BuildTargetFn;
  readonly extractImage: ExtractImageFn;
}

function airunResolution(
  vendor: string,
  model: string,
  fallbackProvider?: string,
): ChannelResolution {
  // Declared providers supply the vendor prefix; unknown-provider fallback
  // preserves an explicit model reference, just as the legacy dispatch did.
  const full =
    fallbackProvider !== undefined
      ? model.includes("/")
        ? model
        : `${fallbackProvider}/${model}`
      : model.toLowerCase().startsWith(`${vendor}/`)
        ? model
        : `${vendor}/${model}`;
  return { channel: "airun", vendor, model: full };
}

// Ordered native declarations own selection, model normalization and transport
// together. OpenAI's gpt-image exception precedes its native adapter; advertised
// capabilities are derived from these same executable declarations.
const providerDeclarations: readonly ProviderDeclaration[] = [
  {
    provider: "google",
    prefixes: ["google/"],
    resolveModel: (model, fallback) =>
      airunResolution("google", model, fallback),
    buildTarget: buildAirunTarget,
    extractImage: extractAirunImage,
  },
  {
    provider: "krea",
    prefixes: ["krea/"],
    resolveModel: (model, fallback) => airunResolution("krea", model, fallback),
    buildTarget: buildAirunTarget,
    extractImage: extractAirunImage,
  },
  {
    provider: "openai",
    prefixes: ["openai/gpt-image"],
    acceptsModel: (model) =>
      model
        .toLowerCase()
        .replace(/^openai\//, "")
        .startsWith("gpt-image"),
    resolveModel: (model, fallback) =>
      airunResolution("openai", model, fallback),
    buildTarget: buildAirunTarget,
    extractImage: extractAirunImage,
  },
  {
    provider: "fal",
    prefixes: ["fal/"],
    resolveModel: (model) => gatewayResolution("fal", model),
    buildTarget: buildFalTarget,
    extractImage: extractFalImage,
  },
  {
    provider: "google-vertex-ai",
    prefixes: ["google-vertex-ai/"],
    resolveModel: (model) => gatewayResolution("google-vertex-ai", model),
    buildTarget: buildVertexAiTarget,
    extractImage: extractVertexAiImage,
  },
  {
    provider: "gwllm",
    prefixes: ["gwllm/", "custom-gwllm/"],
    resolveModel: (model) => ({
      channel: "gateway",
      vendor: "gwllm",
      model: model.replace(/^(?:custom-)?gwllm\//i, ""),
    }),
    buildTarget: buildImagesTarget,
    extractImage: extractOpenAiImage,
  },
  {
    provider: "openai",
    prefixes: ["openai/"],
    resolveModel: (model) => gatewayResolution("openai", model),
    buildTarget: buildImagesTarget,
    extractImage: extractOpenAiImage,
  },
  {
    provider: "workers-ai",
    prefixes: ["workers-ai/", "@cf/"],
    acceptsModel: (model) => model.toLowerCase().startsWith("@cf/"),
    resolveModel: (model) => ({
      channel: "airun",
      vendor: "migrated",
      model,
      migrated: true,
    }),
    buildTarget: buildAirunTarget,
    extractImage: extractAirunImage,
  },
];

function selectProvider(req: { provider: string; model: string }): {
  declaration: ProviderDeclaration;
  resolved: ChannelResolution;
} {
  let provider = req.provider.trim().toLowerCase();
  const model = req.model.trim();
  const lowerModel = model.toLowerCase();
  const legacy = provider === "workers-ai";
  if (legacy && !lowerModel.startsWith("@cf/")) {
    const slash = lowerModel.indexOf("/");
    provider = slash > 0 ? lowerModel.slice(0, slash) : "";
    // fal-ai is a model namespace, accepted only by this legacy migration.
    if (provider === "fal-ai") provider = "fal";
  }
  // Preserve published pool/snapshot identities; advertise only the CF slug.
  if (provider === "custom-gwllm") provider = "gwllm";
  const declared = providerDeclarations.find(
    (entry) =>
      entry.provider === provider &&
      (!entry.acceptsModel || entry.acceptsModel(model)),
  );
  if (declared)
    return { declaration: declared, resolved: declared.resolveModel(model) };
  if (legacy) {
    throw new UnsupportedModelError(
      `unsupported provider/model: ${req.provider}/${req.model} (workers-ai only supports @cf/ legacy models; use provider "google"/"krea"/"openai" for "google/*" etc.)`,
    );
  }
  const ref = `${provider}/${lowerModel}`;
  const fallback = providerDeclarations.find((entry) =>
    entry.prefixes.some(
      (prefix) => lowerModel.startsWith(prefix) || ref.startsWith(prefix),
    ),
  );
  if (fallback)
    return {
      declaration: fallback,
      resolved: fallback.resolveModel(model, provider),
    };
  throw new UnsupportedModelError(
    `unsupported provider/model: ${req.provider}/${req.model}`,
  );
}

export interface ProviderRoute {
  readonly target: UpstreamTarget;
  readonly extractImage: ExtractImageFn;
}

export function listProviders(): string[] {
  return [
    ...new Set(providerDeclarations.map((entry) => entry.provider)),
  ].sort();
}

// A route binds request transport and response interpretation in one selection.
// Callers never need to supply provider/model again to decode its response.
export function prepareRoute(
  req: CanonicalImageRequest,
  env: Env,
  inputs: FetchedInput[],
): ProviderRoute {
  const { declaration: entry, resolved } = selectProvider(req);
  return {
    target: entry.buildTarget(
      { ...req, model: resolved.model },
      env,
      inputs,
      resolved,
    ),
    extractImage: entry.extractImage,
  };
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
