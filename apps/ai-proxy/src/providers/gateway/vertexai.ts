// gateway/vertexai vendor：Vertex AI 两种原生 input 映射 + 回包解析。
// transport（host/headers/path 拼接）已收进 ./index.ts，本文件不碰 env。
// 铁律：本文件只许出现 input 字段映射与回包解析，不许出现 host/auth/URL 拼接，
// 不许 import 兄弟 vendor 与 transport 符号（providers.test.ts 静态锁死）。

import type { CanonicalImageRequest } from "../../contracts.js";
import { decodeBase64Native, type FetchedInput } from "../../io.js";
import {
  splitImageRoles,
  UnsupportedModelError,
  UpstreamError,
  isQueuePayload,
  type ExtractedImage,
  type VendorInput,
} from "../shared.js";

// Transport keys belong to the URL, never to a native vendor body.
const TRANSPORT_KEYS = new Set([
  "vertex_project",
  "vertex_region",
  "vertex_resource",
]);

type VendorBody = Omit<VendorInput, "path_source">;

// generateContent 原生形态：{contents:[{role,parts}]}。
// prompt 必填（空串即 fail-closed，有图也不放行）；images 逐张 append
// multimodal part，字节来自已拉取输入（dataURI 约定：绝不透传 presigned URL）。
export function buildVertexAiGenerateContentInput(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
): VendorBody {
  if (!req.prompt) {
    throw new UnsupportedModelError(
      "vertex-ai generateContent requires a non-empty prompt",
    );
  }
  const { images, mask } = splitImageRoles(req, inputs);
  if (mask) {
    throw new UnsupportedModelError(
      "vertex-ai generateContent does not support mask_index",
    );
  }
  if (images.length > 4) {
    throw new UnsupportedModelError(
      "vertex-ai generateContent supports at most 4 input images",
    );
  }
  const parts: Record<string, unknown>[] = [{ text: req.prompt }];
  for (const img of images) {
    parts.push({
      inlineData: { mimeType: img.contentType, data: img.base64 },
    });
  }
  // Native options (generationConfig, safetySettings, etc.) are explicit operator
  // input. Canonical contents remain authoritative in both real and dry bodies.
  const options = Object.fromEntries(
    Object.entries(req.provider_options ?? {}).filter(
      ([key]) =>
        !TRANSPORT_KEYS.has(key) && key !== "contents" && key !== "model",
    ),
  );
  const body: Record<string, unknown> = {
    ...options,
    contents: [{ role: "user", parts }],
  };
  // dry-run 脱敏：嵌套 base64 逐个占位（顶层复用逻辑此处不适用）。
  const dryParts: Record<string, unknown>[] = [{ text: req.prompt }];
  for (let i = 0; i < images.length; i++) {
    dryParts.push({
      inlineData: {
        mimeType: images[i].contentType,
        data: `<input-image:${i}:redacted>`,
      },
    });
  }
  return {
    body,
    dryBody: { ...options, contents: [{ role: "user", parts: dryParts }] },
  };
}

// predict（Imagen）原生形态：{instances, parameters}。
// parameters 全部来自显式 provider_options（除 transport 保留键），不猜默认值；
// canonical seed/宽高如有则透传（显式 options 可覆盖，不硬编码缺省）。
export function buildVertexAiPredictInput(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
): VendorBody {
  if (!req.prompt) {
    throw new UnsupportedModelError(
      "vertex-ai predict requires a non-empty prompt",
    );
  }
  const { images, mask } = splitImageRoles(req, inputs);
  if (mask) {
    throw new UnsupportedModelError(
      "vertex-ai predict does not support mask_index",
    );
  }
  if (images.length > 1) {
    throw new UnsupportedModelError(
      "vertex-ai predict supports at most one input image",
    );
  }
  const instance: Record<string, unknown> = { prompt: req.prompt };
  if (images.length === 1) {
    instance["image"] = { bytesBase64Encoded: images[0].base64 };
  }
  const opts = (req.provider_options ?? {}) as Record<string, unknown>;
  const parameters: Record<string, unknown> = {
    ...(req.seed !== undefined ? { seed: req.seed } : {}),
    ...(req.width !== undefined ? { width: req.width } : {}),
    ...(req.height !== undefined ? { height: req.height } : {}),
  };
  for (const [k, v] of Object.entries(opts)) {
    if (TRANSPORT_KEYS.has(k)) continue;
    parameters[k] = v;
  }
  const body: Record<string, unknown> = {
    instances: [instance],
    parameters,
  };
  const dryInstance: Record<string, unknown> = { prompt: req.prompt };
  if (images.length === 1) {
    dryInstance["image"] = {
      bytesBase64Encoded: "<input-image:0:redacted>",
    };
  }
  return {
    body,
    dryBody: { instances: [dryInstance], parameters },
  };
}

function firstImageB64FromParts(parts: unknown[]): string | undefined {
  for (const p of parts) {
    if (!p || typeof p !== "object") continue;
    const part = p as Record<string, unknown>;
    for (const key of ["inlineData", "inline_data"]) {
      const inline = part[key] as Record<string, unknown> | undefined;
      if (!inline || typeof inline !== "object") continue;
      const data = inline["data"] ?? inline["bytesBase64Encoded"];
      if (typeof data === "string" && data.length > 0) return data;
    }
  }
  return undefined;
}

function firstB64FromPredictions(predictions: unknown[]): string | undefined {
  for (const p of predictions) {
    if (typeof p === "string" && p.length > 0) return p;
    if (!p || typeof p !== "object") continue;
    const pred = p as Record<string, unknown>;
    const direct = pred["bytesBase64Encoded"] ?? pred["bytes_base64_encoded"];
    if (typeof direct === "string" && direct.length > 0) return direct;
    // 嵌套形态兼容：{image:{bytesBase64Encoded}} 或双层包装。
    if (direct && typeof direct === "object") {
      const nested = (direct as Record<string, unknown>)["bytesBase64Encoded"];
      if (typeof nested === "string" && nested.length > 0) return nested;
    }
    const image = pred["image"];
    if (image && typeof image === "object") {
      const nested = (image as Record<string, unknown>)["bytesBase64Encoded"];
      if (typeof nested === "string" && nested.length > 0) return nested;
    }
  }
  return undefined;
}

// 三形态统一抽图：generateContent inlineData 图片优先；predict predictions
// 两种嵌套兼容；纯文本（无图）与空 predictions 一律 UpstreamError，绝不猜。
export async function extractVertexAiImage(
  res: Response,
): Promise<ExtractedImage> {
  const data: unknown = await res.json().catch(() => null);
  if (isQueuePayload(data)) {
    throw new UnsupportedModelError(
      "vertex-ai returned queue payload; sync only",
      res.status,
    );
  }
  const d = data as {
    candidates?: Array<{ content?: { parts?: unknown[] } }>;
    predictions?: unknown[];
  } | null;
  const parts = d?.candidates?.[0]?.content?.parts;
  if (Array.isArray(parts)) {
    const b64 = firstImageB64FromParts(parts);
    if (b64) {
      return { bytes: await decodeBase64Native(b64, "image/png") };
    }
    throw new UpstreamError(
      "vertex-ai generateContent response missing image data",
      res.status,
    );
  }
  if (Array.isArray(d?.predictions)) {
    const b64 = firstB64FromPredictions(d.predictions as unknown[]);
    if (b64) {
      return { bytes: await decodeBase64Native(b64, "image/png") };
    }
    throw new UpstreamError(
      "vertex-ai predict response missing image data",
      res.status,
    );
  }
  throw new UpstreamError("vertex-ai response missing image data", res.status);
}
