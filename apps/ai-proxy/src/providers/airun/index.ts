// airun channel transport：host/auth/envelope/回包全部归本文件。
// vendor 文件（./google、./openai）只做 input 字段映射，不碰本文件的符号。

import type { CanonicalImageRequest, Env } from "../../contracts.js";
import { decodeBase64Native, type FetchedInput } from "../../io.js";
import {
  gatewayPolicyHeaders,
  splitImageRoles,
  UnsupportedModelError,
  UpstreamError,
  dryBodyWithImagePlaceholders,
  isQueuePayload,
  type ChannelResolution,
  type ExtractedImage,
  type UpstreamTarget,
  type VendorInput,
} from "../shared.js";
import { buildGoogleInput } from "./google.js";
import { buildOpenAiAirunInput } from "./openai.js";

type FetchFn = typeof fetch;

export const AIRUN_BASE = "https://api.cloudflare.com/client/v4";

export function airunBase(env: Env): string {
  return `${AIRUN_BASE}/accounts/${env.CF_ACCOUNT_ID}/ai/run`;
}

function migratedNote(): string {
  return "workers-ai gateway slug pending migration; routed to /ai/run";
}

// krea + 未知 airun vendor 的通用 input：prompt + provider_options Merge，
// 输入图走 image_urls（data URI，绝不透传 presigned URL；P0 待首单确认）。
function buildGenericAirunInput(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
  pathSource: string,
): VendorInput {
  const input: Record<string, unknown> = {
    prompt: req.prompt,
    ...((req.provider_options ?? {}) as Record<string, unknown>),
  };
  if (inputs.length > 0) {
    input["image_urls"] = inputs.map((i) => i.dataUri);
  }
  return {
    body: input,
    dryBody: dryBodyWithImagePlaceholders(input, inputs.length),
    path_source: pathSource,
  };
}

// workers-ai 迁移 input：沿用旧 @cf 文生图 body 字段（prompt/width/height/
// num_steps/guidance/seed/image_b64/mask_b64 + provider_options Merge），
// 仅 transport 从网关切到 /ai/run。
function buildMigratedInput(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
): VendorInput {
  const input: Record<string, unknown> = {
    prompt: req.prompt,
    negative_prompt: req.negative_prompt,
    width: req.width ?? 1024,
    height: req.height ?? 1024,
    num_steps: req.steps ?? 20,
    guidance: req.guidance_scale ?? 3.5,
    seed: req.seed,
    ...((req.provider_options ?? {}) as Record<string, unknown>),
  };
  let pathSource = "airun:migrated:workers-ai:text-to-image";
  const { images, mask } = splitImageRoles(req, inputs);
  if (images.length > 1)
    throw new UnsupportedModelError("workers-ai supports one input image");
  if (images.length >= 1) {
    input["image_b64"] = images[0].base64;
    pathSource = "airun:migrated:workers-ai:img2img";
  }
  if (mask) {
    input["mask_b64"] = mask.base64;
    pathSource = "airun:migrated:workers-ai:inpainting";
  }
  return {
    body: input,
    dryBody: dryBodyWithImagePlaceholders(input, inputs.length),
    path_source: pathSource,
  };
}

// airun vendor input 注册表：vendor → 纯 input 字段映射 builder。
// krea 无专用 builder，走通用 input（buildGenericAirunInput），故不在表内，
// 落到回退分支（动态 path_source 保持原语义）。表内只存函数引用，不含 transport。
const AIRUN_INPUT_BUILDERS: Record<
  string,
  (req: CanonicalImageRequest, inputs: FetchedInput[]) => VendorInput
> = {
  google: buildGoogleInput,
  openai: buildOpenAiAirunInput,
  migrated: buildMigratedInput,
};

function pickVendorInput(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
  resolved: ChannelResolution,
): VendorInput {
  splitImageRoles(req, inputs);
  if (req.mask_index !== undefined && resolved.vendor !== "migrated") {
    throw new UnsupportedModelError(
      `airun ${resolved.vendor} does not support an explicit mask`,
    );
  }
  const build = AIRUN_INPUT_BUILDERS[resolved.vendor];
  if (build) return build(req, inputs);
  return buildGenericAirunInput(
    req,
    inputs,
    inputs.length > 0
      ? `airun:${resolved.vendor}:edit`
      : `airun:${resolved.vendor}`,
  );
}

export function buildAirunTarget(
  req: CanonicalImageRequest,
  env: Env,
  inputs: FetchedInput[],
  resolved: ChannelResolution,
): UpstreamTarget {
  const vendor = pickVendorInput(req, inputs, resolved);
  const envelope = { model: resolved.model, input: vendor.body };
  const headers: Record<string, string> = {
    Authorization: `Bearer ${env.CF_AIG_TOKEN}`,
    "Content-Type": "application/json",
  };
  // Workers AI（@cf/）经 /ai/run 必须带网关 id；第三方模型不需要。
  if (resolved.model.startsWith("@cf/")) {
    headers["cf-aig-gateway-id"] = env.CF_GATEWAY_ID;
    Object.assign(headers, gatewayPolicyHeaders());
  }
  const dryEnvelope: Record<string, unknown> = {
    model: resolved.model,
    input: vendor.dryBody,
  };
  if (resolved.migrated) {
    dryEnvelope["migration_note"] = migratedNote();
  }
  return {
    url: airunBase(env),
    method: "POST",
    headers,
    body: JSON.stringify(envelope),
    dryBody: dryEnvelope,
    path_source: vendor.path_source,
  };
}

function firstString(...values: unknown[]): string | undefined {
  for (const v of values) {
    if (typeof v === "string" && v.length > 0) return v;
  }
  return undefined;
}

// /ai/run 回包三件套：二进制直传 / presigned URL 拉取 / base64 解码。
export async function extractAirunImage(
  res: Response,
  fetchFn: FetchFn = fetch,
): Promise<ExtractedImage> {
  const contentType = res.headers.get("content-type") || "";
  if (contentType.includes("image/")) {
    return { bytes: await res.arrayBuffer() };
  }
  const data = (await res.json().catch(() => null)) as {
    result?: unknown;
    image?: unknown;
    images?: unknown;
    data?: Array<{ b64_json?: string; url?: string }>;
    output?: unknown;
    b64_json?: unknown;
    url?: unknown;
    seed?: number;
    state?: unknown;
    success?: unknown;
    errors?: unknown;
  } | null;
  if (isQueuePayload(data)) {
    throw new UnsupportedModelError(
      "ai-run returned queue payload; sync only",
      res.status,
    );
  }
  if (data && typeof data === "object") {
    if (data["state"] === "Failed") {
      throw new UpstreamError(
        `ai-run run failed: ${JSON.stringify(data).slice(0, 500)}`,
        res.status,
      );
    }
    if (data["success"] === false) {
      throw new UpstreamError(
        `ai-run request failed: ${JSON.stringify(data["errors"] ?? data).slice(0, 500)}`,
        res.status,
      );
    }
  }
  const r =
    data && typeof data === "object"
      ? (data["result"] as Record<string, unknown> | string | null)
      : null;
  const payload = firstString(
    typeof r === "string" ? r : undefined,
    typeof r === "object" && r !== null ? (r["image"] as unknown) : undefined,
    typeof r === "object" && r !== null && Array.isArray(r["images"])
      ? ((r["images"] as unknown[])[0] as unknown)
      : undefined,
    typeof r === "object" && r !== null && Array.isArray(r["data"])
      ? ((r["data"] as Array<{ b64_json?: string; url?: string }>)[0]
          ?.b64_json ??
          (r["data"] as Array<{ b64_json?: string; url?: string }>)[0]?.url)
      : undefined,
    data && typeof data === "object" ? (data["image"] as unknown) : undefined,
    data && typeof data === "object" && Array.isArray(data["images"])
      ? ((data["images"] as unknown[])[0] as unknown)
      : undefined,
    data && typeof data === "object" && Array.isArray(data["data"])
      ? ((data["data"] as Array<{ b64_json?: string; url?: string }>)[0]
          ?.b64_json ??
          (data["data"] as Array<{ b64_json?: string; url?: string }>)[0]?.url)
      : undefined,
    data && typeof data === "object"
      ? (data["b64_json"] as unknown)
      : undefined,
    data && typeof data === "object" ? (data["url"] as unknown) : undefined,
    data && typeof data === "object" && Array.isArray(data["output"])
      ? ((data["output"] as unknown[])[0] as unknown)
      : undefined,
  );
  if (!payload) {
    throw new UpstreamError(
      "ai-run response missing image payload",
      res.status,
    );
  }
  const seed =
    (typeof r === "object" && r !== null && typeof r["seed"] === "number"
      ? (r["seed"] as number)
      : undefined) ??
    (data && typeof data.seed === "number" ? data.seed : undefined);
  if (payload.startsWith("data:")) {
    const mime =
      payload.slice(5, payload.indexOf(";") > 5 ? payload.indexOf(";") : 5) ||
      "image/png";
    return {
      bytes: await decodeBase64Native(payload, mime),
      ...(seed !== undefined ? { seed } : {}),
    };
  }
  if (payload.startsWith("http://") || payload.startsWith("https://")) {
    const imgRes = await fetchFn(payload);
    if (!imgRes.ok) {
      throw new UpstreamError(
        `fetch ai-run temporary image failed: ${imgRes.status}`,
        res.status,
      );
    }
    return {
      bytes: await imgRes.arrayBuffer(),
      ...(seed !== undefined ? { seed } : {}),
    };
  }
  const bytes = await decodeBase64Native(payload, "image/png");
  return {
    bytes,
    ...(seed !== undefined ? { seed } : {}),
  };
}
