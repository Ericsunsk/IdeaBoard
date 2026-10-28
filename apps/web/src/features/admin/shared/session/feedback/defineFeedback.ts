import { formatAdminDisplayError, useAdminToast } from './toast'

/**
 * Admin 反馈文案表：领域文件只声明文案，不直接调用 toast。
 *
 * - `{ success }` / `{ info }` / `{ warning }`：静态文案或由参数拼出的文案，直接 toast。
 * - `{ errorText }`：不经 format 的 error 文案（剪贴板、标识回显等确定性提示）。
 * - `{ error }`：经 `formatAdminDisplayError` 的失败文案；函数形式时以 error 之后的参数拼 fallback。
 * - `{ successMessage }` / `{ errorMessage }`：服务端回传 message 优先，缺省用 fallback。
 */
type AdminFeedbackCopyText = string | ((...args: never[]) => string)

type AdminFeedbackEntry =
  | { success: AdminFeedbackCopyText }
  | { info: AdminFeedbackCopyText }
  | { warning: AdminFeedbackCopyText }
  | { errorText: AdminFeedbackCopyText }
  | { error: AdminFeedbackCopyText }
  | { successMessage: string }
  | { errorMessage: string }

/**
 * 文案表与方法签名一一匹配：无参方法用静态文案，带参方法用同参拼接函数；
 * unknown 首参为错误，其余参数供 fallback 使用。可选 message 首参允许
 * successMessage/errorMessage，语气由领域文案决定。
 */
type AdminFeedbackStaticEntry =
  { success: string } | { info: string } | { warning: string } | { errorText: string }

type AdminFeedbackEntryFor<F> = F extends (...args: infer A extends Array<unknown>) => void
  ? A['length'] extends 0
    ? AdminFeedbackStaticEntry
    : F extends (error: unknown, ...rest: infer Rest extends Array<unknown>) => void
      ? { error: string | ((...args: Rest) => string) }
      : undefined extends A[0]
        ? { successMessage: string } | { errorMessage: string }
        : | { success: (...args: A) => string }
          | { warning: (...args: A) => string }
          | { info: (...args: A) => string }
          | { errorText: (...args: A) => string }
  : never

type AdminFeedbackCopy = Record<string, AdminFeedbackEntry>

function renderCopyText(text: AdminFeedbackCopyText, args: Array<unknown>): string {
  if (typeof text === 'string') return text
  return (text as (...args: Array<unknown>) => string)(...args)
}

/**
 * 通用反馈工厂：领域文件只剩文案表声明，调用方签名与行为保持不变。
 * `toast` 由内部 `useAdminToast()` 获取，须在 setup 上下文中调用（与原各 `useXFeedback` 一致）。
 * 文案表 key 必须与 `T` 方法名一一对应（多写少写都编译失败），每种 entry 的
 * 类型必须与对应方法签名匹配（见 `AdminFeedbackEntryFor`）。
 */
export function defineFeedback<T extends Record<string, (...args: never[]) => void>>(copy: {
  [K in keyof T]: AdminFeedbackEntryFor<T[K]>
}): T {
  const toast = useAdminToast()
  const table = copy as unknown as AdminFeedbackCopy
  const built: Record<string, (...args: Array<unknown>) => void> = {}
  for (const [key, entry] of Object.entries(table)) {
    if ('success' in entry) {
      const text = entry.success
      built[key] = (...args: Array<unknown>) => {
        toast.success(renderCopyText(text, args))
      }
    } else if ('info' in entry) {
      const text = entry.info
      built[key] = (...args: Array<unknown>) => {
        toast.info(renderCopyText(text, args))
      }
    } else if ('warning' in entry) {
      const text = entry.warning
      built[key] = (...args: Array<unknown>) => {
        toast.warning(renderCopyText(text, args))
      }
    } else if ('errorText' in entry) {
      const text = entry.errorText
      built[key] = (...args: Array<unknown>) => {
        toast.error(renderCopyText(text, args))
      }
    } else if ('error' in entry) {
      const fallback = entry.error
      built[key] = (error: unknown, ...rest: Array<unknown>) => {
        toast.error(formatAdminDisplayError(error, renderCopyText(fallback, rest)))
      }
    } else if ('successMessage' in entry) {
      const fallback = entry.successMessage
      built[key] = (...args: Array<unknown>) => {
        const message = args[0] as string | null | undefined
        toast.success(message || fallback)
      }
    } else {
      const fallback = entry.errorMessage
      built[key] = (...args: Array<unknown>) => {
        const message = args[0] as string | null | undefined
        toast.error(message || fallback)
      }
    }
  }
  return built as unknown as T
}
