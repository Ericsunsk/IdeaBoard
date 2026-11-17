// gateway/openai vendor：openai input 字段映射 + 回包解析。
// transport（host/headers/path）已收进 ./index.ts，本文件不碰 env。
// 铁律：本文件只许出现 input 字段映射与回包解析，不许出现 host/auth/URL 拼接，
// 不许 import 兄弟 vendor 与 transport 符号（providers.test.ts 静态锁死）。

import type { CanonicalImageRequest } from "../../contracts.js";
import {
  decodeBase64Native,
  maskSecrets,
  type FetchedInput,
} from "../../io.js";
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

// GWLLM follows Images API multipart: one image or ordered image[] references.
export function buildGwllmInput(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
): Omit<VendorInput, "path_source"> {
  if (!req.model.trim())
    throw new UnsupportedModelError("GWLLM requires a non-empty model id");
  if (req.mask_index !== undefined)
    throw new UnsupportedModelError("GWLLM image masks are not supported");
  if (
    Object.keys(req.provider_options ?? {}).some((key) =>
      ["model", "prompt", "stream"].includes(key),
    )
  )
    throw new UnsupportedModelError(
      "GWLLM model/prompt must use canonical fields; streaming is not supported",
    );
  if (Object.hasOwn(req.provider_options ?? {}, "image[]"))
    throw new UnsupportedModelError(
      "image roles must use canonical images and mask_index",
    );
  const { images } = splitImageRoles(req, inputs);
  if (images.length > 4)
    throw new UnsupportedModelError(
      "GWLLM supports at most 4 reference images",
    );
  const options = req.provider_options ?? {};
  if (Object.hasOwn(options, "n") && options.n !== 1)
    throw new UnsupportedModelError(
      "GWLLM requires n=1; each Worker request produces one task output",
    );
  const { width, height } = defaults(req);
  return buildImagesInput(
    {
      ...options,
      model: req.model,
      prompt: req.prompt,
      size:
        req.width !== undefined || req.height !== undefined
          ? `${width}x${height}`
          : (options.size ?? `${width}x${height}`),
      response_format: options.response_format ?? "b64_json",
      n: 1,
    },
    images,
  );
}

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
  return buildImagesInput(
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

function buildImagesInput(
  fields: Record<string, unknown>,
  images: FetchedInput[],
  mask?: FetchedInput,
): Omit<VendorInput, "path_source"> {
  if (images.length === 0) {
    return {
      body: fields,
      dryBody: dryBodyWithImagePlaceholders(fields, 0),
    };
  }
  // 有图走 edits（multipart）。via Gateway 的 multipart 映射为 P0 未确认状态；
  // 未确认前保持 fail-closed 语义（默认仍尝试，400/404 即 UPSTREAM_ERROR）。
  const form = new FormData();
  form.append("model", fields.model as string);
  form.append("prompt", fields.prompt as string);
  const imageField = images.length > 1 ? "image[]" : "image";
  for (const [index, image] of images.entries()) {
    form.append(
      imageField,
      new Blob([image.bytes], { type: image.contentType }),
      `image-${index}.png`,
    );
  }
  if (mask) {
    const second = mask;
    form.append(
      "mask",
      new Blob([second.bytes], { type: second.contentType }),
      "mask.png",
    );
  }
  for (const [key, value] of Object.entries(fields)) {
    if (key === "model" || key === "prompt") continue;
    form.append(key, typeof value === "string" ? value : JSON.stringify(value));
  }
  const previewFields = Object.fromEntries(
    Object.entries(maskSecrets(fields) as Record<string, unknown>).map(
      ([key, value]) => [
        key,
        typeof value === "string" ? value : JSON.stringify(value),
      ],
    ),
  );
  return {
    body: form,
    dryBody: {
      kind: "multipart/form-data",
      fields: {
        ...previewFields,
        [imageField]:
          images.length > 1
            ? images.map((_, index) => `<input-image:${index}:bytes-redacted>`)
            : "<input-image:0:bytes-redacted>",
        ...(mask ? { mask: `<mask:bytes-redacted>` } : {}),
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
