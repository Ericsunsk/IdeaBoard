import type {
  EnhanceNodeItem,
  FusionNodeItem,
  GenerateNodeItem,
  PaletteNodeItem,
  TryOnNodeItem,
} from '../../../types/canvasNodes'

type OptionItem<T extends string | number> = { label: string; value: T }
type ModelOptionItem = { key: string; label: string }

export const GENERATE_MODE_OPTIONS: OptionItem<GenerateNodeItem['mode']>[] = [
  { label: 'Pattern', value: 'pattern-gen' },
  { label: 'Clone', value: 'style-clone' },
  { label: 'Tile C', value: 'seamless-tile-classic' },
  { label: 'Tile K', value: 'seamless-tile-cartoon' },
]

export const GENERATE_RATIO_OPTIONS: GenerateNodeItem['ratio'][] = ['1:1', '3:4', '4:3', '2:3', '3:2']
export const GENERATE_COUNT_OPTIONS: GenerateNodeItem['count'][] = [1, 2, 4]
export const GENERATE_QUALITY_OPTIONS: GenerateNodeItem['quality'][] = ['1k', '2k', '4k']

export const ENHANCE_MODE_OPTIONS: OptionItem<EnhanceNodeItem['mode']>[] = [
  { label: 'Pattern', value: 'pattern-upscale' },
  { label: 'HD', value: 'hd-upscale' },
  { label: 'Deblur', value: 'deblur' },
  { label: 'Extend', value: 'canvas-extend' },
]

export const ENHANCE_UPSCALE_MODE_OPTIONS: OptionItem<EnhanceNodeItem['upscaleMode']>[] = [
  { label: 'Base', value: 'base' },
  { label: 'Enhance', value: 'enhancement' },
]

export const ENHANCE_UPSCALE_FACTOR_OPTIONS: EnhanceNodeItem['upscaleFactor'][] = [2, 3, 4]

export const ENHANCE_OUTPUT_FORMAT_OPTIONS: OptionItem<EnhanceNodeItem['outputFormat']>[] = [
  { label: 'JPG', value: 'jpg' },
  { label: 'PNG', value: 'png' },
]

export const TRYON_MODE_OPTIONS: OptionItem<TryOnNodeItem['mode']>[] = [
  { label: 'Fabric', value: 'fabric-on-body' },
  { label: 'Pattern', value: 'pattern-on-body' },
]

export const TRYON_TILE_MODE_OPTIONS: OptionItem<TryOnNodeItem['tileMode']>[] = [
  { label: 'Seamless', value: 'seamless' },
  { label: 'Single', value: 'single' },
]

export const PALETTE_MODE_OPTIONS: OptionItem<PaletteNodeItem['mode']>[] = [
  { label: 'Women', value: 'women' },
  { label: 'Men', value: 'men' },
  { label: 'Kids', value: 'kids' },
  { label: 'Ref', value: 'reference' },
]

export const PALETTE_PRESET_MAP = {
  women: [
    ['#8B3A3A', '#6B2D3E'],
    ['#B08050', '#C85040', '#A03030'],
    ['#D08050', '#E0B898', '#3D4560'],
    ['#3040A0', '#CC3030', '#B09878', '#D8DCE0'],
  ],
  men: [
    ['#6088A8', '#A08088', '#B83858'],
    ['#CC7840', '#384880', '#886840'],
    ['#B84858', '#707070', '#A8BCC8', '#586068'],
    ['#908090', '#B87878', '#D8C898'],
  ],
  kids: [
    ['#A08878', '#6080A8', '#C86040', '#B84050'],
    ['#987058', '#80B0A0', '#C87840', '#D8B8D0'],
    ['#886888', '#489060', '#C8C090', '#788898'],
    ['#D8A8A0', '#E8E0C8', '#98A868', '#8890A0'],
  ],
} satisfies Record<Exclude<PaletteNodeItem['mode'], 'reference'>, string[][]>

export const FUSION_MODE_FAMILY_OPTIONS: OptionItem<'style' | 'pattern' | 'craft'>[] = [
  { label: 'Style', value: 'style' },
  { label: 'Pattern', value: 'pattern' },
  { label: 'Craft', value: 'craft' },
]

export const FUSION_MODE_VARIANT_OPTIONS: OptionItem<'creative' | 'style'>[] = [
  { label: 'Creative', value: 'creative' },
  { label: 'Style', value: 'style' },
]

export const MODEL_BACKGROUND_OPTIONS: ModelOptionItem[] = [
  { key: 'original', label: '原图背景' },
  { key: 'studio', label: '影棚白底' },
]

export const MODEL_ANGLE_OPTIONS: ModelOptionItem[] = [
  { key: 'front_full', label: '正面全身' },
  { key: 'front_left_45', label: '前左 45°' },
  { key: 'front_right_45', label: '前右 45°' },
  { key: 'left_90', label: '左侧 90°' },
  { key: 'right_90', label: '右侧 90°' },
  { key: 'back_left_45', label: '后左 45°' },
  { key: 'back_right_45', label: '后右 45°' },
  { key: 'back_full', label: '背面全身' },
  { key: 'front_half', label: '正面半身' },
  { key: 'side_half', label: '侧面半身' },
  { key: 'portrait', label: '肖像特写' },
  { key: 'side_portrait', label: '侧颜特写' },
]

export function resolveFusionFamilyMode(
  family: 'style' | 'pattern' | 'craft',
  variant: 'creative' | 'style',
): FusionNodeItem['mode'] {
  if (family === 'style') return 'style-fusion'
  if (family === 'pattern') return variant === 'creative' ? 'pattern-fusion-creative' : 'pattern-fusion-style'
  return variant === 'creative' ? 'craft-fusion-creative' : 'craft-fusion-style'
}
