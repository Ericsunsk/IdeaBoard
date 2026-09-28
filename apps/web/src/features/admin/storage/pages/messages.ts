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
  searchLabel: '搜索存储对象',
  searchPlaceholder: '搜索存储名称、供应商或存储桶...',
  filterLabel: '筛选存储状态',
  refresh: '刷新存储',
  create: '新增存储',
  storageCount: (count: number) => `共 ${count} 个存储服务`,
  totalCount: (count: number) => `(共 ${count} 个)`,
  allStatuses: '全部状态',
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
  tableEmptyTitle: (hasActiveFilter: boolean) =>
    hasActiveFilter ? '未找到匹配的存储配置' : '暂无存储配置',
  tableEmptyHint: (hasActiveFilter: boolean) =>
    hasActiveFilter
      ? '未找到符合当前搜索或筛选条件的存储。请尝试调整关键词或清空筛选。'
      : '当前没有已注册的对象存储配置。点击右上角新增存储开始配置。',
  tableClearFilters: '清空筛选条件',
  tableSectionAria: '对象存储',
  tableSectionCount: (count: number) => `共 ${count} 个配置`,
  tableAria: '对象存储配置列表',
  tableScrollAria: '对象存储配置列表横向滚动区域',
  tableEmptyMatch: '暂无匹配存储',
  serviceColumn: '存储服务',
  statusColumn: '状态',
  bucketColumn: '存储桶 / 路径',
  endpointColumn: '访问端点 / 域名',
  currentColumn: '当前生效',
  actionsColumn: '操作',
  currentEffective: '当前生效',
  configureAction: '配置',
  editAction: '编辑',
  testAction: '测试连接',
  deleteAction: '删除',
  // Editor dialog.
  editorCreateTitle: '添加对象存储',
  editorFallbackTitle: '存储配置',
  editorDescription: '填写对象存储配置后保存。',
  editorCreateSubtitle: '选择云存储厂商预设或自定义 S3 兼容接入',
  editorProviderSubtitle: (providerLabel: string) => `供应商：${providerLabel}`,
  editorCurrentBadge: '当前生效存储',
  editorTabsAria: '存储配置分页',
  editorTabBasic: '基础与凭据',
  editorTabEndpoint: '端点与网络',
  editorTabAdvanced: '高级选项',
  editorEnableLabel: '启用该存储配置',
  editorEnableState: (enabled: boolean) => (enabled ? '已启用' : '已关闭'),
  editorEnableAria: '存储启用状态',
  editorNameLabel: '存储配置名称',
  editorNamePlaceholder: '例如：生产环境主存储 / 阿里云 OSS',
  editorDetectDomains: '一键自动探测域名',
  editorDomainsFound: '探测到域名：',
  editorSaveApply: (isCurrent: boolean) => (isCurrent ? '保存' : '设为当前'),
} as const
