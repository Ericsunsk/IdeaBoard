import { computed, ref, watch } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import type { ShareAccessInput, ShareWorkspaceAccess } from '../adapters/boardsResource'
import {
  captureShareScope,
  classifyShareError,
  isShareScopeCurrent,
  mintShareAttempt,
  replaceShareAccess,
  shareTransport,
  type ShareActionScope,
  type ShareActionResult,
  type ShareAttempt,
  type ShareFailureReason,
  type ShareTransport,
} from './commands'
import { revalidateAndConvergeBoardAccess } from '../revocation/convergeAccessLoss'
import {
  readShareBoardFacts,
  shareAccessOptions,
  toShareBoardFacts,
  type ShareBoardFacts,
} from './queries'
import { useShareFeedback, type ShareFeedback } from './feedback'
import { useShareRoster } from './useShareRoster'
import type { SharePanel } from './useSharePanel'

export type ShareDialogDeps = {
  transport?: ShareTransport
  feedback?: ShareFeedback
  ensureDraftSaved?: (boardId: string) => Promise<boolean>
}

type ShareSnapshot = {
  scope: ShareActionScope
  generation: number
  facts: ShareBoardFacts
  creatorId: string
}

export function useShareDialog(options: ShareDialogDeps & { panel: SharePanel }) {
  const transport = options.transport ?? shareTransport
  const feedback = options.feedback ?? useShareFeedback()
  const client = useQueryClient()
  const snapshot = ref<ShareSnapshot | null>(null)
  const openError = ref<ShareFailureReason | null>(null)
  const isOpening = ref(false)
  const isSaving = ref(false)
  const attempts = new Map<string, ShareAttempt>()
  const facts = computed(() => snapshot.value?.facts ?? null)
  const canManage = computed(() => facts.value?.capabilities.manage === true)
  const enabled = () => options.panel.isOpen.value && snapshot.value !== null && canManage.value
  const accessQuery = useQuery(
    computed(() => ({
      ...shareAccessOptions(
        snapshot.value?.scope.workspaceId ?? '',
        snapshot.value?.scope.boardId ?? '',
        transport.fetchAccess,
      ),
      enabled,
    })),
  )
  const workspaceAccess = computed(
    () => accessQuery.data.value?.workspaceAccess ?? facts.value?.workspaceAccess ?? 'none',
  )
  const memberUserIds = computed(() => accessQuery.data.value?.memberUserIds ?? [])
  const roster = useShareRoster({
    transport,
    enabled,
    scope: () => snapshot.value?.scope ?? null,
    creatorId: () => snapshot.value?.creatorId ?? '',
    selectedIds: () => memberUserIds.value,
  })
  const isBusy = computed(
    () =>
      isOpening.value ||
      isSaving.value ||
      (enabled() && (accessQuery.isPending.value || roster.query.isPending.value)),
  )
  const hasMembersError = computed(() => accessQuery.isError.value || roster.query.isError.value)
  let openToken = 0

  function isSnapshotCurrent(snap: ShareSnapshot): boolean {
    return (
      snapshot.value === snap &&
      options.panel.generation.value === snap.generation &&
      isShareScopeCurrent(snap.scope)
    )
  }

  async function openPanel(): Promise<void> {
    const target = options.panel.target.value
    if (!target) return
    const scope = captureShareScope(target.boardId)
    if (!scope) {
      openError.value = 'board-not-saved'
      return
    }
    const generation = options.panel.generation.value
    const token = ++openToken
    isOpening.value = true
    isSaving.value = false
    snapshot.value = null
    openError.value = null
    attempts.clear()
    try {
      const facts =
        readShareBoardFacts(scope.workspaceId, scope.boardId) ??
        toShareBoardFacts(await transport.fetchMeta(scope.workspaceId, scope.boardId))
      if (!isShareScopeCurrent(scope) || options.panel.generation.value !== generation) return
      if (!facts) {
        openError.value = 'board-not-saved'
        return
      }
      const creatorId = facts.capabilities.manage
        ? await transport.fetchBoardCreatorIdentity(scope.workspaceId, scope.boardId)
        : ''
      if (!isShareScopeCurrent(scope) || options.panel.generation.value !== generation) return
      snapshot.value = { scope, generation, facts, creatorId }
    } catch (error) {
      if (isShareScopeCurrent(scope) && options.panel.generation.value === generation)
        openError.value = classifyShareError(error)
    } finally {
      if (token === openToken) isOpening.value = false
    }
  }

  async function convergeBoardAccessLoss(
    snap: ShareSnapshot,
    reason: ShareFailureReason,
  ): Promise<void> {
    if (reason !== 'forbidden' && reason !== 'not-found') return
    const verdict = await revalidateAndConvergeBoardAccess(snap.scope.boardId, {
      isCurrent: () => isSnapshotCurrent(snap),
      workspaceId: snap.scope.workspaceId,
      fetchMeta: async (...args) => {
        const meta = await transport.fetchMeta(...args)
        if (isSnapshotCurrent(snap)) {
          const latest = toShareBoardFacts(meta)
          if (latest) snap.facts = latest
        }
        return meta
      },
    })
    if (!isSnapshotCurrent(snap)) return
    if (verdict === 'revoked') options.panel.close()
  }

  async function resolveAttempt(key: string, snap: ShareSnapshot): Promise<ShareAttempt> {
    const previous = attempts.get(key)
    if (previous) return previous
    const access =
      accessQuery.data.value ??
      (await transport.fetchAccess(snap.scope.workspaceId, snap.scope.boardId))
    return mintShareAttempt(access.accessVersion)
  }

  async function handleFailure(
    snap: ShareSnapshot,
    key: string,
    attempt: ShareAttempt,
    result: Exclude<ShareActionResult, { isOk: true }>,
  ): Promise<void> {
    if (result.reason === 'sync-failed') attempts.set(key, attempt)
    else attempts.delete(key)
    if (result.reason === 'scope-changed') return
    feedback.actionFailed(result.reason)
    await convergeBoardAccessLoss(snap, result.reason)
  }

  async function replaceAccess(input: ShareAccessInput): Promise<void> {
    const snap = snapshot.value
    if (!snap || !canManage.value || isBusy.value || hasMembersError.value) return
    const key = JSON.stringify([input.workspaceAccess, [...input.memberUserIds].sort()])
    isSaving.value = true
    try {
      if (options.ensureDraftSaved && !(await options.ensureDraftSaved(snap.scope.boardId))) {
        if (isSnapshotCurrent(snap)) feedback.actionFailed('board-not-saved')
        return
      }
      if (!isSnapshotCurrent(snap)) return
      const attempt = await resolveAttempt(key, snap)
      if (!isSnapshotCurrent(snap)) return
      const result = await replaceShareAccess({ transport, client }, snap.scope, attempt, input)
      if (!isSnapshotCurrent(snap)) return
      if (!result.isOk) {
        await handleFailure(snap, key, attempt, result)
        return
      }
      attempts.delete(key)
      feedback.accessUpdated()
    } catch (error) {
      if (isSnapshotCurrent(snap)) feedback.actionFailed(classifyShareError(error))
    } finally {
      if (isSnapshotCurrent(snap)) isSaving.value = false
    }
  }

  async function setWorkspaceAccess(value: ShareWorkspaceAccess): Promise<void> {
    if (value === workspaceAccess.value) return
    await replaceAccess({
      workspaceAccess: value,
      memberUserIds: value === 'none' ? [] : [...memberUserIds.value],
    })
  }

  async function toggleMember(userId: string): Promise<void> {
    if (userId === snapshot.value?.creatorId || workspaceAccess.value === 'none') return
    const selected = new Set(memberUserIds.value)
    if (selected.has(userId)) selected.delete(userId)
    else selected.add(userId)
    await replaceAccess({
      workspaceAccess: workspaceAccess.value,
      memberUserIds: [...selected].sort(),
    })
  }

  watch(
    [() => options.panel.isOpen.value, () => options.panel.generation.value],
    ([isOpen]) => {
      if (isOpen) void openPanel()
      else {
        snapshot.value = null
        isSaving.value = false
      }
    },
    { immediate: true },
  )

  return {
    facts,
    canManage,
    isOpening,
    openError,
    isBusy,
    hasMembersError,
    workspaceAccess,
    accessState: computed(
      () => accessQuery.data.value?.accessState ?? facts.value?.accessState ?? 'private',
    ),
    memberRows: roster.memberRows,
    setWorkspaceAccess,
    toggleMember,
    openPanel,
    retryOpen: () => {
      snapshot.value = null
      void openPanel()
    },
    retryQueries: () => {
      void accessQuery.refetch()
      void roster.query.refetch()
    },
  }
}

export type ShareDialogController = ReturnType<typeof useShareDialog>
