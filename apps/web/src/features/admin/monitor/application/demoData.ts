import { adminTaskStatuses } from '../domain/status'
import { getAdminWorkflowFeatureMeta } from '../../shared/lib/workflowFeatureAdminMeta'
import type { AdminMonitorDetailModel } from '../infrastructure/resource'
import type { AdminTaskBillingSnapshot } from '../../../../shared/api/generated-query/model/index'
import type { AdminTaskRow } from './useAdminTaskList'

export const DEMO_MODE_QUERY_VALUE = '1'

type DemoTemplate = {
  user: string
  feature: string
  model: string
  prompt: string
  failedPrompt?: string
}

// Plausible image-workflow prompts; failures reuse the matching failedPrompt.
const demoTemplates: readonly DemoTemplate[] = [
  {
    user: 'Eric Sun',
    feature: 'render',
    model: 'seedream-v2',
    prompt: 'render: 产品主图草图成图，白底 + 柔光阴影，输出 1 张',
    failedPrompt: 'render: 超时未返回网关回执，已释放预留额度',
  },
  {
    user: 'Alice W',
    feature: 'variations',
    model: 'gpt-image-1',
    prompt: 'variations: 主图风格延展 3 张，暖色电商风',
  },
  {
    user: 'Dev Bot',
    feature: 'upscale',
    model: 'topaz-upscale',
    prompt: 'upscale: 详情页长图 2x 放大',
    failedPrompt: 'upscale: 上游模型返回 500，已自动释放额度',
  },
  {
    user: 'System Cron',
    feature: 'inpaint',
    model: 'seedream-v2',
    prompt: 'inpaint: 局部重绘商品 logo 区域',
  },
  {
    user: 'Eric Sun',
    feature: 'outpaint',
    model: 'gpt-image-1',
    prompt: 'outpaint: 横版海报左右扩图',
    failedPrompt: 'outpaint: 参考图缺失，已释放预留额度',
  },
  {
    user: 'Alice W',
    feature: 'cutout',
    model: 'bria-cutout',
    prompt: 'cutout: 主图一键抠图去背景',
  },
  {
    user: 'Dev Bot',
    feature: 'fusion',
    model: 'seedream-v2',
    prompt: 'fusion: 模特图与产品图多图融合',
  },
  {
    // 超长 Message 演示截断：实际渲染 120 字 + 省略号，hover 看全文
    user: 'Eric Sun',
    feature: 'variations',
    model: 'gpt-image-1',
    prompt:
      'variations: 双十一大促主会场英雄区横版海报延展适配，要求保留主标题排版与价格标签位置，背景从深蓝渐变过渡到品红光斑，模特姿态不变，输出三张不同构图分别适配首屏轮播、信息流卡片与开屏广告位，同步输出各尺寸安全边距标注与字体描边参数，失败自动重试两次并回退默认模板',
  },
]

// Fixed seed so charts and rows stay stable across renders within one duration.
function createSeededRandom(seed: number) {
  let state = seed >>> 0
  return () => {
    state = (state * 1664525 + 1013904223) >>> 0
    return state / 0xffffffff
  }
}

const DEMO_RESULT_IMAGES = [
  '/uploads/site/assets/demo/demo-render.svg',
  '/uploads/site/assets/demo/demo-variations.svg',
  '/uploads/site/assets/demo/demo-upscale.svg',
  '/uploads/site/assets/demo/demo-fusion.svg',
] as const

const DEMO_STATUSES = [
  adminTaskStatuses.completed,
  adminTaskStatuses.completed,
  adminTaskStatuses.completed,
  adminTaskStatuses.running,
  adminTaskStatuses.running,
  adminTaskStatuses.pending,
  adminTaskStatuses.failed,
] as const

export function createDemoMonitorList(durationMs: number, count = 42): AdminTaskRow[] {
  const random = createSeededRandom(Math.floor(durationMs / 60000) + count)
  const now = Date.now()
  return Array.from({ length: count }, (_, index) => {
    // 峰值任务集中在区间中段（约 45% 处），让单桶已完成数突破 10（达到约 16~18 条），直观展示 Y 轴自适应扩展
    const isPeakBurst = index < 16
    const status = isPeakBurst
      ? adminTaskStatuses.completed
      : DEMO_STATUSES[index % DEMO_STATUSES.length]!

    const timeOffset = isPeakBurst
      ? durationMs * 0.45 + (random() - 0.5) * (durationMs * 0.08)
      : Math.floor(random() * durationMs)

    const template = demoTemplates[index % demoTemplates.length]!
    const failed = status === adminTaskStatuses.failed && template.failedPrompt
    const completed = status === adminTaskStatuses.completed
    const createdAt = new Date(now - timeOffset)
    const completedAt = completed
      ? new Date(createdAt.getTime() + 5000 + Math.floor(random() * 115000))
      : null
    // 已完成行挂 1-4 张结果图，演示 Result 列 3 张上限 + +N 溢出
    const resultCount = completed ? 1 + (index % 4) : 0
    const resultImages = DEMO_RESULT_IMAGES.slice(0, Math.min(resultCount, DEMO_RESULT_IMAGES.length))
    return {
      task_id: `demo-task-${String(index + 1).padStart(3, '0')}`,
      user_public_id: '00000000-0000-0000-0000-000000000000',
      user_display_name: template.user,
      prompt: failed ? template.failedPrompt! : template.prompt,
      feature_slug: template.feature,
      credits: completed ? 2 + Math.floor(random() * 3) : 0,
      result: resultImages.length > 0 ? JSON.stringify({ image_urls: resultImages }) : '',
      latest_execution: {
        provider: '',
        gateway_step: '',
        resolved_model: template.model,
      },
      priority: 1 + Math.floor(random() * 3),
      status,
      error_msg: failed ? template.failedPrompt! : '',
      created_at: createdAt.toISOString(),
      completed_at: completedAt ? completedAt.toISOString() : null,
    }
  }).sort((left, right) => right.created_at.localeCompare(left.created_at))
}

export type DemoMonitorStats = {
  pending: number
  running: number
  completed: number
  failed: number
  cancelled: number
}

export function createDemoMonitorStats(list: readonly AdminTaskRow[]): DemoMonitorStats {
  return {
    pending: list.filter((item) => item.status === adminTaskStatuses.pending).length,
    running: list.filter((item) => item.status === adminTaskStatuses.running).length,
    completed: list.filter((item) => item.status === adminTaskStatuses.completed).length,
    failed: list.filter((item) => item.status === adminTaskStatuses.failed).length,
    cancelled: list.filter((item) => item.status === adminTaskStatuses.cancelled).length,
  }
}

// 演示详情弹窗：用行内数据本地拼装，不触达真实详情接口。
export function createDemoTaskDetail(row: AdminTaskRow): AdminMonitorDetailModel {
  const meta = getAdminWorkflowFeatureMeta(row.feature_slug)
  const settled = row.status === adminTaskStatuses.completed
  const billing: AdminTaskBillingSnapshot = {
    sku: `${row.feature_slug || 'render'}:demo`,
    output_resolution: 'default',
    display_name: meta?.label ?? row.feature_slug,
    price_version_id: 1,
    price_version: 1,
    unit_credits: row.credits,
    quantity: 1,
    total_credits: row.credits,
    captured_credits: settled ? row.credits : 0,
    quoted_at: row.created_at,
    settlement_state: settled ? 'captured' : 'released',
    settled_at: row.completed_at,
    feature_exists: true,
  }
  return {
    task_id: row.task_id,
    correlation_id: `demo-${row.task_id}`,
    user_public_id: row.user_public_id,
    user_display_name: row.user_display_name,
    prompt: row.prompt,
    user_prompt: row.prompt,
    params: '{}',
    feature_slug: row.feature_slug,
    priority: row.priority,
    status: row.status,
    retry_count: 0,
    error_msg: row.error_msg,
    result: row.result,
    outputs: [],
    created_at: row.created_at,
    updated_at: row.created_at,
    completed_at: row.completed_at,
    cancelled_at: null,
    cancel_reason: '',
    billing,
    latest_execution: {
      provider: row.latest_execution?.provider ?? '',
      gateway_step: row.latest_execution?.gateway_step ?? '',
      resolved_model: row.latest_execution?.resolved_model ?? '',
    },
    execution_attempts: [],
  }
}
