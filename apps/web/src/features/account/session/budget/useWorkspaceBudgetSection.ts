import { computed, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { useI18n } from 'vue-i18n'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { accountScoped, noRetry } from '../../../../core/query/queryDefaults'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import {
  fetchWorkspaceBudget,
  isBudgetExhausted as isExhausted,
  MAX_WORKSPACE_BUDGET_LIMIT,
  updateWorkspaceBudget,
  workspaceBudgetQueryKeys,
  type WorkspaceBudgetSummary,
  type UpdateWorkspaceBudgetInput,
} from './resource'

// Budget section controller for the workspace settings dialog. Usage
// rides TanStack Query on the workspace budget key; writes go through
// the budget adapter and converge by invalidation — never optimistic.
// Only billing managers see management controls (`canManage` derives
// from the billing-account consumption scope the dialog resolves); the
// backend still enforces 403 for every write.
type WorkspaceBudgetSectionOptions = {
  workspaceId: MaybeRefOrGetter<string>
  /** Billing-account consumption scope: 'account' means the caller is a billing manager. */
  consumptionScope: MaybeRefOrGetter<string | null | undefined>
  isExpanded: MaybeRefOrGetter<boolean>
}

export function useWorkspaceBudgetSection(options: WorkspaceBudgetSectionOptions) {
  const { t } = useI18n()
  const toast = useAppToast()
  const queryClient = useQueryClient()

  const workspaceIdOf = (): string => toValue(options.workspaceId).trim()

  const budgetQuery = useQuery({
    queryKey: computed(() => workspaceBudgetQueryKeys.detail(workspaceIdOf())),
    queryFn: ({ signal }) => fetchWorkspaceBudget(workspaceIdOf(), signal),
    enabled: computed(() => toValue(options.isExpanded) && workspaceIdOf() !== ''),
    ...accountScoped(),
    ...noRetry(),
  })

  const budget = computed<WorkspaceBudgetSummary | null>(() => budgetQuery.data.value ?? null)
  const isLoading = computed(() => budgetQuery.isLoading.value)
  const hasLoadError = computed(() => budgetQuery.isError.value)
  const canManage = computed(() => toValue(options.consumptionScope) === 'account')
  const isExhaustedBudget = computed(() => (budget.value ? isExhausted(budget.value) : false))

  function retryBudget(): void {
    void budgetQuery.refetch()
  }

  const limitDraft = ref('')
  const isSaving = ref(false)
  const saveError = ref<string | null>(null)

  function startEditing(): void {
    saveError.value = null
    limitDraft.value =
      budget.value?.period === 'monthly' && budget.value.limitCredits != null
        ? String(budget.value.limitCredits)
        : ''
  }

  function validateLimit(raw: string): number | null {
    const trimmed = raw.trim()
    if (!/^\d+$/.test(trimmed)) return null
    const value = Number(trimmed)
    if (!Number.isSafeInteger(value) || value <= 0 || value > MAX_WORKSPACE_BUDGET_LIMIT) {
      return null
    }
    return value
  }

  function describeSaveError(error: unknown): string {
    const code = userErrorCode(error)
    return code != null
      ? `${t('workspace.budget.saveFailed')} (${code})`
      : t('workspace.budget.saveFailed')
  }

  async function save(input: UpdateWorkspaceBudgetInput): Promise<void> {
    if (isSaving.value) return
    isSaving.value = true
    saveError.value = null
    try {
      await updateWorkspaceBudget(workspaceIdOf(), input)
      await queryClient.invalidateQueries({
        queryKey: workspaceBudgetQueryKeys.detail(workspaceIdOf()),
      })
      toast.success(t('workspace.budget.saved'))
    } catch (error) {
      saveError.value = describeSaveError(error)
      toast.error(saveError.value)
    } finally {
      isSaving.value = false
    }
  }

  async function saveMonthly(): Promise<void> {
    if (isSaving.value) return
    const limit = validateLimit(limitDraft.value)
    if (limit == null) {
      saveError.value = t('workspace.budget.invalidLimit', { max: MAX_WORKSPACE_BUDGET_LIMIT })
      return
    }
    await save({ period: 'monthly', limitCredits: limit })
  }

  async function saveUnlimited(): Promise<void> {
    await save({ period: 'unlimited' })
  }

  return {
    budget,
    isLoading,
    hasLoadError,
    canManage,
    isExhausted: isExhaustedBudget,
    retryBudget,
    limitDraft,
    isSaving,
    saveError,
    startEditing,
    saveMonthly,
    saveUnlimited,
  }
}
