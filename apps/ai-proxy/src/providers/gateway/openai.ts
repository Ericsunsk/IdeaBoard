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
  dryBodyWithImagePlaceholders,
  isQueuePayload,
  type ExtractedImage,
  type VendorInput,
} from "../shared.js";

type FetchFn = typeof fetch;

export function buildOpenAiInput(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
): Omit<VendorInput, "path_source"> {
  const { width, height } = defaults(req);
  const { images, mask } = splitImageRoles(req, inputs);
  if (images.length > 1)
    throw new UnsupportedModelError(
      "gateway OpenAI edits supports one input image",
    );
  if (inputs.length === 0) {
    const body: Record<string, unknown> = {
      model: req.model,
      prompt: req.prompt,
      size: `${width}x${height}`,
      response_format: "b64_json",
      ...((req.provider_options ?? {}) as Record<string, unknown>),
    };
    return {
      body,
      dryBody: dryBodyWithImagePlaceholders(body, 0),
    };
  }
  // 有图走 edits（multipart）。via Gateway 的 multipart 映射为 P0 未确认状态；
  // 未确认前保持 fail-closed 语义（默认仍尝试，400/404 即 UPSTREAM_ERROR）。
  const form = new FormData();
  form.append("model", req.model);
  form.append("prompt", req.prompt);
  const first = images[0];
  form.append(
    "image",
    new Blob([first.bytes], { type: first.contentType }),
    "image-0.png",
  );
  if (mask) {
    const second = mask;
    form.append(
      "mask",
      new Blob([second.bytes], { type: second.contentType }),
      "mask.png",
    );
  }
  form.append("size", `${width}x${height}`);
  form.append("response_format", "b64_json");
  if (req.provider_options) {
    for (const [k, v] of Object.entries(req.provider_options)) {
      if (
        [
          "model",
          "prompt",
          "image",
          "mask",
          "size",
          "response_format",
        ].includes(k)
      )
        continue;
      form.append(k, typeof v === "string" ? v : JSON.stringify(v));
    }
  }
  return {
    body: form,
    dryBody: {
      kind: "multipart/form-data",
      fields: {
        model: req.model,
        prompt: req.prompt,
        image: `<input-image:0:bytes-redacted>`,
        ...(mask ? { mask: `<mask:bytes-redacted>` } : {}),
        size: `${width}x${height}`,
        response_format: "b64_json",
      },
    },
  };
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
