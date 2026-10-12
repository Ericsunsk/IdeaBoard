// Canvas task request DTO (W2 stream 1, workspace v1 transport).
//
// Feature-owned view of the canvas submission/readiness contract. These
// types were previously re-exported from the legacy `shared/api/task`
// transport (`/api/user/tasks/*`, deleted backend-side); they now live with
// the canvas execution contract that is their only consumer.
//
// `media_asset_ids` carries opaque `asset_...` workspace media references
// for the v1 `inputs.media_asset_ids` union member. Legacy URL/numeric
// inputs (`image_urls`, `image_asset_ids`, `reference_image_asset_ids`,
// `mask_image_url`) stay on the DTO so builders keep compiling the full
// request, but the workspace adapter refuses to send them (v1 rejects raw
// media URLs and has no numeric-id mapping): unmappable inputs fail closed
// with `CanvasWorkspaceTaskUnmappableError` instead of a cryptic 422.
/** @externalContract Canvas task request parameters. */
export type CanvasTaskParams = {
  use_mask?: boolean
  ratio?: string
  custom_width?: number
  custom_height?: number
  original_image_width?: number
  original_image_height?: number
  output_count?: number
  output_resolution?: string
  variation?: { strength?: number }
  /**
   * Opaque `asset_...` inpaint mask reference minted by the workspace
   * upload-session flow (the only durable mask path; data-URL/storage-URL
   * masks fail closed in the workspace adapter).
   */
  mask_asset_id?: string
  [key: string]: unknown
}

export type CanvasTaskRequestData = {
  prompt: string
  service?: string
  client_action_id?: string
  priority?: number
  image_urls?: string[]
  image_asset_ids?: number[]
  reference_image_asset_ids?: number[]
  /** Opaque `asset_...` ids; the only media refs the v1 transport sends. */
  media_asset_ids?: string[]
  params?: CanvasTaskParams
}

/** @externalContract Canvas task readiness billing response. */
type SubmitTaskReadinessBilling = {
  sufficient: boolean
  unit_credits: number
  total_credits: number
  available_credits: number
  reserved_credits: number
  price_version_id: number
  price_version?: number
  quantity?: number
  [key: string]: unknown
}

/** @externalContract Canvas task readiness response. */
export type SubmitTaskReadinessData = {
  ready: boolean
  message?: string
  error_code?: string
  service?: string
  billing?: SubmitTaskReadinessBilling | null
}

export type SubmitTaskReadinessMatrixRequest = {
  requests: CanvasTaskRequestData[]
}
