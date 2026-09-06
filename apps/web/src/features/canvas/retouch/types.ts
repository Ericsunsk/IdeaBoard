export type RetouchMode = 'inpaint' | 'adjust'

export type ImageAdjustmentKind = 'hsl' | 'threshold' | 'curves' | 'levels' | 'sharpen'

export type RetouchParamValue = number

export type RetouchHslParams = {
  hue: number
  saturation: number
  lightness: number
}

export type RetouchThresholdParams = {
  threshold: number
  invert: boolean
}

export type RetouchCurveChannel = 'rgb' | 'red' | 'green' | 'blue'

export type RetouchCurvePoint = {
  id: string
  input: number
  output: number
}

export type RetouchCurvesParams = {
  channels: Partial<Record<RetouchCurveChannel, RetouchCurvePoint[]>>
}

export type RetouchLevelsParams = {
  shadows: number
  midtones: number
  highlights: number
  outputBlack: number
  outputWhite: number
}

export type RetouchSharpenParams = {
  amount: number
  radius: number
  threshold: number
  protectColor: boolean
}

export type RetouchParamKey = keyof RetouchHslParams

export type RetouchAdjustmentWholeParams =
  RetouchThresholdParams | RetouchCurvesParams | RetouchLevelsParams | RetouchSharpenParams

export type RetouchSourceLayer = {
  id: string
  type: 'source'
  name: string
  visible: true
  content?: RetouchRasterContent
}

export type RetouchEmptyPaintContent = {
  kind: 'empty'
}

export type RetouchRasterContent = {
  kind: 'raster'
  dataUrl: string
  width: number
  height: number
}

export type RetouchPaintContent = RetouchEmptyPaintContent | RetouchRasterContent

export type RetouchPaintLayer = {
  id: string
  type: 'paint'
  name: string
  visible: boolean
  content: RetouchPaintContent
}

export type RetouchHslLayer = {
  id: string
  type: 'adjustment'
  kind: 'hsl'
  name: string
  visible: boolean
  params: RetouchHslParams
}

export type RetouchThresholdLayer = {
  id: string
  type: 'adjustment'
  kind: 'threshold'
  name: string
  visible: boolean
  params: RetouchThresholdParams
}

export type RetouchCurvesLayer = {
  id: string
  type: 'adjustment'
  kind: 'curves'
  name: string
  visible: boolean
  params: RetouchCurvesParams
}

export type RetouchLevelsLayer = {
  id: string
  type: 'adjustment'
  kind: 'levels'
  name: string
  visible: boolean
  params: RetouchLevelsParams
}

export type RetouchSharpenLayer = {
  id: string
  type: 'adjustment'
  kind: 'sharpen'
  name: string
  visible: boolean
  params: RetouchSharpenParams
}

export type RetouchAdjustmentLayer =
  | RetouchHslLayer
  | RetouchThresholdLayer
  | RetouchCurvesLayer
  | RetouchLevelsLayer
  | RetouchSharpenLayer

export type RetouchMergedLayer = {
  id: string
  type: 'merged'
  name: string
  visible: boolean
  children: RetouchEditableLayer[]
}

export type RetouchEditableLayer = RetouchAdjustmentLayer | RetouchPaintLayer | RetouchMergedLayer

export type RetouchLayer = RetouchSourceLayer | RetouchEditableLayer

export type RetouchLayerByKind<TKind extends ImageAdjustmentKind> = Extract<
  RetouchAdjustmentLayer,
  { kind: TKind }
>

export type RetouchRenderedPreview = {
  type: 'rendered-image'
  imageData: ImageData
  width: number
  height: number
}
