import type { Task } from '../api'

export interface GalleryGroup {
  label: string
  images: string[]
}

// 当前工作流链路仍会产出的 prompt 前缀 → 中文标签
const CURRENT_PREFIX_LABELS: Record<string, string> = {
  'sketch-to-design': '线稿成款',
  'partial-edit': '局部改款',
  'style-innovate': '款式创新',
  'style-fusion': '双图融合',
  'series-color': '系列配色',
  'directed-recolor': '定向换色',
  'fabric-on-body': 'Wear 上身',
  'multi-angle': '模特视角',
  'virtual-3d': '商品展示',
  'face-swap': '定向换脸',
  'bg-swap': '换背景',
  'remove-bg': '抠图',
  'pattern-gen': '图案生成',
  'to-vector': '转矢量图',
  'hd-upscale': '高清放大',
  'canvas-extend': '画布延伸',
}

function normalizeGroupKey(prefix: string | null): string {
  // 阶段 3 清理：legacy 前缀不再做别名映射
  return prefix || '__text2img__'
}

/**
 * 从 prompt 提取前缀 key，如 "[sketch-to-design] xxx" → "sketch-to-design"
 * 无前缀返回 null
 */
function extractPrefix(prompt: string): string | null {
  const m = prompt.match(/^\[([\w\u4e00-\u9fff -]+)\] /)
  return m ? m[1] : null
}

/**
 * 解析 task.result 取 image_url
 */
function getImageUrl(task: Task): string {
  try {
    return JSON.parse(task.result || '{}').image_url || ''
  } catch {
    return ''
  }
}

/**
 * 将历史任务按功能分组，每组包含标签和图片 URL 数组。
 * 只包含 completed 且有图片的任务。
 */
export function buildGalleryGroups(tasks: Task[]): GalleryGroup[] {
  const map = new Map<string, string[]>()

  for (const t of tasks) {
    if (t.status !== 'completed' || !t.result) continue
    const url = getImageUrl(t)
    if (!url) continue

    const prefix = t.prompt ? extractPrefix(t.prompt) : null
    const key = normalizeGroupKey(prefix)

    if (!map.has(key)) map.set(key, [])
    map.get(key)!.push(url)
  }

  const groups: GalleryGroup[] = []

  // "文生图"放最前面
  if (map.has('__text2img__')) {
    groups.push({ label: '文生图', images: map.get('__text2img__')! })
    map.delete('__text2img__')
  }

  // 其余按已有顺序
  for (const [key, images] of map) {
    const label = CURRENT_PREFIX_LABELS[key] || key
    groups.push({ label, images })
  }

  return groups
}
