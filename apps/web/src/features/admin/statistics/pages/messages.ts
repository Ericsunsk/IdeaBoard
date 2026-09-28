// Colocated admin messages for the statistics page (zh-only, no `t()` wrapping).
//
// Copy Policy (`shared/CONTEXT.md`): admin/ops surfaces keep their literals next
// to the page that owns them instead of locale bundles. Chart-footnote literals
// owned by `useUsageStatisticsCharts` stay in their chart modules.

export const STATISTICS_MESSAGES = {
  todayCalls: '今日调用',
  todayFailed: '今日失败',
  todaySuccessRate: '今日成功率',
  monthCalls: '本月调用',
  trendTitle: '调用量与消耗趋势',
  riskMatrixTitle: '模型风险矩阵',
  qualityTrendTitle: '服务质量趋势',
  bottleneckTitle: '瓶颈来源',
  loadFailed: '加载统计数据失败',
} as const
