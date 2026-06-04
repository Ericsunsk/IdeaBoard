import { h } from 'vue'
import { NTag, NImage, NPopover, NButton, NIcon } from 'naive-ui'
import { Eye } from '@iconoir/vue'
import type { DataTableColumns } from 'naive-ui'
import type { Task } from '../../../../shared/api/task'
import { sanitizeTaskErrorMessage } from '../../../../shared/utils/errorMessages'
import { calcDurationSeconds, formatAdminDateTime } from '../../../../shared/utils/dateTime'
import { PUBLIC_ASSET_BASE_URL, resolveAssetUrl } from '../../../../shared/config/runtimeUrls'

type TaskStatusTagType = 'default' | 'warning' | 'success' | 'error'

interface TaskStatusConfig {
  type: TaskStatusTagType
  text: string
  color: string
}

type MonitorTaskRow = Task & {
  username?: string
  email?: string
  user?: {
    username?: string
    email?: string
  }
}

export const ERROR_TEXT_COLOR = 'var(--admin-danger)'
export const DEFAULT_STATUS_COLOR = 'var(--admin-text-3)'

const HTTP_URL_RE = /^https?:\/\//i
const UPLOAD_ASSET_PATH_PREFIX = '/uploads/'
const ENCODED_BACKSLASH_RE = /%5c/i

export const taskStatusOptions = [
  { label: '等待中', value: 'pending' },
  { label: '执行中', value: 'running' },
  { label: '已完成', value: 'completed' },
  { label: '失败', value: 'failed' },
]

const taskStatusConfigMap: Record<string, TaskStatusConfig> = {
  pending: { type: 'default', text: '等待中', color: DEFAULT_STATUS_COLOR },
  running: { type: 'warning', text: '执行中', color: '#f59e0b' },
  completed: { type: 'success', text: '已完成', color: '#22c55e' },
  failed: { type: 'error', text: '失败', color: ERROR_TEXT_COLOR },
}

function getCurrentOrigin(): string {
  if (typeof window !== 'undefined' && window.location.origin) return window.location.origin
  return 'http://ideaboard.local'
}

function parseResultImageUrl(rawUrl: string): URL | null {
  try {
    return new URL(rawUrl, getCurrentOrigin())
  } catch {
    return null
  }
}

function getTrustedAssetBaseUrl(): URL | null {
  if (!PUBLIC_ASSET_BASE_URL) return null
  return parseResultImageUrl(PUBLIC_ASSET_BASE_URL)
}

function hasUploadAssetPath(pathname: string, basePathname = ''): boolean {
  const normalizedBase = basePathname.replace(/\/+$/, '')
  const uploadPath = `${normalizedBase}${UPLOAD_ASSET_PATH_PREFIX}`
  return pathname.startsWith(uploadPath)
}

function isTrustedUploadUrl(url: URL): boolean {
  const currentOrigin = getCurrentOrigin()
  if (url.origin === currentOrigin && hasUploadAssetPath(url.pathname)) return true

  const trustedAssetBase = getTrustedAssetBaseUrl()
  return (
    !!trustedAssetBase &&
    url.origin === trustedAssetBase.origin &&
    hasUploadAssetPath(url.pathname, trustedAssetBase.pathname)
  )
}

export function getSafeResultImageUrl(rawUrl: string): string {
  const trimmed = rawUrl.trim()
  if (
    !trimmed ||
    trimmed.startsWith('//') ||
    trimmed.includes('\\') ||
    ENCODED_BACKSLASH_RE.test(trimmed)
  ) {
    return ''
  }

  if (!trimmed.startsWith('/') && !HTTP_URL_RE.test(trimmed)) return ''

  const resolved = resolveAssetUrl(trimmed)
  const parsed = parseResultImageUrl(resolved)
  if (!parsed || !isTrustedUploadUrl(parsed)) return ''

  return resolved
}

export function isAllowedResultImageUrl(rawUrl: string): boolean {
  return getSafeResultImageUrl(rawUrl) !== ''
}

export function getSafeTaskResultImageUrl(result: string): string {
  try {
    const parsed = JSON.parse(result)
    if (parsed && typeof parsed === 'object') {
      const imageUrl = (parsed as Record<string, unknown>).image_url
      const safeImageUrl = typeof imageUrl === 'string' ? getSafeResultImageUrl(imageUrl) : ''
      if (safeImageUrl) return safeImageUrl

      const imageUrls = (parsed as Record<string, unknown>).image_urls
      if (Array.isArray(imageUrls)) {
        for (const candidate of imageUrls) {
          if (typeof candidate !== 'string') continue
          const safeCandidate = getSafeResultImageUrl(candidate)
          if (safeCandidate) return safeCandidate
        }
      }
    }
  } catch {
    return ''
  }
  return ''
}

export function getTaskStatusConfig(status: string): TaskStatusConfig {
  return (
    taskStatusConfigMap[status] || { type: 'default', text: status, color: DEFAULT_STATUS_COLOR }
  )
}

export function createMonitorTaskColumns(options?: {
  onShowDetail?: (row: Task) => void
}): DataTableColumns<Task> {
  return [
    {
      title: '创建时间',
      key: 'created_at',
      width: 130,
      render: (row) => {
        return h(
          'span',
          { style: { fontSize: '12px', color: 'var(--admin-text-1)', fontWeight: '500' } },
          formatAdminDateTime(row.created_at, row.created_at),
        )
      },
    },
    {
      title: '结果',
      key: 'result',
      width: 70,
      align: 'center',
      render: (row) => {
        if (row.status === 'completed' && row.result) {
          const imageUrl = getSafeTaskResultImageUrl(row.result)
          if (!imageUrl) return '-'
          return h(NImage, {
            width: 42,
            height: 42,
            src: imageUrl,
            style: { borderRadius: '4px', cursor: 'pointer', display: 'block' },
            objectFit: 'cover',
            previewDisabled: false,
            lazy: true,
          })
        }
        return '-'
      },
    },
    {
      title: '用户',
      key: 'user',
      width: 100,
      render: (row) => {
        const taskRow = row as MonitorTaskRow
        const identifier =
          taskRow.username ||
          taskRow.email ||
          taskRow.user?.username ||
          taskRow.user?.email ||
          taskRow.user_id
        return h('div', { style: { display: 'flex', alignItems: 'center', gap: '6px' } }, [
          h(
            'span',
            {
              style: {
                fontWeight: '600',
                color: 'var(--admin-text-1)',
                fontSize: '12px',
                maxWidth: '90px',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              },
            },
            identifier,
          ),
        ])
      },
    },
    {
      title: '提示词',
      key: 'prompt',
      width: 120,
      render: (row) => {
        const text = row.prompt || '-'
        return h(
          NPopover,
          { trigger: 'hover', placement: 'top', style: { maxWidth: '400px' } },
          {
            trigger: () =>
              h(
                'span',
                {
                  style: {
                    color: 'var(--admin-text-2)',
                    fontSize: '12px',
                    cursor: 'help',
                    display: '-webkit-box',
                    '-webkit-line-clamp': '1',
                    '-webkit-box-orient': 'vertical',
                    overflow: 'hidden',
                    'word-break': 'break-all',
                  },
                },
                text,
              ),
            default: () => h('div', { style: { fontSize: '12px', lineHeight: '1.6' } }, text),
          },
        )
      },
    },
    { title: '账号', key: 'account_id', width: 50, align: 'center' },
    {
      title: 'API调用',
      key: 'api_call',
      width: 110,
      render: (row) => {
        if (!row.result) return h('span', { style: { color: 'var(--admin-text-3)' } }, '-')
        try {
          const r = JSON.parse(row.result)
          if (r.platform) {
            return h('span', { style: { fontSize: '12px' } }, [
              h(
                NTag,
                { size: 'tiny', type: 'info', bordered: false, style: { borderRadius: '4px' } },
                () => r.platform,
              ),
              r.model_set
                ? h(
                    'span',
                    {
                      style: { marginLeft: '4px', color: 'var(--admin-text-3)', fontSize: '10px' },
                    },
                    r.model_set,
                  )
                : null,
            ])
          }
        } catch {
          return h('span', { style: { color: 'var(--admin-text-3)', fontSize: '12px' } }, '-')
        }
        return h('span', { style: { color: 'var(--admin-text-3)', fontSize: '12px' } }, '-')
      },
    },
    {
      title: '状态',
      key: 'status',
      width: 80,
      render: (row) => {
        const config = getTaskStatusConfig(row.status)
        return h(
          NTag,
          { type: config.type, size: 'small', bordered: false, style: { borderRadius: '6px' } },
          () => config.text,
        )
      },
    },
    {
      title: '耗时',
      key: 'duration',
      width: 60,
      align: 'center',
      render: (row) => {
        if (!row.completed_at || !row.created_at)
          return h('span', { style: { color: 'var(--admin-text-3)' } }, '-')
        const sec = calcDurationSeconds(row.created_at, row.completed_at)
        if (sec == null) return '-'
        const color =
          sec > 60
            ? 'var(--admin-danger)'
            : sec > 30
              ? 'var(--admin-warning)'
              : 'var(--admin-success)'
        return h('span', { style: { fontSize: '12px', color, fontWeight: '500' } }, `${sec}s`)
      },
    },
    {
      title: '错误诊断',
      key: 'info',
      width: 140,
      render: (row) => {
        if (row.status === 'failed' && row.error_msg) {
          return h(
            'span',
            { style: { color: ERROR_TEXT_COLOR, fontSize: '12px' } },
            sanitizeTaskErrorMessage(row.error_msg),
          )
        }
        return h('span', { style: { color: 'var(--admin-text-3)', fontSize: '12px' } }, '-')
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 70,
      align: 'center',
      render: (row) =>
        h(
          NButton,
          {
            size: 'tiny',
            quaternary: true,
            onClick: () => options?.onShowDetail?.(row),
          },
          {
            icon: () => h(NIcon, null, () => h(Eye)),
            default: () => '详情',
          },
        ),
    },
  ]
}
