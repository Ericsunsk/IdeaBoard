export interface SegConfigForm {
  enabled: boolean
  access_key_id: string
  access_key_secret: string
  region_id: string
}

export interface SegConfigSavePayload {
  enabled: boolean
  access_key_id: string
  region_id: string
  access_key_secret?: string
}

export interface SegMetaItem {
  label: string
  value: string
}

export interface SegSaveBuildResult {
  payload: SegConfigSavePayload
  nextSecret: string
}

export const SEG_DEFAULT_REGION = 'cn-shanghai'

export const SEG_CONFIG_COPY = {
  pageTitle: '图片处理',
  pageDesc: '当前已接入阿里云分割抠图。通过卡片查看状态，并在弹窗内维护密钥与地域配置。',
  cardProvider: '阿里云',
  cardTitle: '阿里云分割抠图',
  cardSubtitle: '面向用户侧的阿里云分割抠图与图片处理能力。',
  cardFeatureLabel: '功能',
  cardFeatureValue: '阿里云分割抠图',
  cardRegionLabel: 'Region',
  cardAccessKeyLabel: 'AccessKey',
  cardSecretLabel: 'Secret',
  cardEditButton: '编辑配置',
  chipStatusOn: '开启',
  chipStatusOff: '关闭',
  chipService: '服务 阿里云视觉智能',
  chipType: '类型 阿里云分割抠图',
  editorEyebrow: '阿里云 · 图片处理',
  editorTitle: '编辑阿里云分割抠图',
  editorDesc: '保存后会立即更新当前系统使用中的图片处理配置，影响用户侧分割抠图功能可用性。',
  editorProvider: '阿里云分割抠图',
  labelEnabled: '启用状态',
  enabledOn: '已启用',
  enabledOff: '已关闭',
  labelRegionId: 'Region ID',
  regionPlaceholder: '默认 cn-shanghai',
  regionHint: '阿里云视觉智能接入地域，默认 `cn-shanghai`。',
  sectionSecret: '密钥配置',
  labelAccessKeyId: 'AccessKey ID',
  accessKeyPlaceholder: '请输入 AccessKey ID',
  accessKeyHintPrefix: '前往 ',
  accessKeyHintLink: 'AccessKey 管理',
  accessKeyHintSuffix: ' 获取。',
  labelAccessKeySecret: 'AccessKey Secret',
  secretPlaceholderConfigured: '已配置，保留当前值或输入新的 Secret',
  secretPlaceholderEmpty: '请输入 AccessKey Secret',
  secretHintPrefix: '请先在 ',
  secretHintLink: '阿里云',
  secretHintSuffix: ' 开通分割抠图服务。',
  editorNote: '当前只有这张阿里云分割抠图卡会接管用户侧的图片分割流程。',
  cancelButton: '取消',
  saveButton: '保存配置',
  saveSuccess: '保存成功',
  saveFailed: '保存失败',
  statusCurrent: '当前处理',
  statusConfigured: '已配置',
  statusEmpty: '未配置',
  emptyFallback: '未配置',
} as const

export function createDefaultSegForm(): SegConfigForm {
  return {
    enabled: false,
    access_key_id: '',
    access_key_secret: '',
    region_id: SEG_DEFAULT_REGION,
  }
}

export function isSegConfigured(card: SegConfigForm, secretConfigured: boolean): boolean {
  return Boolean(card.access_key_id && (card.access_key_secret || secretConfigured))
}

export function segStatusText(enabled: boolean, configured: boolean): string {
  if (enabled && configured) return SEG_CONFIG_COPY.statusCurrent
  if (configured) return SEG_CONFIG_COPY.statusConfigured
  return SEG_CONFIG_COPY.statusEmpty
}

export function segStatusClass(enabled: boolean, configured: boolean): 'is-current' | 'is-enabled' | 'is-empty' {
  if (enabled && configured) return 'is-current'
  if (configured) return 'is-enabled'
  return 'is-empty'
}

export function segMetaItems(card: SegConfigForm, secretConfigured: boolean): SegMetaItem[] {
  return [
    { label: SEG_CONFIG_COPY.cardFeatureLabel, value: SEG_CONFIG_COPY.cardFeatureValue },
    { label: SEG_CONFIG_COPY.cardRegionLabel, value: card.region_id || SEG_DEFAULT_REGION },
    { label: SEG_CONFIG_COPY.cardAccessKeyLabel, value: card.access_key_id || SEG_CONFIG_COPY.emptyFallback },
    { label: SEG_CONFIG_COPY.cardSecretLabel, value: secretConfigured ? '已配置' : '未配置' },
  ]
}

export function segFeatureChips(card: SegConfigForm): string[] {
  return [
    `状态 ${card.enabled ? SEG_CONFIG_COPY.chipStatusOn : SEG_CONFIG_COPY.chipStatusOff}`,
    SEG_CONFIG_COPY.chipService,
    SEG_CONFIG_COPY.chipType,
  ]
}

export function normalizeSegConfig(rawConfig: unknown): { card: SegConfigForm; secretConfigured: boolean } {
  const cfg = (rawConfig && typeof rawConfig === 'object' ? rawConfig : {}) as Partial<SegConfigForm>
  const hasMaskedSecret = typeof cfg.access_key_secret === 'string' && cfg.access_key_secret.includes('*')
  return {
    secretConfigured: hasMaskedSecret || Boolean(cfg.access_key_secret),
    card: {
      enabled: Boolean(cfg.enabled),
      access_key_id: cfg.access_key_id || '',
      access_key_secret: '',
      region_id: cfg.region_id || SEG_DEFAULT_REGION,
    },
  }
}

export function buildSegSavePayload(form: SegConfigForm): SegSaveBuildResult {
  const nextSecret = form.access_key_secret.trim()
  const payload: SegConfigSavePayload = {
    enabled: form.enabled,
    access_key_id: form.access_key_id,
    region_id: form.region_id,
  }
  if (nextSecret) {
    payload.access_key_secret = nextSecret
  }
  return { payload, nextSecret }
}
