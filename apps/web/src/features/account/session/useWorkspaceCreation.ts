import { computed, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useI18n } from 'vue-i18n'
import { organizationCreationIdempotencyKey } from '../../../api/idempotencyKey'
import { useAppToast } from '../../../core/composables/useAppToast'
import { fetchBillingAccount } from './billing-account/resource'

type OrganizationItem = { publicId: string; name: string }

export function useWorkspaceCreation(options: {
  workspaces: MaybeRefOrGetter<readonly { publicId: string; organizationId: string | null }[]>
  activeWorkspaceId: MaybeRefOrGetter<string>
  isOpen: MaybeRefOrGetter<boolean>
  isCreating: MaybeRefOrGetter<boolean>
  nameMaxLength: MaybeRefOrGetter<number>
  onLoadOrganizations?: (() => Promise<OrganizationItem[]>) | undefined
  onCreateOrganization?:
    ((name: string, idempotencyKey: string) => Promise<OrganizationItem>) | undefined
}) {
  const { t } = useI18n()
  const toast = useAppToast()
  const activeWorkspace = computed(() =>
    toValue(options.workspaces).find(
      (item) => item.publicId === toValue(options.activeWorkspaceId),
    ),
  )
  // The dialog pins its source account. An unresolved shared payer never falls
  // through to the server's personal Default account.
  const createBillingMode = ref<'shared' | 'new'>('shared')
  const sharedBillingAccountId = ref<string | null>(null)
  const isSharedAccountLoading = ref(false)
  const sharedAccountError = ref<string | null>(null)
  let createAccountRequest = 0
  const createScopeId = ref('personal')
  const createOrganizationId = computed(() =>
    createScopeId.value === 'personal' ? null : createScopeId.value,
  )
  const organizationItems = ref<OrganizationItem[]>([])
  const isOrganizationsLoading = ref(false)
  const organizationLoadError = ref<string | null>(null)
  const isOrganizationFormOpen = ref(false)
  const organizationName = ref('')
  const organizationError = ref<string | null>(null)
  const isCreatingOrganization = ref(false)
  let organizationIntent: { name: string; key: string } | null = null
  let organizationLoadVersion = 0
  const createScopeOptions = computed(() => {
    const known = new Map(
      organizationItems.value.map((organization) => [organization.publicId, organization]),
    )
    for (const item of toValue(options.workspaces)) {
      if (item.organizationId && !known.has(item.organizationId)) {
        known.set(item.organizationId, { publicId: item.organizationId, name: item.organizationId })
      }
    }
    return [...known.values()]
  })
  const createSourceWorkspaceId = computed(() => {
    const inScope = toValue(options.workspaces).filter(
      (item) => item.organizationId === createOrganizationId.value,
    )
    return (
      inScope.find((item) => item.publicId === toValue(options.activeWorkspaceId))?.publicId ??
      inScope[0]?.publicId ??
      ''
    )
  })
  async function loadOrganizations(): Promise<void> {
    if (!options.onLoadOrganizations) return
    const version = ++organizationLoadVersion
    isOrganizationsLoading.value = true
    organizationLoadError.value = null
    try {
      const organizations = await options.onLoadOrganizations()
      if (version === organizationLoadVersion) organizationItems.value = organizations
    } catch {
      if (version === organizationLoadVersion)
        organizationLoadError.value = t('workspace.switcher.organization.loadFailed')
    } finally {
      if (version === organizationLoadVersion) isOrganizationsLoading.value = false
    }
  }
  async function createOrganization(): Promise<void> {
    if (!options.onCreateOrganization || isCreatingOrganization.value) return
    const name = organizationName.value.trim()
    if (!name || name.length > toValue(options.nameMaxLength)) {
      organizationError.value = t('workspace.switcher.organization.invalidName', {
        max: toValue(options.nameMaxLength),
      })
      return
    }
    if (organizationIntent?.name !== name)
      organizationIntent = { name, key: organizationCreationIdempotencyKey() }
    isCreatingOrganization.value = true
    ++organizationLoadVersion
    isOrganizationsLoading.value = false
    organizationError.value = null
    try {
      const organization = await options.onCreateOrganization(name, organizationIntent.key)
      organizationItems.value = [
        ...organizationItems.value.filter((item) => item.publicId !== organization.publicId),
        organization,
      ]
      createScopeId.value = organization.publicId
      isOrganizationFormOpen.value = false
      organizationIntent = null
      organizationName.value = ''
      toast.success(t('workspace.switcher.organization.created'))
    } catch {
      organizationError.value = t('workspace.switcher.organization.createFailed')
    } finally {
      isCreatingOrganization.value = false
    }
  }
  const canSubmitCreate = computed(
    () =>
      !toValue(options.isCreating) &&
      !isCreatingOrganization.value &&
      !isOrganizationFormOpen.value &&
      (createOrganizationId.value != null ||
        (createBillingMode.value === 'new' && !createOrganizationId.value) ||
        (!isSharedAccountLoading.value &&
          sharedBillingAccountId.value != null &&
          sharedAccountError.value == null)),
  )
  const sharedAccountHint = computed(() => {
    if (createOrganizationId.value)
      return t('workspace.switcher.billingOrganizationHint', {
        organization: createOrganizationId.value,
      })
    const current = sharedBillingAccountId.value
    return current
      ? t('workspace.switcher.billingSharedHint', { account: current })
      : t('workspace.switcher.billingSharedHintFallback')
  })

  async function loadSharedAccountForCreate(): Promise<void> {
    const request = ++createAccountRequest
    const workspaceId = createSourceWorkspaceId.value
    sharedBillingAccountId.value = null
    sharedAccountError.value = null
    isSharedAccountLoading.value = true
    try {
      const account = await fetchBillingAccount(workspaceId)
      if (request !== createAccountRequest) return
      sharedBillingAccountId.value = account.billingAccountId
    } catch {
      if (request !== createAccountRequest) return
      sharedAccountError.value = t('account.errors.billingAccountLoadFailed')
    } finally {
      if (request === createAccountRequest) isSharedAccountLoading.value = false
    }
  }

  watch([() => toValue(options.isOpen), createSourceWorkspaceId, createScopeId], ([isOpen]) => {
    ++createAccountRequest
    sharedBillingAccountId.value = null
    sharedAccountError.value = null
    isSharedAccountLoading.value = false
    if (isOpen) {
      createBillingMode.value = 'shared'
      if (!createOrganizationId.value) void loadSharedAccountForCreate()
    }
  })

  watch(
    () => toValue(options.activeWorkspaceId),
    () => {
      if (toValue(options.isOpen) && !toValue(options.isCreating)) {
        createScopeId.value = activeWorkspace.value?.organizationId ?? 'personal'
      }
    },
  )

  function prepare(): void {
    createBillingMode.value = 'shared'
    sharedBillingAccountId.value = null
    createScopeId.value = activeWorkspace.value?.organizationId ?? 'personal'
    isOrganizationFormOpen.value = false
    organizationError.value = null
    void loadOrganizations()
  }
  function fundingInput():
    | { organizationId: string }
    | { isNewBillingAccount: true }
    | { billingAccountId: string }
    | null {
    if (!canSubmitCreate.value) return null
    if (createOrganizationId.value) return { organizationId: createOrganizationId.value }
    if (createBillingMode.value === 'new') return { isNewBillingAccount: true }
    return sharedBillingAccountId.value ? { billingAccountId: sharedBillingAccountId.value } : null
  }
  return {
    createBillingMode,
    sharedAccountHint,
    isSharedAccountLoading,
    sharedAccountError,
    createScopeId,
    createOrganizationId,
    createScopeOptions,
    isOrganizationsLoading,
    organizationLoadError,
    isOrganizationFormOpen,
    organizationName,
    organizationError,
    isCreatingOrganization,
    canSubmitCreate,
    loadSharedAccountForCreate,
    loadOrganizations,
    createOrganization,
    prepare,
    fundingInput,
  }
}
