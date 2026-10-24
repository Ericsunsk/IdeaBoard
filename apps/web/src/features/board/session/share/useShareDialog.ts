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
  // Catalog meta already carries workspaceAccess/accessState: private boards
  // (workspaceAccess none) render the Off state from facts without an access
  // GET; the version + member list fetch on first share write via
  // resolveAttempt. Shared boards still fetch members for selection display.
  const accessEnabled = () =>
    options.panel.isOpen.value &&
    snapshot.value !== null &&
    canManage.value &&
    snapshot.value.facts.workspaceAccess !== 'none'
  const accessQuery = useQuery(
    computed(() => ({
      ...shareAccessOptions(
        snapshot.value?.scope.workspaceId ?? '',
        snapshot.value?.scope.boardId ?? '',
        transport.fetchAccess,
      ),
      enabled: accessEnabled,
    })),
  )
  // A private catalog fact supersedes access data cached before revocation.
  // Its query is disabled, so that old data cannot refresh on invalidation.
  const sharedAccess = computed(() =>
    facts.value?.workspaceAccess === 'none' ? undefined : accessQuery.data.value,
  )
  const workspaceAccess = computed(
    () => sharedAccess.value?.workspaceAccess ?? facts.value?.workspaceAccess ?? 'none',
  )
  const memberUserIds = computed(() => sharedAccess.value?.memberUserIds ?? [])
  const rosterEnabled = () =>
    options.panel.isOpen.value && snapshot.value !== null && canManage.value
  const roster = useShareRoster({
    transport,
    enabled: rosterEnabled,
    scope: () => snapshot.value?.scope ?? null,
    creatorId: () => facts.value?.creatorUserId ?? '',
    selectedIds: () => memberUserIds.value,
  })
  const isBusy = computed(
    () =>
      isOpening.value ||
      isSaving.value ||
      (rosterEnabled() && roster.query.isPending.value) ||
      (accessEnabled() && accessQuery.isPending.value),
  )
  const hasMembersError = computed(
    () =>
      (accessEnabled() && accessQuery.isError.value) ||
      (rosterEnabled() && roster.query.isError.value),
  )
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
      snapshot.value = { scope, generation, facts }
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
      (snap.facts.workspaceAccess === 'none' ? undefined : accessQuery.data.value) ??
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
      snap.facts = {
        ...snap.facts,
        workspaceAccess: result.access.workspaceAccess,
        accessState: result.access.accessState,
      }
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
    if (userId === facts.value?.creatorUserId || workspaceAccess.value === 'none') return
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
      () => sharedAccess.value?.accessState ?? facts.value?.accessState ?? 'private',
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
      // Directed retry: only the failed instance refetches so a roster error
      // never refires the access GET and vice versa.
      if (accessEnabled() && accessQuery.isError.value) void accessQuery.refetch()
      if (rosterEnabled() && roster.query.isError.value) void roster.query.refetch()
    },
  }
}

export type ShareDialogController = ReturnType<typeof useShareDialog>
