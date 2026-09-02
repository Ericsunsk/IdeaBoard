export type ResolvedTaskImageResult = {
  url: string
  mediaRef?: import('../../../../shared/media/MediaRef').MediaRef
  ratio: number
  width?: number
  height?: number
  mimeType?: string
  resultIndex?: number
}
