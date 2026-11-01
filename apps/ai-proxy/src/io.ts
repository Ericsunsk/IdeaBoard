// 输入拉取上限与类型门禁（fail-closed；原 constants.ts 并入 io，providers 侧见 providers/index.ts）。
export const MAX_INPUT_BYTES = 10 * 1024 * 1024;
export const ALLOWED_INPUT_CONTENT_TYPE_PREFIX = "image/";

export interface FetchedInput {
  bytes: ArrayBuffer;
  contentType: string;
  base64: string;
  dataUri: string;
}

export class InputFetchError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "InputFetchError";
  }
}

export class UploadError extends Error {
  upstreamStatus?: number;
  constructor(message: string, upstreamStatus?: number) {
    super(message);
    this.name = "UploadError";
    this.upstreamStatus = upstreamStatus;
  }
}

type FetchFn = typeof fetch;

// 高效 base64（分块，避免 spread 大数组爆 CPU/调用栈）。
export function encodeBase64(bytes: ArrayBuffer): string {
  const u8 = new Uint8Array(bytes);
  const CHUNK = 0x8000;
  let binary = "";
  for (let i = 0; i < u8.length; i += CHUNK) {
    binary += String.fromCharCode(...u8.subarray(i, i + CHUNK));
  }
  return btoa(binary);
}

// 零 CPU 解码：走 Worker 原生 C++ 层 fetch 解析 data URL，避免 JS 循环。
export async function decodeBase64Native(
  base64: string,
  mime = "image/png",
): Promise<ArrayBuffer> {
  const dataUrl = base64.startsWith("data:")
    ? base64
    : `data:${mime};base64,${base64}`;
  const res = await fetch(dataUrl);
  if (!res.ok) throw new Error(`base64 data URL decode failed: ${res.status}`);
  return res.arrayBuffer();
}

// presigned GET 拉输入：上限 + content-type 检查，失败 fail-closed（抛 InputFetchError）。
export async function fetchInputImage(
  url: string,
  fetchFn: FetchFn = fetch,
  maxBytes: number = MAX_INPUT_BYTES,
): Promise<FetchedInput> {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    throw new InputFetchError("invalid input image URL");
  }
  if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
    throw new InputFetchError("input image URL must be http(s)");
  }
  const res = await fetchFn(url, { method: "GET" });
  if (!res.ok) {
    throw new InputFetchError(`fetch input image failed: ${res.status}`);
  }
  const contentType = res.headers.get("content-type") || "";
  if (
    !contentType.toLowerCase().startsWith(ALLOWED_INPUT_CONTENT_TYPE_PREFIX)
  ) {
    throw new InputFetchError(
      `unsupported input content-type: ${contentType || "(empty)"}`,
    );
  }
  const declared = res.headers.get("content-length");
  if (declared !== null && Number(declared) > maxBytes) {
    throw new InputFetchError(`input image too large: ${declared} bytes`);
  }
  const bytes = await res.arrayBuffer();
  if (bytes.byteLength === 0) throw new InputFetchError("empty input image");
  if (bytes.byteLength > maxBytes) {
    throw new InputFetchError(
      `input image too large: ${bytes.byteLength} bytes`,
    );
  }
  const base64 = encodeBase64(bytes);
  return {
    bytes,
    contentType,
    base64,
    dataUri: `data:${contentType};base64,${base64}`,
  };
}

// presigned PUT 直传输出：exact key，原样 PUT，不改写 URL；失败抛 UploadError。
export async function putOutput(
  uploadUrl: string,
  body: ArrayBuffer,
  contentType: string,
  fetchFn: FetchFn = fetch,
): Promise<void> {
  const res = await fetchFn(uploadUrl, {
    method: "PUT",
    headers: { "Content-Type": contentType },
    body,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new UploadError(
      `S3 presigned PUT failed (${res.status})${text ? `: ${text.slice(0, 500)}` : ""}`,
      res.status,
    );
  }
}

// dry-run/日志脱敏：绝不输出 URL、header、token。
const SECRET_KEY_RE =
  /(token|api[_-]?key|secret|auth|signature|sig|expires|x-amz|credential)/i;
export function maskSecrets(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(maskSecrets);
  if (value !== null && typeof value === "object") {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      out[k] = SECRET_KEY_RE.test(k) ? "***" : maskSecrets(v);
    }
    return out;
  }
  return value;
}

// 安全日志：只允许 task/provider/model/duration/status/code，不含 URL/密钥/body。
export function safeLog(
  fields: Record<string, string | number | undefined>,
): void {
  const allow = new Set([
    "task_id",
    "provider",
    "model",
    "path_source",
    "upstream_status",
    "duration_ms",
    "code",
    "ok",
  ]);
  const out: Record<string, string | number> = {};
  for (const [k, v] of Object.entries(fields)) {
    if (allow.has(k) && v !== undefined) out[k] = v;
  }
  console.log(JSON.stringify({ scope: "ai-proxy", ...out }));
}
