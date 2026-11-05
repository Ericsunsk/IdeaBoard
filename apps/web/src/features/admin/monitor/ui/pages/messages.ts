// Colocated admin messages for the monitor page (zh-only, no `t()` wrapping).
//
// Copy Policy (`core/CONTEXT.md`): admin/ops surfaces keep their literals next
// to the page that owns them instead of locale bundles. Toast literals stay in
// `session/useAdminMonitorFeedback.ts` next to the feedback factory;
// toolbar/table/dialog/cell copy for `components/TaskActivityTable.vue` /
// `components/TaskControlToolbar.vue` / `components/TaskEventsCard.vue` /
// `AdminTaskResultDialog.vue` / `TaskResultCell.vue` lives here. Metric labels
// stay in `features/admin/shared/ui/styling/taskMetricTokens`; status/attempt labels stay in
// `core/status.ts` / `taskResultLabels.ts`; time-range presets stay in
// `core/timeRange.ts`; `t('admin.*')` keys in `TaskControlToolbar.vue` belong
// to the cross-surface locale bundle and stay as-is.

export const MONITOR_MESSAGES = {
  staleNotice: '任务监控连接不稳定，当前显示上次成功数据。',
  retryConnection: '重试连接',
  retry: '重试',
  // Task result dialog.
  detailAria: '任务详情控制台',
  detailDescription: '查看任务执行的完整快照与结果。',
  copyTaskIdAria: '复制任务 ID',
  detailLoading: '正在加载任务详情…',
  detailUnavailable: '任务详情不可用',
  detailFailedTitle: '执行失败',
  detailUnknownError: '处理过程中发生未知错误',
  detailStoppedTitle: '任务已停止',
  detailStoppedFallback: '任务已停止，未继续执行。',
  detailDurationLabel: '总耗时:',
  detailCreatedLabel: '创建于:',
  identityTitle: '身份与归属',
  userLabel: '用户',
  copyUserIdAria: '复制用户 Public ID',
  priorityLabel: '优先级',
  providerLabel: '供应商',
  modelLabel: '模型',
  createdAtLabel: '创建时间',
  completedAtLabel: '完成时间',
  promptTitle: '任务指令 (Prompt)',
  copyPrompt: '复制全文',
  promptEmpty: '无提示词',
  resultTitle: '生成结果',
  resultImageAlt: '任务生成结果图片',
  openResultImage: '在新窗口打开',
  resultDeleted: '图片已由用户删除。',
  resultUnsafe: '结果图片链接不安全，已拒绝预览。',
  resultEmpty: '没有可预览的图片结果。',
  attemptsTitle: '执行轨迹',
  attemptsCount: (count: number) => `${count} 次尝试`,
  attemptStageLabel: '阶段:',
  attemptFailureLabel: '失败分类:',
  attemptLayerLabel: '重试层:',
  attemptGatewayRetriesLabel: '网关重试:',
  attemptProviderLatencyLabel: 'Provider 耗时:',
  attemptTotalLatencyLabel: '总耗时:',
  attemptsEmpty: '暂无执行记录',
  routingTitle: '路由快照',
  routingMissing: '该任务未记录路由快照（快照落盘前的历史任务）。',
  routingServiceLabel: '服务',
  routingResolutionLabel: '请求清晰度',
  routingPoolVersionLabel: '池版本',
  routingSelectedLabel: '选中模型',
  routingFinalLabel: '最终模型',
  routingRetryCandidateLabel: '兜底候选',
  routingEligibleLabel: '候选池',
  billingTitle: '计费快照',
  billingFeatureGone: '功能已删除',
  billingFeatureLabel: '功能',
  billingUnitLabel: '单价',
  billingQuantityLabel: '调用数量',
  billingTotalLabel: '总消耗',
  billingSettlementLabel: '结算状态',
  billingQuotedLabel: '报价时间',
  billingSettledLabel: '结算时间',
  apiResultTitle: 'API 响应摘要',
  copyJson: '复制 JSON',
  apiResultEmpty: '暂无 API 响应结果',
  debugToggle: (expanded: boolean) => `原始调试详情 (脱敏) · ${expanded ? '隐藏' : '展开'}`,
  // Activity table.
  tableEmpty: '暂无事件日志',
  timestampColumn: '时间戳',
  serviceColumn: '服务',
  modelColumn: '模型',
  statusColumn: 'Status',
  resultColumn: '结果',
  messageColumn: '消息',
  creditsColumn: '消耗',
  durationColumn: '耗时',
  moreColumn: '详情',
  cancelAction: '取消',
  cancelTaskAria: '取消任务',
  moreDetailAria: '更多任务详情',
  totalRecords: (total: number) => `共 ${total} 条记录`,
  // Control toolbar.
  timeRangeAria: '选择时间范围',
  moreActionsAria: '更多操作',
  // Events card.
  eventsTitle: '事件',
  eventsInfoTitle: '查看事件时序概览',
  chartsToggle: (collapsed: boolean) => (collapsed ? '展开图表' : '折叠图表'),
  // Result cell.
  resultInspectAria: (taskId: string, index: number) => `查看任务 ${taskId} 结果 ${index} 详情`,
  resultImageAltIndex: (taskId: string, index: number) => `任务 ${taskId} 结果 ${index}`,
} as const
