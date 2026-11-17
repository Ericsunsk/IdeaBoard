// Internal provider vocabulary and pure image/transport policies. This module
// imports no channel or vendor implementation, keeping route composition acyclic.
import type { CanonicalImageRequest } from "../contracts.js";
import type { FetchedInput } from "../io.js";

export type Channel = "airun" | "gateway";

export interface ChannelResolution {
  channel: Channel;
  // channel 内 vendor：airun 下为 "google" | "openai" | "krea" | "migrated"，
  // gateway 下为 "fal" | "openai" | "google-vertex-ai" | "custom-gwllm"。
  vendor: string;
  // Native model: airun uses author/model or @cf/...; gateway removes only its routing prefix.
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
  seed?: number;
}

// A mask is a role, never inferred from the number or order of images.
export function splitImageRoles(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
): { images: FetchedInput[]; mask?: FetchedInput } {
  const reserved = [
    "images",
    "image",
    "image_url",
    "image_urls",
    "image_input",
    "image_b64",
    "mask",
    "mask_url",
    "mask_b64",
    "mask_index",
  ];
  if (
    Object.keys(req.provider_options ?? {}).some((k) => reserved.includes(k))
  ) {
    throw new UnsupportedModelError(
      "image roles must use canonical images and mask_index",
    );
  }
  if (req.mask_index === undefined) return { images: inputs };
  if (
    !Number.isInteger(req.mask_index) ||
    req.mask_index < 0 ||
    req.mask_index >= inputs.length ||
    inputs.length < 2
  ) {
    throw new UnsupportedModelError(
      "mask_index must identify one mask alongside an input image",
    );
  }
  return {
    images: inputs.filter((_, i) => i !== req.mask_index),
    mask: inputs[req.mask_index],
  };
}

// Applied only by transports that actually use AI Gateway. Caller options
// are request body fields and cannot override this policy.
export function gatewayPolicyHeaders(): Record<string, string> {
  return { "cf-aig-skip-cache": "true", "cf-aig-collect-log-payload": "false" };
}

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
  return clone;
}
