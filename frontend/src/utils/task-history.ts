import type { Task } from '../api'
import { DEFAULT_IMAGE_DOWNLOAD_NAME, USER_DOWNLOAD_ENDPOINT } from '../constants/media'

export function stripFeaturePrefix(str?: string): string {
  if (!str) return ''
  if (str.startsWith('[')) {
    const idx = str.indexOf('] ')
    if (idx > 0) return str.slice(idx + 2)
  }
  return str
}

export function getTaskImageUrl(task: Task): string {
  try {
    return JSON.parse(task.result || '{}').image_url || ''
  } catch {
    return ''
  }
}

export function isCompletedFeatureTaskWithImage(task: Task, featurePrefix: string): boolean {
  if (task.status !== 'completed') return false
  if (!task.prompt || !task.prompt.startsWith(featurePrefix)) return false
  return !!getTaskImageUrl(task)
}

export function downloadImageByProxy(url?: string, fileName = DEFAULT_IMAGE_DOWNLOAD_NAME) {
  if (!url) return
  const a = document.createElement('a')
  a.href = `${USER_DOWNLOAD_ENDPOINT}?url=${encodeURIComponent(url)}`
  a.download = fileName
  a.click()
}
