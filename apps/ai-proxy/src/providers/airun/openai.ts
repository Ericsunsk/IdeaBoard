// airun/openai vendor：gpt-image 系 input 字段映射（仅 input，不含 transport）。
// Merge 语义：provider_options 透传，可覆盖 quality/size/background/output_format；
// size 缺省时由 canonical width/height 合成（沿用网关时代行为）。
// 铁律：本文件只许出现 input 字段映射，不许出现 host/auth/URL 拼接，
// 不许 import 兄弟 vendor 与 transport 符号（providers.test.ts 静态锁死）。

import type { CanonicalImageRequest } from "../../contracts.js";
import type { FetchedInput } from "../../io.js";
import {
  dryBodyWithImagePlaceholders,
  type VendorInput,
} from "../index.js";

export function buildOpenAiAirunInput(
  req: CanonicalImageRequest,
  inputs: FetchedInput[],
): VendorInput {
  const width = req.width ?? 1024;
  const height = req.height ?? 1024;
  const opts = (req.provider_options ?? {}) as Record<string, unknown>;
  const input: Record<string, unknown> = {
    prompt: req.prompt,
    ...opts,
  };
  if (inputs.length > 0) {
    input["images"] = inputs.map((i) => i.dataUri);
  }
  if (input["size"] === undefined) {
    input["size"] = `${width}x${height}`;
  }
  return {
    body: input,
    dryBody: dryBodyWithImagePlaceholders(input, inputs.length),
    path_source:
      inputs.length > 0 ? "airun:openai:gpt-image:edit" : "airun:openai:gpt-image",
  };
}
