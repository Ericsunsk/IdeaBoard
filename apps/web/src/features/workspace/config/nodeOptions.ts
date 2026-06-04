import type { ViewpointNodeItem } from '../types/canvasNodes'

type OptionItem<T extends string | number> = { label: string; value: T }
type ViewpointCategory = ViewpointNodeItem['activeCategory']
type ViewpointOptionItem = { key: string; label: string }

export const GENERATE_RATIO_OPTIONS = ['1:1', '3:4', '4:3', '2:3', '3:2'] as const
export const GENERATE_COUNT_OPTIONS = [1, 2, 4] as const

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
  ],
  bottom: [
    { key: 'btm_front', label: '正面立体' },
    { key: 'btm_side', label: '微侧立体' },
    { key: 'btm_half_side', label: '半侧动态' },
    { key: 'btm_back', label: '背面立体' },
  ],
  set: [
    { key: 'set_front', label: '正面展示' },
    { key: 'set_45_side', label: '45° 微侧' },
    { key: 'set_side_stand', label: '侧面静立' },
    { key: 'set_back', label: '背面展示' },
  ],
}

const VIEWPOINT_LABEL_MAP = Object.values(VIEWPOINT_OPTIONS_BY_CATEGORY).reduce<
  Record<string, string>
>((acc, options) => {
  options.forEach((option) => {
    acc[option.key] = option.label
  })
  return acc
}, {})

export function describeViewpointOption(key: string): string {
  return VIEWPOINT_LABEL_MAP[key] ?? ''
}

export function normalizeViewpointSelectionByCategory(
  category: ViewpointCategory,
  selectedViewpoint: string,
): string {
  const options = VIEWPOINT_OPTIONS_BY_CATEGORY[category] ?? []
  return options.some((option) => option.key === selectedViewpoint) ? selectedViewpoint : ''
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
