import type {
  EnhanceNodeItem,
  GenerateNodeItem,
  TryOnNodeItem,
  ViewpointNodeItem,
} from '../../../types/canvasNodes'

type OptionItem<T extends string | number> = { label: string; value: T }
type ModelOptionItem = { key: string; label: string }
type ViewpointCategory = ViewpointNodeItem['activeCategory']
type ViewpointOptionItem = { key: string; label: string }

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
  { label: 'HD', value: 'hd-upscale' },
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

export const VIEWPOINT_CATEGORY_OPTIONS: OptionItem<ViewpointCategory>[] = [
  { label: 'Top', value: 'top' },
  { label: 'Bottom', value: 'bottom' },
  { label: 'Set', value: 'set' },
]

export const VIEWPOINT_OPTIONS_BY_CATEGORY: Record<ViewpointCategory, ViewpointOptionItem[]> = {
  top: [
    { key: 'top_front', label: '正面平视' },
    { key: 'top_side', label: '微侧展示' },
    { key: 'top_45', label: '45° 侧视' },
    { key: 'top_back', label: '背面展示' },
    { key: 'top_side_open', label: '微侧敞开' },
    { key: 'top_side_pocket', label: '微侧插兜' },
    { key: 'top_front_pocket', label: '正面插兜' },
    { key: 'top_collar', label: '领口挺括' },
    { key: 'top_woman_slim', label: '女士修身' },
    { key: 'top_side_arm', label: '斜侧曲臂' },
    { key: 'top_front_arm', label: '正侧曲臂' },
    { key: 'top_coat_open', label: '大衣敞开' },
    { key: 'top_coat_side', label: '大衣侧兜' },
  ],
  bottom: [
    { key: 'btm_front', label: '正面立体' },
    { key: 'btm_side', label: '微侧立体' },
    { key: 'btm_half_side', label: '半侧动态' },
    { key: 'btm_back', label: '背面立体' },
    { key: 'btm_wrinkle_free', label: '裤子去皱' },
    { key: 'btm_skirt_flow', label: '裙摆微扬' },
    { key: 'btm_3d_fold', label: '立体微折' },
    { key: 'btm_flat_fold', label: '平铺微折' },
    { key: 'btm_half_walk', label: '半侧微步' },
    { key: 'btm_waist_fold', label: '腰头翻折' },
    { key: 'btm_drape', label: '垂坠平整' },
  ],
  set: [
    { key: 'set_front', label: '正面展示' },
    { key: 'set_45_side', label: '45° 微侧' },
    { key: 'set_side_stand', label: '侧面静立' },
    { key: 'set_back', label: '背面展示' },
    { key: 'set_slim', label: '修身立体' },
    { key: 'set_lively', label: '活泼姿态' },
    { key: 'set_skirt_flow', label: '裙身飘逸' },
    { key: 'set_natural_walk', label: '自然行走' },
  ],
}

export const VIEWPOINT_LIGHTING_OPTIONS: OptionItem<ViewpointNodeItem['lighting']>[] = [
  { label: '暖白', value: 'warm_white' },
  { label: '午后', value: 'afternoon' },
  { label: '硬光', value: 'hard_light' },
]

export const VIEWPOINT_LIGHTING_LABELS: Record<ViewpointNodeItem['lighting'], string> = {
  warm_white: '高级暖白',
  afternoon: '午后暖阳',
  hard_light: '质感硬光',
}
