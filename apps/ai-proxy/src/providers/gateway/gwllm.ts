// GWLLM vendor policy; OpenAI Images encoding is shared with the native adapter.

import type { CanonicalImageRequest } from "../../contracts.js";
import type { FetchedInput } from "../../io.js";
import {
  defaults,
  splitImageRoles,
  UnsupportedModelError,
  type VendorInput,
} from "../shared.js";
import { encodeImagesInput } from "./images.js";

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
  return encodeImagesInput(
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
