// Colocated admin messages for the storage page (zh-only, no `t()` wrapping).
//
// Copy Policy (`core/CONTEXT.md`): admin/ops surfaces keep their literals next
// to the page that owns them instead of locale bundles. Toast literals stay in
// `useStorageFeedback.ts` next to the feedback factory; table/dialog copy for
// `components/AdminStorageTable.vue` / `components/AdminStorageEditorDialog.vue`
// lives here; preset keywords in `PRESET_MATCHERS` are matching constants,
// not user-visible copy, and stay inline. Preset catalog labels and workspace
// confirm copy stay in `meta.ts` / `workspace.ts` next to the logic that owns
// them.

export const STORAGE_MESSAGES = {
  storageTitle: '存储',
  storageSubtitle: '对象存储服务供应商与凭据管理',
  create: '新增存储',
  localEndpoint: '服务器本地',
  save: '保存',
  applyCurrent: '设为当前',
  secretKeptPlaceholder: '已设置，留空则保持不变',
  fieldPlaceholder: (fieldLabel: string) => `请输入${fieldLabel}`,
  switchLogoTitle: '点击切换存储 Logo',
  switchLogoAria: '切换存储 Logo',
  // Table.
  tableSectionAria: '对象存储',
  tableAria: '对象存储设置列表',
  tableScrollAria: '对象存储设置列表横向滚动区域',
  tableEmptyMatch: '暂无匹配存储',
  serviceColumn: 'Service',
  enableColumn: 'Enable',
  bucketColumn: '存储桶 / 路径',
  endpointColumn: '访问端点 / 域名',
  actionsColumn: '操作',
  configureAction: '设置',
  testAction: '测试连接',
  deleteAction: '删除',
  // Editor dialog.
  editorCreateTitle: '新增存储',
  editorFallbackTitle: '存储设置',
  editorDescription: '填写对象存储设置后保存。',
  editorProviderSubtitle: (providerLabel: string) => `供应商：${providerLabel}`,
  editorTabsAria: '存储设置分页',
  editorTabBasic: '基础与凭据',
  editorTabEndpoint: '端点与网络',
  editorTabAdvanced: '高级选项',
  editorNameLabel: '存储设置名称',
  editorNamePlaceholder: '例如：生产环境主存储 / 阿里云 OSS',
  editorDetectDomains: '一键自动探测域名',
  editorDomainsFound: '探测到域名：',
} as const
