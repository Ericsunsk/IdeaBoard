import { SERVICE, type Service } from '@/core/contracts/imageServices'

const ADMIN_SERVICE_GROUP = {
  image: 'image',
  prompt: 'prompt',
  fusion: 'fusion',
} as const

export type AdminServiceGroupKey = (typeof ADMIN_SERVICE_GROUP)[keyof typeof ADMIN_SERVICE_GROUP]

export interface AdminServiceGroupMeta {
  key: AdminServiceGroupKey
  title: string
  order: number
}

export const ADMIN_SERVICE_GROUPS: readonly AdminServiceGroupMeta[] = [
  { key: ADMIN_SERVICE_GROUP.image, title: 'Image', order: 10 },
  { key: ADMIN_SERVICE_GROUP.prompt, title: 'Prompt', order: 20 },
  { key: ADMIN_SERVICE_GROUP.fusion, title: 'Fusion', order: 30 },
] as const

const ADMIN_SERVICE_GROUP_BY_KEY = Object.fromEntries(
  ADMIN_SERVICE_GROUPS.map((group) => [group.key, group]),
) as Readonly<Record<AdminServiceGroupKey, AdminServiceGroupMeta>>

export interface AdminServiceMeta {
  label: string
  groupKey: AdminServiceGroupKey
}

const ADMIN_SERVICE_META: Readonly<Record<Service, AdminServiceMeta>> = {
  [SERVICE.render]: { label: '草图成图', groupKey: ADMIN_SERVICE_GROUP.image },
  [SERVICE.variations]: {
    label: '风格延展',
    groupKey: ADMIN_SERVICE_GROUP.image,
  },
  [SERVICE.inpaint]: { label: '局部重绘', groupKey: ADMIN_SERVICE_GROUP.image },
  [SERVICE.outpaint]: { label: '扩图', groupKey: ADMIN_SERVICE_GROUP.image },
  [SERVICE.upscale]: { label: 'Upscale', groupKey: ADMIN_SERVICE_GROUP.image },
  [SERVICE.cutout]: { label: '抠图', groupKey: ADMIN_SERVICE_GROUP.image },
  [SERVICE.generation]: {
    label: 'Prompt 生图',
    groupKey: ADMIN_SERVICE_GROUP.prompt,
  },
  [SERVICE.fusion]: { label: '多图融合', groupKey: ADMIN_SERVICE_GROUP.fusion },
}

export const ADMIN_SERVICE_ORDER = Object.keys(ADMIN_SERVICE_META) as readonly Service[]

export function getAdminServiceOrder(slug: string): number {
  const index = ADMIN_SERVICE_ORDER.findIndex((service) => service === slug)
  return index >= 0 ? index : Number.MAX_SAFE_INTEGER
}

export function getAdminServiceMeta(slug: string): AdminServiceMeta | null {
  return Object.hasOwn(ADMIN_SERVICE_META, slug) ? ADMIN_SERVICE_META[slug as Service] : null
}

export function getAdminServiceGroupMeta(groupKey: AdminServiceGroupKey): AdminServiceGroupMeta {
  return ADMIN_SERVICE_GROUP_BY_KEY[groupKey]
}
