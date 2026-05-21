export const WEAR_TILE_MODE_DESCRIPTIONS = {
  seamless: '平铺重复',
  single: '单张贴合',
} as const

export type WearTileMode = keyof typeof WEAR_TILE_MODE_DESCRIPTIONS

export const WEAR_TILE_MODE_TEMPLATE_DESC = '贴合法（平铺重复 / 单张贴合）'

export function describeWearTileMode(mode: WearTileMode): string {
  return WEAR_TILE_MODE_DESCRIPTIONS[mode]
}
