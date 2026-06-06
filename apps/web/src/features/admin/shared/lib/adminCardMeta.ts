type AdminCardMeta = {
  title: string
}

export const adminCards = {
  usageStatistics: {
    trend: {
      title: '调用量趋势',
    },
    modelRisk: {
      title: '模型风险矩阵',
    },
    qualityTrend: {
      title: '服务质量趋势',
    },
    slowModels: {
      title: '模型尾延迟',
    },
    bottleneckSource: {
      title: '瓶颈来源',
    },
    failureCategories: {
      title: '失败类型分布',
    },
  },
  taskMonitor: {
    executionPressure: {
      title: '执行压力',
    },
    taskDistribution: {
      title: '任务分布',
    },
    accountConcurrency: {
      title: '实时并发',
    },
  },
} as const satisfies Record<string, Record<string, AdminCardMeta>>
