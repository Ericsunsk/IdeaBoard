// gateway channel transport：host/auth/provider 原生 path 全部归本文件。
// vendor 文件（./fal、./openai）只做 input 字段映射 + 回包解析，不碰本文件的符号。

import type { CanonicalImageRequest, Env } from "../../contracts.js";
import type { FetchedInput } from "../../io.js";
import {
  gatewayPolicyHeaders,
  type UpstreamTarget,
  type VendorInput,
} from "../shared.js";
import { buildFalInput, extractFalImage } from "./fal.js";
import { buildOpenAiInput, extractOpenAiImage } from "./openai.js";

export const CF_GATEWAY_BASE = "https://gateway.ai.cloudflare.com/v1";

// Provider slug（URL path 第一段），必须与 CF provider-specific 规范一致。
export const PROVIDER_SLUG_FAL = "fal";
export const PROVIDER_SLUG_OPENAI = "openai";

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
    url: `${gatewayBase(env)}/${PROVIDER_SLUG_FAL}/${req.model}`,
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
): UpstreamTarget {
  const vendor = buildOpenAiInput(req, inputs);
  // Payload form determines the operation; diagnostics are derived afterwards.
  const isGenerations = !(vendor.body instanceof FormData);
  return {
    url: `${gatewayBase(env)}/${PROVIDER_SLUG_OPENAI}/${isGenerations ? OPENAI_GENERATIONS_PATH : OPENAI_EDITS_PATH}`,
    method: "POST",
    headers: gatewayHeaders(env, !(vendor.body instanceof FormData)),
    body:
      vendor.body instanceof FormData
        ? vendor.body
        : JSON.stringify(vendor.body),
    dryBody: vendor.dryBody,
    path_source: `openai:images:${isGenerations ? "generations" : "edits"}`,
  };
}

export { extractFalImage, extractOpenAiImage };
