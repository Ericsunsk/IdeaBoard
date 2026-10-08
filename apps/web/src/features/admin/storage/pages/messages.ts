// Colocated admin messages for the storage page (zh-only, no `t()` wrapping).
//
// Copy Policy (`shared/CONTEXT.md`): admin/ops surfaces keep their literals next
// to the page that owns them instead of locale bundles. Toast literals stay in
// `useStorageFeedback.ts` next to the feedback factory; table/dialog copy for
// `components/AdminStorageTable.vue` / `components/AdminStorageEditorDialog.vue`
// lives here; preset keywords in `PRESET_MATCHERS` are matching constants,
// not user-visible copy, and stay inline. Preset catalog labels, profile status
// text, and workspace confirm copy stay in `meta.ts` / `profiles.ts` /
// `workspace.ts` next to the logic that owns them.

export const STORAGE_MESSAGES = {
  storageTitle: '存储配置',
  storageSubtitle: '对象存储服务供应商与凭据管理',
  create: '新增存储',
  current: '使用中',
  configured: '已配置',
  unconfigured: '未配置',
  localEndpoint: '服务器本地',
  cancel: '取消',
  save: '保存',
  applyCurrent: '设为当前',
  secretKeptPlaceholder: '已配置，留空则保持不变',
  fieldPlaceholder: (fieldLabel: string) => `请输入${fieldLabel}`,
  switchLogoTitle: '点击切换存储 Logo',
  switchLogoAria: '切换存储 Logo',
  // Table.
  tableLoadingAria: '加载存储配置中',
  tableLoadFailed: '加载存储配置失败',
  retry: '重试',
  tableEmptyTitle: '暂无存储配置',
  tableEmptyHint: '当前没有已注册的对象存储配置。点击右上角新增存储开始配置。',
  tableSectionAria: '对象存储',
  tableAria: '对象存储配置列表',
  tableScrollAria: '对象存储配置列表横向滚动区域',
  tableEmptyMatch: '暂无匹配存储',
  serviceColumn: '存储服务',
  enableColumn: '启用',
  bucketColumn: '存储桶 / 路径',
  endpointColumn: '访问端点 / 域名',
  actionsColumn: '操作',
  configureAction: '设置',
  testAction: '测试连接',
  deleteAction: '删除',
  // Editor dialog.
  editorCreateTitle: '新增存储',
  editorFallbackTitle: '存储配置',
  editorDescription: '填写对象存储配置后保存。',
  editorCreateSubtitle: '选择云存储厂商预设或自定义 S3 兼容接入',
  editorProviderSubtitle: (providerLabel: string) => `供应商：${providerLabel}`,
  editorCurrentBadge: '当前生效存储',
  editorTabsAria: '存储配置分页',
  editorTabBasic: '基础与凭据',
  editorTabEndpoint: '端点与网络',
  editorTabAdvanced: '高级选项',
  editorNameLabel: '存储配置名称',
  editorNamePlaceholder: '例如：生产环境主存储 / 阿里云 OSS',
  editorDetectDomains: '一键自动探测域名',
  editorDomainsFound: '探测到域名：',
} as const
