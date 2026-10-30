export type ImageExecutionInput = {
  url: string
  ratio: number
  /** Canonical workspace AssetID for submission and media delivery. */
  assetId?: string | undefined
}

export type ImageResultAnchor = {
  x: number
  y: number
  width: number
  outputAnchorNodeId?: string
}

export type ImageExecutionResultOptions = {
  anchor?: ImageResultAnchor
  targetRatio?: number
  targetWidth?: number
}
