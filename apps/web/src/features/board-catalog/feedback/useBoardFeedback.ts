import { useAppToast } from '../../../shared/composables/useAppToast'
import type {
  CanvasBoardCreateResult,
  CanvasBoardDeleteResult,
  CanvasBoardDuplicateResult,
} from '../../canvas-board/catalog/canvasBoardCommands'
import type { CanvasBoardCatalogRenameResult } from '../../canvas-board/catalog/canvasBoardCatalog'

type BoardToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'warning' | 'error'>

type FailedCreateResult = Extract<CanvasBoardCreateResult, { ok: false }>
type FailedRenameResult = Extract<CanvasBoardCatalogRenameResult, { ok: false }>
type RenameValidationFailure = Extract<
  FailedRenameResult,
  { reason: 'local-name-conflict' | 'invalid-name' }
>
type RenameSyncFailure = Extract<
  FailedRenameResult,
  { reason: 'remote-name-conflict' | 'sync-failed' }
>
type FailedDuplicateResult = Extract<CanvasBoardDuplicateResult, { ok: false }>
type FailedDeleteResult = Extract<CanvasBoardDeleteResult, { ok: false }>

export type BoardFeedback = {
  createFailed: (result: FailedCreateResult) => void
  renameValidationFailed: (result: RenameValidationFailure) => void
  renameSyncFailed: (result: RenameSyncFailure) => void
  renamed: () => void
  copyFailed: (result: FailedDuplicateResult) => void
  copied: () => void
  deleteFailed: (result: FailedDeleteResult) => void
  deleted: () => void
  catalogSyncFailed: () => void
  catalogReloaded: () => void
}

export function createBoardFeedback(appToast: BoardToast): BoardFeedback {
  return {
    createFailed(result) {
      appToast.error(
        result.reason === 'name-conflict' ? '画布名称已存在，请重试' : '创建同步失败，请稍后重试',
      )
    },
    renameValidationFailed(result) {
      appToast.warning(result.reason === 'local-name-conflict' ? '名称已存在' : '画布名称不能为空')
    },
    renameSyncFailed(result) {
      if (result.reason === 'remote-name-conflict' && result.hydrate?.ok) {
        appToast.error('名称冲突，已重新加载画布列表')
        return
      }
      if (result.reason === 'sync-failed' && result.rollback.ok) {
        appToast.error('重命名同步失败，已恢复原名称')
        return
      }
      appToast.error(
        result.reason === 'remote-name-conflict'
          ? '名称冲突且恢复未完成，请重新加载画布列表'
          : '重命名同步失败且无法恢复，请重新加载画布列表',
      )
    },
    renamed() {
      appToast.success('画布已重命名')
    },
    copyFailed(result) {
      if (result.reason === 'source-missing') {
        appToast.error('复制失败')
        return
      }
      appToast.error(
        result.reason === 'name-conflict'
          ? '复制后的画布名称已存在，请重试'
          : '复制同步失败，请稍后重试',
      )
    },
    copied() {
      appToast.success('已复制画布')
    },
    deleteFailed(result) {
      appToast.error(result.reason === 'sync-failed' ? '删除失败，请稍后重试' : '删除失败')
    },
    deleted() {
      appToast.success('已删除画布')
    },
    catalogSyncFailed() {
      appToast.error('画布同步失败，请稍后重试')
    },
    catalogReloaded() {
      appToast.success('画布列表已重新加载')
    },
  }
}

export function useBoardFeedback() {
  return createBoardFeedback(useAppToast())
}
