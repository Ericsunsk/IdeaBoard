// gateway/fal vendor：fal input 字段映射 + 回包解析。
// transport（host/headers/path）已收进 ./index.ts，本文件不碰 env。
// 铁律：本文件只许出现 input 字段映射与回包解析，不许出现 host/auth/URL 拼接，
// 不许 import 兄弟 vendor 与 transport 符号（providers.test.ts 静态锁死）。

import type { CanonicalImageRequest } from "../../contracts.js";
import type { FetchedInput } from "../../io.js";
import {
  splitImageRoles,
  UnsupportedModelError,
  UpstreamError,
  defaults,
  dryBodyWithImagePlaceholders,
  isQueuePayload,
  type ExtractedImage,
  type VendorInput,
} from "../shared.js";

type FetchFn = typeof fetch;

export function buildFalInput(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
): VendorInput {
  const { width, height } = defaults(req);
  const body: Record<string, unknown> = {
    prompt: req.prompt,
    image_size: { width, height },
    num_inference_steps: req.steps ?? 28,
    guidance_scale: req.guidance_scale ?? 3.5,
    seed: req.seed,
    ...((req.provider_options ?? {}) as Record<string, unknown>),
  };
  const { images, mask } = splitImageRoles(req, inputs);
  let pathSource = "fal:text-to-image";
  if (images.length > 0) {
    const uris = images.map((i) => i.dataUri);
    body["image_url"] = uris[0];
    body["image_urls"] = uris;
    if (mask) body["mask_url"] = mask.dataUri;
    pathSource = mask ? "fal:img2img+mask" : "fal:img2img";
  }
  return {
    body,
    dryBody: dryBodyWithImagePlaceholders(body, inputs.length),
    path_source: pathSource,
  };
}

export async function extractFalImage(
  res: Response,
  fetchFn: FetchFn = fetch,
): Promise<ExtractedImage> {
  const data: unknown = await res.json().catch(() => null);
  if (res.status === 202 || isQueuePayload(data)) {
    throw new UnsupportedModelError(
      "fal returned queue payload; sync models only",
      res.status,
    );
  }
  const d = data as {
    images?: Array<{ url?: string }>;
    image?: { url?: string };
    seed?: number;
  } | null;
  const imgUrl = d?.images?.[0]?.url ?? d?.image?.url;
  if (!imgUrl)
    throw new UpstreamError("Fal response missing image URL", res.status);
  const imgRes = await fetchFn(imgUrl);
  if (!imgRes.ok)
    throw new UpstreamError(
      `fetch fal temporary image failed: ${imgRes.status}`,
      res.status,
    );
  return {
    bytes: await imgRes.arrayBuffer(),
    seed: d?.seed,
  };
}
