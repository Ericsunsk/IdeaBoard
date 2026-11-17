// OpenAI Images JSON/multipart encoding and redacted wire preview.
// Vendor modules resolve their field policies before calling this encoder.

import { maskSecrets, type FetchedInput } from "../../io.js";
import { dryBodyWithImagePlaceholders, type VendorInput } from "../shared.js";

export function encodeImagesInput(
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
