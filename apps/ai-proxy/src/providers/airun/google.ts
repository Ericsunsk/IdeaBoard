// airun/google vendor：nano-banana 系 input 字段映射（仅 input，不含 transport）。
// 铁律：本文件只许出现 input 字段映射，不许出现 host/auth/URL 拼接，
// 不许 import 兄弟 vendor 与 transport 符号（providers.test.ts 静态锁死）。

import type { CanonicalImageRequest } from "../../contracts.js";
import type { FetchedInput } from "../../io.js";
import {
  UnsupportedModelError,
  dryBodyWithImagePlaceholders,
  type VendorInput,
} from "../shared.js";

// additionalProperties:false：provider_options 白名单，超集直接拒绝。
const ALLOWED_OPTIONS = new Set([
  "aspect_ratio",
  "output_format",
  "resolution",
]);

const ASPECT_RATIOS = new Set([
  "match_input_image",
  "1:1",
  "2:3",
  "3:2",
  "3:4",
  "4:3",
  "4:5",
  "5:4",
  "9:16",
  "16:9",
  "21:9",
]);

const OUTPUT_FORMATS = new Set(["jpg", "png"]);

const RESOLUTIONS = new Set(["1K", "2K", "4K"]);

const MAX_IMAGE_INPUTS = 3;

export function buildGoogleInput(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
): VendorInput {
  const opts = (req.provider_options ?? {}) as Record<string, unknown>;
  const unknown = Object.keys(opts).filter((k) => !ALLOWED_OPTIONS.has(k));
  if (unknown.length > 0) {
    throw new UnsupportedModelError(
      `unsupported google option(s): ${unknown.sort().join(",")}`,
    );
  }
  const aspectRatio =
    (opts["aspect_ratio"] as string | undefined) ??
    (inputs.length > 0 ? "match_input_image" : "1:1");
  if (!ASPECT_RATIOS.has(aspectRatio)) {
    throw new UnsupportedModelError(
      `unsupported google aspect_ratio: ${String(aspectRatio)}`,
    );
  }
  const outputFormat = (opts["output_format"] as string | undefined) ?? "png";
  if (!OUTPUT_FORMATS.has(outputFormat)) {
    throw new UnsupportedModelError(
      `unsupported google output_format: ${String(outputFormat)}`,
    );
  }
  const resolution = (opts["resolution"] as string | undefined) ?? "1K";
  if (!RESOLUTIONS.has(resolution)) {
    throw new UnsupportedModelError(
      `unsupported google resolution: ${String(resolution)}`,
    );
  }
  if (inputs.length > MAX_IMAGE_INPUTS) {
    throw new UnsupportedModelError(
      `google image_input supports at most ${MAX_IMAGE_INPUTS} inputs`,
    );
  }
  const input: Record<string, unknown> = {
    prompt: req.prompt,
    aspect_ratio: aspectRatio,
    output_format: outputFormat,
    resolution,
  };
  if (inputs.length > 0) {
    input["image_input"] = inputs.map((i) => i.dataUri);
  }
  return {
    body: input,
    dryBody: dryBodyWithImagePlaceholders(input, inputs.length),
    path_source:
      inputs.length > 0 ? "airun:google:img2img" : "airun:google:text-to-image",
  };
}
