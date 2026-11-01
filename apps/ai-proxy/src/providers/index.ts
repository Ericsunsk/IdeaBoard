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
  buildOpenAiTarget,
  extractFalImage,
  extractOpenAiImage,
} from "./gateway/index.js";

import {
  UnsupportedModelError,
  type ChannelResolution,
  type ExtractedImage,
  type UpstreamTarget,
} from "./shared.js";

type FetchFn = typeof fetch;

// channel 判定：先 provider 声明，后模型前缀回落。规则见文件头。
function resolveChannel(req: {
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
    buildTarget: buildAirunTarget,
    extractImage: extractAirunImage,
  },
  "airun:openai": {
    buildTarget: buildAirunTarget,
    extractImage: extractAirunImage,
  },
  "airun:krea": {
    buildTarget: buildAirunTarget,
    extractImage: extractAirunImage,
  },
  "airun:migrated": {
    buildTarget: buildAirunTarget,
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
export interface ProviderRoute {
  readonly target: UpstreamTarget;
  readonly extractImage: ExtractImageFn;
}

// A route binds request transport and response interpretation in one selection.
// Callers never need to supply provider/model again to decode its response.
export function prepareRoute(
  req: CanonicalImageRequest,
  env: Env,
  inputs: FetchedInput[],
): ProviderRoute {
  const resolved = resolveChannel(req);
  const entry = PROVIDER_REGISTRY[`${resolved.channel}:${resolved.vendor}`];
  if (!entry) {
    throw new UnsupportedModelError(
      `unsupported provider/model: ${req.provider}/${req.model}`,
    );
  }
  return {
    target: entry.buildTarget(req, env, inputs, resolved),
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
