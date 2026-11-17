// gateway channel transport：host/auth/provider 原生 path 全部归本文件。
// vendor 文件（./fal、./openai）只做 input 字段映射 + 回包解析，不碰本文件的符号。

import type { CanonicalImageRequest, Env } from "../../contracts.js";
import type { FetchedInput } from "../../io.js";
import {
  gatewayPolicyHeaders,
  UnsupportedModelError,
  type ChannelResolution,
  type UpstreamTarget,
  type VendorInput,
} from "../shared.js";
import { buildFalInput, extractFalImage } from "./fal.js";
import {
  buildGwllmInput,
  buildOpenAiInput,
  extractOpenAiImage,
} from "./openai.js";
import {
  buildVertexAiGenerateContentInput,
  buildVertexAiPredictInput,
  extractVertexAiImage,
} from "./vertexai.js";

export const CF_GATEWAY_BASE = "https://gateway.ai.cloudflare.com/v1";

// Provider slug（URL path 第一段），必须与 CF provider-specific 规范一致。
export const PROVIDER_SLUG_FAL = "fal";
export const PROVIDER_SLUG_OPENAI = "openai";
// Custom Provider slug=gwllm, base_url=https://api.gwllm.com; keep /v1 here.
export const PROVIDER_SLUG_GWLLM = "custom-gwllm";
// Vertex 原生端点 slug：CF 文档固定为 google-vertex-ai，禁止自创变体
//（如 google-vertex）；输入只认准确 slug，见 provider declarations 精确匹配。
export const PROVIDER_SLUG_VERTEX_AI = "google-vertex-ai";

// Vertex 缺省 region：CF 文档建议用具体 region 而非 global，此处缺省
// us-central1（来源：CF AI Gateway Vertex 文档示例 region）。
export const VERTEX_DEFAULT_REGION = "us-central1";
// Vertex 缺省 resource：统一 generateContent，不按 model 嗅探判 predict。
// 理由：现有 path_source 惯例（fal 按有无图、openai 按 JSON/multipart 实际
// payload 形态定 operation，model 前缀只在 provider declarations 选 channel 时用，
// vendor/channel 内不再二次按 model 分流）；同 channel 内统一缺省更可预期，
// Imagen predict 需显式声明 vertex_resource，避免模型名 typo 静默错 body。
export const VERTEX_DEFAULT_RESOURCE = "generateContent";

// CF Gateway 上 fal 的 model path 命名空间约定：裸 model（如 birefnet）必须补
// `fal-ai/` 前缀，即 `.../fal/fal-ai/<model>`；已含 `/` 的视为显式全路径，
// 原样透传（允许 operator override）。
export const FAL_MODEL_NAMESPACE = "fal-ai";

export function falModelPath(model: string): string {
  const m = model.trim();
  return m.includes("/") ? m : `${FAL_MODEL_NAMESPACE}/${m}`;
}

// OpenAI 在 Gateway 下的子路径（替换 https://api.openai.com/v1 后的剩余路径）。
export const OPENAI_GENERATIONS_PATH = "images/generations";
export const OPENAI_EDITS_PATH = "images/edits";

export function gatewayBase(env: Env): string {
  return `${CF_GATEWAY_BASE}/${env.CF_ACCOUNT_ID}/${env.CF_GATEWAY_ID}`;
}

function gatewayHeaders(env: Env, json: boolean): Record<string, string> {
  return json
    ? {
        ...gatewayPolicyHeaders(),
        "cf-aig-authorization": `Bearer ${env.CF_AIG_TOKEN}`,
        "Content-Type": "application/json",
      }
    : {
        ...gatewayPolicyHeaders(),
        "cf-aig-authorization": `Bearer ${env.CF_AIG_TOKEN}`,
      };
}

export function buildFalTarget(
  req: CanonicalImageRequest,
  env: Env,
  inputs: FetchedInput[],
): UpstreamTarget {
  const vendor: VendorInput = buildFalInput(req, inputs);
  return {
    // 仅同步端点 /fal/{model}；绝不设置 x-fal-target-url 指向 queue.*。
    // model path 经 falModelPath 归一化（裸 model 补 fal-ai/ 前缀）。
    url: `${gatewayBase(env)}/${PROVIDER_SLUG_FAL}/${falModelPath(req.model)}`,
    method: "POST",
    headers: gatewayHeaders(env, true),
    body: JSON.stringify(vendor.body),
    dryBody: vendor.dryBody,
    path_source: vendor.path_source,
  };
}

export function buildOpenAiTarget(
  req: CanonicalImageRequest,
  env: Env,
  inputs: FetchedInput[],
  resolved: ChannelResolution,
): UpstreamTarget {
  const gwllm = resolved.vendor === PROVIDER_SLUG_GWLLM;
  const vendor = gwllm
    ? buildGwllmInput(req, inputs)
    : buildOpenAiInput(req, inputs);
  const namespace = gwllm ? `${PROVIDER_SLUG_GWLLM}/v1` : PROVIDER_SLUG_OPENAI;
  // Payload form determines the operation; diagnostics are derived afterwards.
  const isGenerations = !(vendor.body instanceof FormData);
  return {
    url: `${gatewayBase(env)}/${namespace}/${isGenerations ? OPENAI_GENERATIONS_PATH : OPENAI_EDITS_PATH}`,
    method: "POST",
    headers: gatewayHeaders(env, !(vendor.body instanceof FormData)),
    body:
      vendor.body instanceof FormData
        ? vendor.body
        : JSON.stringify(vendor.body),
    dryBody: vendor.dryBody,
    path_source: `${gwllm ? PROVIDER_SLUG_GWLLM : PROVIDER_SLUG_OPENAI}:images:${isGenerations ? "generations" : "edits"}`,
  };
}

// Vertex transport：URL 全量组装唯一位置。
// 模板：{gatewayBase}/google-vertex-ai/v1/projects/{project}/locations/{region}/publishers/google/models/{model}:{resource}
// project/region/resource 均来自 provider_options（vertex_project/vertex_region/vertex_resource）：
// 缺 vertex_project 直接 fail-closed；region 缺省 us-central1；resource 缺省
// generateContent，仅接受 generateContent 或 predict 前缀（predict 族），未知 fail-closed。
function resolveVertexOptions(req: CanonicalImageRequest): {
  project: string;
  region: string;
  resource: string;
} {
  const opts = (req.provider_options ?? {}) as Record<string, unknown>;
  const project = opts["vertex_project"];
  if (typeof project !== "string" || project.trim().length === 0) {
    throw new UnsupportedModelError(
      "vertex-ai requires provider_options.vertex_project",
    );
  }
  const rawRegion = opts["vertex_region"];
  const region =
    rawRegion === undefined || rawRegion === null
      ? VERTEX_DEFAULT_REGION
      : rawRegion;
  if (typeof region !== "string" || region.trim().length === 0) {
    throw new UnsupportedModelError(
      "vertex-ai vertex_region must be a non-empty string",
    );
  }
  const rawResource = opts["vertex_resource"];
  const resource =
    rawResource === undefined || rawResource === null
      ? VERTEX_DEFAULT_RESOURCE
      : rawResource;
  if (
    typeof resource !== "string" ||
    (resource !== "generateContent" && !resource.startsWith("predict"))
  ) {
    throw new UnsupportedModelError(
      `unsupported vertex-ai resource: ${String(rawResource)}`,
    );
  }
  const model = req.model.trim();
  if (model.length === 0 || model.includes("/") || model.includes(":")) {
    throw new UnsupportedModelError(
      "vertex-ai model must be a bare model id without '/' or ':'",
    );
  }
  return {
    project: project.trim(),
    region: region.trim(),
    resource,
  };
}

export function buildVertexAiTarget(
  req: CanonicalImageRequest,
  env: Env,
  inputs: FetchedInput[],
): UpstreamTarget {
  const { project, region, resource } = resolveVertexOptions(req);
  // body 形状与 resource 严格配对，分两个 vendor 入口，绝不混在一个 body 里。
  const vendor =
    resource === "generateContent"
      ? buildVertexAiGenerateContentInput(req, inputs)
      : buildVertexAiPredictInput(req, inputs);
  const model = req.model.trim();
  return {
    url: `${gatewayBase(env)}/${PROVIDER_SLUG_VERTEX_AI}/v1/projects/${encodeURIComponent(project)}/locations/${encodeURIComponent(region)}/publishers/google/models/${encodeURIComponent(model)}:${resource}`,
    method: "POST",
    headers: gatewayHeaders(env, true),
    body: JSON.stringify(vendor.body),
    dryBody: vendor.dryBody,
    path_source: `gateway:vertex-ai:${resource}`,
  };
}

export { extractFalImage, extractOpenAiImage, extractVertexAiImage };
