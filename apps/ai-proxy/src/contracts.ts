// CanonicalImageRequest：唯一的入口合同，只实现最终合同。
// images 约定：presigned GET URL 数组，输入图在前、mask 在后。
//   airun/google（nano-banana 系）：images 全量 -> image_input（data URI，≤3 张）。
//   airun/openai（gpt-image 系）：images 全量 -> images（data URI）。
//   airun/krea + workers-ai 迁移：images 全量 -> image_urls（data URI，P0 待首单确认）。
//   gateway/fal: images 全量 -> image_urls（data URI），images[0] -> image_url，>=2 张时末张 -> mask_url
//   gateway/openai: 无图 -> images/generations（JSON）；有图 -> images/edits（multipart，P0 状态：待首单真生成确认）。
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
  INTERNAL_SECRET: string;
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
