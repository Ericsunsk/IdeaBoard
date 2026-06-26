export type ImageEditorMode = 'inpaint' | 'adjust'

export type ImageEditorTool =
  | 'move'
  | 'selection'
  | 'brush'
  | 'eraser'
  | 'eyedropper'
  | 'crop'
  | 'zoom'

export type ImageAdjustmentKind = 'hsl' | 'threshold' | 'curves' | 'levels' | 'invert'

export type ImageEditorParamValue = number | boolean

export type ImageEditorHslParams = {
  hue: number
  saturation: number
  brightness: number
  contrast: number
}

export type ImageEditorThresholdParams = {
  threshold: number
  invert: boolean
}

export type ImageEditorCurvesParams = {
  input: number
  output: number
}

export type ImageEditorLevelsParams = {
  shadows: number
  midtones: number
  highlights: number
}

export type ImageEditorInvertParams = {
  enabled: boolean
}

export type ImageEditorParamKey =
  | keyof ImageEditorHslParams
  | keyof ImageEditorThresholdParams
  | keyof ImageEditorCurvesParams
  | keyof ImageEditorLevelsParams
  | keyof ImageEditorInvertParams

export type ImageEditorSourceLayer = {
  id: string
  type: 'source'
  name: string
  visible: true
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

export type ImageEditorInvertLayer = {
  id: string
  type: 'adjustment'
  kind: 'invert'
  name: string
  visible: boolean
  params: ImageEditorInvertParams
}

export type ImageEditorAdjustmentLayer =
  | ImageEditorHslLayer
  | ImageEditorThresholdLayer
  | ImageEditorCurvesLayer
  | ImageEditorLevelsLayer
  | ImageEditorInvertLayer

export type ImageEditorLayer = ImageEditorSourceLayer | ImageEditorAdjustmentLayer

export type ImageEditorLayerByKind<TKind extends ImageAdjustmentKind> = Extract<
  ImageEditorAdjustmentLayer,
  { kind: TKind }
>
