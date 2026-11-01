// images contains ordinary references and an optional explicitly indexed mask.
// mask_index is zero-based; absent means every image is an ordinary reference.
// provider 是 Service Model Pool 条目声明（registry slug，小写），model 是裸 model 名；
// channel 判定以 provider 声明优先（见 src/providers/index.ts resolveChannel）。

export type ImageProvider =
  | "workers-ai"
  | "fal"
  | "openai"
  | "google"
  | "krea"
  // registry 自定义 slug（Go 侧合成 custom-<slug>）同样允许：dispatch 走模型前缀回落，
  // 未命中直接 422，未知永不 400 误导。
  | (string & {});

export interface Env {
  CF_ACCOUNT_ID: string;
  CF_GATEWAY_ID: string;
  CF_AIG_TOKEN: string;
  WORKER_INTERNAL_SECRET: string;
}

export interface CanonicalImageRequest {
  task_id: string;
  provider: ImageProvider;
  model: string;
  prompt: string;
  negative_prompt?: string;
  width?: number;
  height?: number;
  steps?: number;
  guidance_scale?: number;
  seed?: number;
  images?: string[];
  mask_index?: number;
  provider_options?: Record<string, unknown>;
  // generate 必填；dry-run 可选。
  upload_url?: string;
  public_url?: string;
}

export type ErrorCode =
  | "BAD_REQUEST"
  | "UNAUTHORIZED"
  | "UPSTREAM_ERROR"
  | "UNSUPPORTED_MODEL"
  | "INPUT_FETCH_FAILED"
  | "UPLOAD_FAILED"
  | "ADAPTER_INTERNAL_ERROR";

export interface GenerateSuccess {
  task_id: string;
  status: "completed";
  provider: ImageProvider;
  model: string;
  image_url: string;
  content_type: string;
  seed?: number;
  duration_ms: number;
  receipt: {
    upstream_status: number;
    provider: ImageProvider;
    model: string;
    log_id?: string;
    cache_status?: string;
  };
}

export interface GenerateFailure {
  task_id: string;
  status: "failed";
  error: {
    code: ErrorCode;
    message: string;
    upstream_status?: number;
  };
}

export interface DryRunResponse {
  provider: ImageProvider;
  model: string;
  target: {
    url: string;
    method: string;
    body: unknown;
  };
  path_source: string;
}
