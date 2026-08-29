export type ImageExecutionInput = {
  url: string
  ratio: number
  mediaAssetId?: number
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
