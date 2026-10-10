export type RetouchMode = 'inpaint' | 'adjust'

export type ImageRetouchKind = 'hsl' | 'threshold' | 'curves' | 'levels' | 'sharpen'

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

export type RetouchHslParamKey = keyof RetouchHslParams

/*
 * Full-kind single-key vocabulary. The intersection map unions the legal
 * key sets so one generic API covers all
 * kinds with per-key value correlation (`threshold` overlaps threshold /
 * sharpen with the same number type, so the intersection stays exact). A
 * wrong value type for a key is a compile error; a key that does not belong
 * to the target layer stays a runtime no-op (same reference).
 */
export type RetouchEffectParamMap = RetouchHslParams &
  RetouchThresholdParams &
  RetouchCurvesParams &
  RetouchLevelsParams &
  RetouchSharpenParams

export type RetouchEffectParamKey = keyof RetouchEffectParamMap

export type RetouchNonHslWholeParams =
  RetouchThresholdParams | RetouchCurvesParams | RetouchLevelsParams | RetouchSharpenParams

/** Label keys and literal parts remain independent of the active UI locale. */
export type RetouchLayerNamePart = string | { readonly labelKey: string }

type RetouchSourceLayer = {
  id: string
  type: 'source'
  name: string
  visible: true
  content?: RetouchRasterContent
  nameParts?: readonly RetouchLayerNamePart[]
}

type RetouchEmptyPaintContent = {
  kind: 'empty'
}

export type RetouchRasterContent = {
  kind: 'raster'
  dataUrl: string
  width: number
  height: number
}

type RetouchPaintContent = RetouchEmptyPaintContent | RetouchRasterContent

export type RetouchPaintLayer = {
  id: string
  type: 'paint'
  name: string
  visible: boolean
  content: RetouchPaintContent
  nameParts?: readonly RetouchLayerNamePart[]
}

type RetouchHslLayer = {
  id: string
  type: 'effect'
  kind: 'hsl'
  name: string
  visible: boolean
  params: RetouchHslParams
  nameParts?: readonly RetouchLayerNamePart[]
}

type RetouchThresholdLayer = {
  id: string
  type: 'effect'
  kind: 'threshold'
  name: string
  visible: boolean
  params: RetouchThresholdParams
  nameParts?: readonly RetouchLayerNamePart[]
}

type RetouchCurvesLayer = {
  id: string
  type: 'effect'
  kind: 'curves'
  name: string
  visible: boolean
  params: RetouchCurvesParams
  nameParts?: readonly RetouchLayerNamePart[]
}

type RetouchLevelsLayer = {
  id: string
  type: 'effect'
  kind: 'levels'
  name: string
  visible: boolean
  params: RetouchLevelsParams
  nameParts?: readonly RetouchLayerNamePart[]
}

type RetouchSharpenLayer = {
  id: string
  type: 'effect'
  kind: 'sharpen'
  name: string
  visible: boolean
  params: RetouchSharpenParams
  nameParts?: readonly RetouchLayerNamePart[]
}

export type RetouchEffectLayer =
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
  nameParts?: readonly RetouchLayerNamePart[]
}

export type RetouchEditableLayer = RetouchEffectLayer | RetouchPaintLayer | RetouchMergedLayer

export type RetouchLayer = RetouchSourceLayer | RetouchEditableLayer

export type RetouchLayerByKind<TKind extends ImageRetouchKind> = Extract<
  RetouchEffectLayer,
  { kind: TKind }
>
