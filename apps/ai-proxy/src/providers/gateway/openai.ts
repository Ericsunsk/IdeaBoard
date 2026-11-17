// gateway/openai vendor：openai input 字段映射 + 回包解析。
// transport（host/headers/path）已收进 ./index.ts，本文件不碰 env。
// 铁律：本文件只许出现 input 字段映射与回包解析，不许出现 host/auth/URL 拼接，
// 不许 import 兄弟 vendor 与 transport 符号（providers.test.ts 静态锁死）。

import type { CanonicalImageRequest } from "../../contracts.js";
import { decodeBase64Native, type FetchedInput } from "../../io.js";
import {
  splitImageRoles,
  UnsupportedModelError,
  UpstreamError,
  defaults,
  isQueuePayload,
  type ExtractedImage,
  type VendorInput,
} from "../shared.js";

import { encodeImagesInput } from "./images.js";

type FetchFn = typeof fetch;

export function buildOpenAiInput(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
): Omit<VendorInput, "path_source"> {
  const { images, mask } = splitImageRoles(req, inputs);
  if (images.length > 1)
    throw new UnsupportedModelError(
      "gateway OpenAI edits supports one input image",
    );
  const { width, height } = defaults(req);
  const fields = {
    model: req.model,
    prompt: req.prompt,
    size: `${width}x${height}`,
    response_format: "b64_json",
  };
  const options = req.provider_options ?? {};
  // Preserve OpenAI's existing overrides: generations merge all options;
  // edits keep canonical model/prompt/size and the b64_json response format.
  return encodeImagesInput(
    images.length === 0
      ? { ...fields, ...options }
      : {
          ...fields,
          ...Object.fromEntries(
            Object.entries(options).filter(
              ([key]) => !Object.hasOwn(fields, key),
            ),
          ),
        },
    images,
    mask,
  );
}

export async function extractOpenAiImage(
  res: Response,
  fetchFn: FetchFn = fetch,
): Promise<ExtractedImage> {
  const data = (await res.json().catch(() => null)) as {
    data?: Array<{ b64_json?: string; url?: string }>;
  } | null;
  if (isQueuePayload(data)) {
    throw new UnsupportedModelError(
      "openai returned queue payload; sync only",
      res.status,
    );
  }
  const item = data?.data?.[0];
  if (item?.b64_json) {
    const bytes = await decodeBase64Native(item.b64_json, "image/png");
    return { bytes };
  }
  if (item?.url) {
    const imgRes = await fetchFn(item.url);
    if (!imgRes.ok)
      throw new UpstreamError(
        `fetch openai image url failed: ${imgRes.status}`,
        res.status,
      );
    const bytes = await imgRes.arrayBuffer();
    return {
      bytes,
    };
  }
  throw new UpstreamError("OpenAI response missing image data", res.status);
}
