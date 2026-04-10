import type { Task } from '../api'

export interface GalleryGroup {
  label: string
  images: string[]
}

// prompt 前缀 → 中文标签
const PREFIX_LABELS: Record<string, string> = {
  '线稿': '线稿生成',
  'sketch-to-design': '线稿成款',
  'partial-edit': '局部改款',
  'style-innovate': '款式创新',
  'style-fusion': '风格融合',
  'series-color': '系列配色',
  'directed-recolor': '定向换色',
  'fabric-on-body': '面料上身',
  'pattern-on-body': '图案上身',
  'multi-angle': '模特多角度',
  'virtual-3d': '虚拟 3D',
  'mannequin-to-model': '人台变模特',
  'face-swap': '定向换脸',
  'bg-swap': '换背景',
  'garment-on-model': '款式上身',
  'pattern-gen': '图案生成',
  'style-clone': '画风复刻',
  'seamless-tile': '四方连续',
  'pattern-fusion': '图案融合',
  'craft-fusion': '工艺融合',
  'pattern-inpaint': '图案涂改',
  'pattern-upscale': '图案变清晰',
  'to-vector': '转矢量图',
  'remove-bg': '抠图褪底',
  'hd-upscale': '高清放大',
  'deblur': '模糊变清晰',
  'canvas-extend': '画布延伸',
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
    const key = prefix || '__text2img__'

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
    const label = PREFIX_LABELS[key] || key
    groups.push({ label, images })
  }

  return groups
}
