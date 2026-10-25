import { computed, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { useI18n } from 'vue-i18n'
import { organizationCreationIdempotencyKey } from '../../../api/idempotencyKey'
import { useAppToast } from '../../../core/composables/useAppToast'

type OrganizationItem = { publicId: string; name: string }

export function useWorkspaceCreation(options: {
  workspaces: MaybeRefOrGetter<readonly { publicId: string; organizationId: string | null }[]>
  activeWorkspaceId: MaybeRefOrGetter<string>
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
  // Billing scope (ADR-0038) is the creation scope and nothing else:
  // Personal workspaces resolve to the caller's Personal account,
  // Organization workspaces to the shared org pool. No billing choice
  // exists, and the scope never switches after creation.
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
  const hasOrganizations = computed(() => createScopeOptions.value.length > 0)
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
      !isOrganizationFormOpen.value,
  )
  const createScopeHint = computed(() => {
    const orgId = createOrganizationId.value
    if (orgId) {
      const orgName =
        createScopeOptions.value.find((item) => item.publicId === orgId)?.name ?? orgId
      return t('workspace.switcher.scopeOrganizationHint', { organization: orgName })
    }
    return t('workspace.switcher.scopePersonalHint')
  })
  function prepare(): void {
    if (!hasOrganizations.value) {
      createScopeId.value = 'personal'
    } else {
      createScopeId.value = activeWorkspace.value?.organizationId ?? 'personal'
    }
    isOrganizationFormOpen.value = false
    organizationError.value = null
    void loadOrganizations()
  }
  function fundingInput(): { organizationId: string } | Record<string, never> {
    if (createOrganizationId.value) return { organizationId: createOrganizationId.value }
    return {}
  }
  return {
    createScopeHint,
    createScopeId,
    createOrganizationId,
    createScopeOptions,
    hasOrganizations,
    isOrganizationsLoading,
    organizationLoadError,
    isOrganizationFormOpen,
    organizationName,
    organizationError,
    isCreatingOrganization,
    canSubmitCreate,
    loadOrganizations,
    createOrganization,
    prepare,
    fundingInput,
  }
}
