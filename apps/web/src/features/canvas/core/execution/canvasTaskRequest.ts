// Feature-owned Canvas submission/readiness DTO. Media inputs use opaque
// workspace AssetIDs through media_asset_ids and params.mask_asset_id.
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
  /** Opaque `asset_...` ids; the only media refs the v1 transport sends. */
  media_asset_ids?: string[]
  /**
   * Opaque `board_...` id binding this submit to its Canvas Board (#937).
   * The server validates Board identity + generate capability and
   * snapshots the access version; omitted ids stay non-Canvas tasks
   * (submitter-only, never auto-published). Set by the execution
   * transport from the Board attachment, never forged by callers.
   */
  board_id?: string
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
