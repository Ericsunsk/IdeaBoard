// Colocated admin messages for the prompt-config page (zh-only, no `t()` wrapping).
//
// Copy Policy (`core/CONTEXT.md`): admin/ops surfaces keep their literals next
// to the page that owns them instead of locale bundles. Toast literals stay in
// `usePromptConfigFeedback.ts` next to the feedback factory; filter-option and
// workspace copy stays in `filtering.ts`/`workspace.ts`.

export const PROMPTS_MESSAGES = {
  searchLabel: '搜索提示词',
  searchPlaceholder: '搜索功能模块、节点标识或提示词内容...',
  filterLabel: '筛选提示词状态',
  refresh: '刷新提示词',
  loadFailedTitle: '系统提示词加载失败',
  loadFailedHint: '未能从服务端加载配置，请重试。',
  reload: '重新加载',
  emptyNoConfig: '暂无系统提示词配置',
  emptyNoMatch: '未找到匹配的提示词模块',
  emptyNoConfigHint: '当前没有可用的工作流节点系统提示词配置。',
  emptyNoMatchHint: '未找到符合当前搜索或筛选条件的功能节点。请尝试调整关键词或清空筛选。',
  clearFilters: '清空筛选条件',
  groupAria: (title: string) => `${title}提示词配置`,
  groupEmpty: '该模块暂无匹配项',
  variablesColumn: '可用变量',
  previewColumn: '系统提示词预览',
  actionsColumn: '操作',
  configured: '已配置',
  unconfigured: '未配置',
  noVariables: '无变量',
  unset: '未设置',
  configureAria: (title: string) => `配置 ${title} 提示词`,
  adjust: '调整',
  setup: '设置',
  editorHeader: (name: string) => `编辑 · ${name}`,
  editorDescription: '编辑该功能的系统提示词模板后保存。',
  editorSystemLabel: '系统提示词',
  editorInsertVariables: '插入变量:',
  editorTemplateAria: (title: string) => `${title} 提示词模板`,
  editorPlaceholder: '输入系统提示词模板，可点击上方变量按钮插入...',
  editorCharCount: (count: number) => `${count} 字符`,
  cancel: '取消',
  save: '保存',
} as const
