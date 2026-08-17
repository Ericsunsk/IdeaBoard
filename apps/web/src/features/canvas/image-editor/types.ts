export type ImageEditorMode = 'inpaint' | 'adjust'

export type ImageAdjustmentKind = 'hsl' | 'threshold' | 'curves' | 'levels' | 'sharpen'

export type ImageEditorParamValue = number

export type ImageEditorHslParams = {
  hue: number
  saturation: number
  lightness: number
}

export type ImageEditorThresholdParams = {
  threshold: number
  invert: boolean
}

export type ImageEditorCurveChannel = 'rgb' | 'red' | 'green' | 'blue'

export type ImageEditorCurvePoint = {
  id: string
  input: number
  output: number
}

export type ImageEditorCurvesParams = {
  channels: Partial<Record<ImageEditorCurveChannel, ImageEditorCurvePoint[]>>
}

export type ImageEditorLevelsParams = {
  shadows: number
  midtones: number
  highlights: number
  outputBlack: number
  outputWhite: number
}

export type ImageEditorSharpenParams = {
  amount: number
  radius: number
  threshold: number
  protectColor: boolean
}

export type ImageEditorParamKey = keyof ImageEditorHslParams

export type ImageEditorAdjustmentWholeParams =
  | ImageEditorThresholdParams
  | ImageEditorCurvesParams
  | ImageEditorLevelsParams
  | ImageEditorSharpenParams

export type ImageEditorSourceLayer = {
  id: string
  type: 'source'
  name: string
  visible: true
  content?: ImageEditorRasterContent
}

export type ImageEditorEmptyPaintContent = {
  kind: 'empty'
}

export type ImageEditorRasterContent = {
  kind: 'raster'
  dataUrl: string
  width: number
  height: number
}

export type ImageEditorPaintContent = ImageEditorEmptyPaintContent | ImageEditorRasterContent

export type ImageEditorPaintLayer = {
  id: string
  type: 'paint'
  name: string
  visible: boolean
  content: ImageEditorPaintContent
}

export type ImageEditorHslLayer = {
  id: string
  type: 'adjustment'
  kind: 'hsl'
  name: string
  visible: boolean
  params: ImageEditorHslParams
}

export type ImageEditorThresholdLayer = {
  id: string
  type: 'adjustment'
  kind: 'threshold'
  name: string
  visible: boolean
  params: ImageEditorThresholdParams
}

export type ImageEditorCurvesLayer = {
  id: string
  type: 'adjustment'
  kind: 'curves'
  name: string
  visible: boolean
  params: ImageEditorCurvesParams
}

export type ImageEditorLevelsLayer = {
  id: string
  type: 'adjustment'
  kind: 'levels'
  name: string
  visible: boolean
  params: ImageEditorLevelsParams
}

export type ImageEditorSharpenLayer = {
  id: string
  type: 'adjustment'
  kind: 'sharpen'
  name: string
  visible: boolean
  params: ImageEditorSharpenParams
}

export type ImageEditorAdjustmentLayer =
  | ImageEditorHslLayer
  | ImageEditorThresholdLayer
  | ImageEditorCurvesLayer
  | ImageEditorLevelsLayer
  | ImageEditorSharpenLayer

export type ImageEditorMergedLayer = {
  id: string
  type: 'merged'
  name: string
  visible: boolean
  children: ImageEditorEditableLayer[]
}

export type ImageEditorEditableLayer =
  ImageEditorAdjustmentLayer | ImageEditorPaintLayer | ImageEditorMergedLayer

export type ImageEditorLayer = ImageEditorSourceLayer | ImageEditorEditableLayer

export type ImageEditorLayerByKind<TKind extends ImageAdjustmentKind> = Extract<
  ImageEditorAdjustmentLayer,
  { kind: TKind }
>

export type ImageEditorRenderedPreview = {
  type: 'rendered-image'
  imageData: ImageData
  width: number
  height: number
}
