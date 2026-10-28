<template>
  <DialogRoot :open="props.isOpen" @update:open="emits('update:isOpen', $event)">
    <DialogPortal>
      <DialogOverlay />
      <DialogContent
        :class="cn(dialogContentBaseClass, accountDialogContentClass, '!p-0 !overflow-hidden')"
        :show-close="false"
      >
        <div :class="accountMainContentClass">
          <!-- Close button in top right, matching AccountDialog -->
          <Button
            type="button"
            variant="ghost"
            :class="accountCloseButtonClass"
            :aria-label="t('common.actions.close')"
            @click="emits('update:isOpen', false)"
          >
            <Xmark class="h-4 w-4" />
          </Button>

          <!-- Left Sidebar (bg-card) -->
          <aside
            class="flex min-h-0 flex-col overflow-y-auto bg-card px-4 py-4 md:py-5 [scrollbar-width:none]"
          >
            <!-- Title -->
            <div class="mb-5 flex min-w-0 items-center justify-between px-1">
              <DialogTitle
                as="h2"
                class="m-0 text-admin-heading leading-tight font-bold text-foreground"
              >
                {{ t('workspace.switcher.title') }}
              </DialogTitle>
              <DialogDescription class="sr-only">
                {{ t('workspace.switcher.listLabel') }}
              </DialogDescription>
            </div>

            <!-- Workspace List Nav: Personal + Organization groups, always expanded -->
            <nav
              class="flex-1 overflow-y-auto space-y-5"
              :aria-label="t('workspace.switcher.listLabel')"
            >
              <section
                aria-labelledby="workspace-settings-group-personal"
                data-settings-group="personal"
              >
                <p
                  id="workspace-settings-group-personal"
                  class="mb-2 px-2 text-admin-meta leading-5 font-semibold tracking-normal text-muted-foreground"
                >
                  {{ t('workspace.switcher.scopePersonalTitle') }}
                </p>
                <div class="space-y-1">
                  <Button
                    v-for="item in personalWorkspaces"
                    :key="item.publicId"
                    type="button"
                    variant="ghost"
                    :class="accountNavTabClass(isWorkspaceActive(item))"
                    :aria-current="isWorkspaceActive(item) ? 'page' : undefined"
                    :title="workspaceItemName(item)"
                    :data-workspace-nav="item.publicId"
                    @click="selectWorkspace(item)"
                  >
                    <span class="truncate">{{ workspaceItemName(item) }}</span>
                  </Button>
                  <p
                    v-if="personalWorkspaces.length === 0"
                    class="px-2 py-4 text-xs text-muted-foreground"
                    aria-live="polite"
                  >
                    {{ t('workspace.switcher.empty') }}
                  </p>
                </div>
              </section>

              <section
                aria-labelledby="workspace-settings-group-organization"
                data-settings-group="organization"
              >
                <p
                  id="workspace-settings-group-organization"
                  class="mb-2 px-2 text-admin-meta leading-5 font-semibold tracking-normal text-muted-foreground"
                >
                  {{
                    props.organizationPanel?.name ?? t('workspace.switcher.scopeOrganizationTitle')
                  }}
                </p>
                <div class="space-y-1">
                  <Button
                    type="button"
                    variant="ghost"
                    :class="accountNavTabClass(isOrganizationSettingsSelected)"
                    :aria-current="isOrganizationSettingsSelected ? 'page' : undefined"
                    :title="t('workspace.switcher.settingsAction')"
                    data-organization-settings="true"
                    @click="selectOrganizationSettings"
                  >
                    <span class="truncate">{{ t('workspace.switcher.settingsAction') }}</span>
                  </Button>
                  <Button
                    v-for="item in organizationWorkspaces"
                    :key="item.publicId"
                    type="button"
                    variant="ghost"
                    :class="accountNavTabClass(isWorkspaceActive(item))"
                    :aria-current="isWorkspaceActive(item) ? 'page' : undefined"
                    :title="workspaceItemName(item)"
                    :data-workspace-nav="item.publicId"
                    @click="selectWorkspace(item)"
                  >
                    <span class="truncate">{{ workspaceItemName(item) }}</span>
                  </Button>
                </div>
              </section>
            </nav>

            <!-- Add Workspace Button at Bottom of Sidebar -->
            <div class="mt-auto pt-4 flex flex-col gap-1.5">
              <Button
                type="button"
                variant="default"
                size="sm"
                class="w-full h-9 justify-center gap-1.5"
                @click="openCreateDialog"
              >
                <Plus class="size-3.5" aria-hidden="true" />
                <span>{{ t('workspace.switcher.createAction') }}</span>
              </Button>
            </div>
          </aside>

          <main :class="accountContentPaneClass">
            <!-- Loading State -->
            <div
              v-if="props.isLoading"
              class="flex flex-1 min-h-60 flex-col items-center justify-center gap-3"
              aria-live="polite"
            >
              <Spinner class="size-8 text-primary" />
              <span class="text-xs text-muted-foreground">
                {{ t('common.actions.loading') }}
              </span>
            </div>

            <!-- UserAccountMenu reports list failures; the dialog provides the retry action. -->
            <div
              v-else-if="props.loadError"
              class="flex flex-col items-start gap-3 rounded-control border border-border bg-card p-4 text-sm text-foreground"
              aria-live="polite"
            >
              <Button type="button" size="sm" @click="props.onRetry()">
                {{ t('common.actions.retry') }}
              </Button>
            </div>

            <!-- Organization settings pane: mutually exclusive with workspace rows -->
            <div v-else-if="isOrganizationSettingsSelected" class="flex flex-col gap-6">
              <component
                :is="props.organizationPanel.component"
                v-if="props.organizationPanel"
                v-bind="props.organizationPanel.bindings"
                @dismiss="dismissOrganizationPanel"
              />
            </div>

            <!-- Active Workspace Detail (per-workspace pane; the scope-level
              organization panel lives in the Settings pane only) -->
            <div v-else-if="activeWorkspace" class="flex flex-col gap-6">
              <template v-if="activeWorkspace">
                <!-- Header Section -->
                <header class="flex flex-col gap-2">
                  <div v-if="editingId === activeWorkspace.publicId" class="flex flex-col gap-2">
                    <div class="flex items-center gap-2">
                      <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The rename input carries its accessible name via aria-label -->
                      <Input
                        ref="renameInputRef"
                        v-model="draftName"
                        size="sm"
                        class="h-9 max-w-xs text-sm bg-background"
                        :invalid="renameError !== null"
                        :disabled="isRenaming"
                        :aria-label="
                          t('workspace.switcher.renameAriaLabel', {
                            name: workspaceItemName(activeWorkspace),
                          })
                        "
                        :maxlength="nameFieldMaxLength"
                        @keydown.enter="commitRename(activeWorkspace)"
                        @keydown.esc.stop.prevent="cancelRename"
                        @keydown.escape.stop.prevent="cancelRename"
                      />
                      <Button
                        type="button"
                        size="sm"
                        class="h-9 shrink-0"
                        :is-loading="isRenaming"
                        :is-disabled="isRenaming"
                        @click="commitRename(activeWorkspace)"
                      >
                        {{ t('common.actions.save') }}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        class="h-9 shrink-0"
                        :is-disabled="isRenaming"
                        @click="cancelRename"
                      >
                        {{ t('common.actions.cancel') }}
                      </Button>
                    </div>
                    <p
                      v-if="renameError"
                      class="m-0 text-xs leading-normal text-destructive"
                      role="alert"
                    >
                      {{ renameError }}
                    </p>
                  </div>
                  <div v-else class="flex items-center justify-between">
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-2">
                        <h2
                          class="m-0 text-admin-modal-title leading-tight font-bold text-foreground truncate"
                        >
                          {{ workspaceItemName(activeWorkspace) }}
                        </h2>
                        <Button
                          v-if="activeCapabilities.canRename"
                          type="button"
                          variant="ghost"
                          size="icon"
                          class="size-7 text-muted-foreground hover:text-foreground"
                          :title="
                            t('workspace.switcher.renameAriaLabel', {
                              name: workspaceItemName(activeWorkspace),
                            })
                          "
                          :aria-label="
                            t('workspace.switcher.renameAriaLabel', {
                              name: workspaceItemName(activeWorkspace),
                            })
                          "
                          @click="startRename(activeWorkspace)"
                        >
                          <EditPencil class="size-3.5" aria-hidden="true" />
                        </Button>
                      </div>
                      <p class="m-0 mt-1 text-xs text-muted-foreground font-mono">
                        ID: {{ activeWorkspace.publicId }}
                      </p>
                    </div>
                  </div>
                </header>

                <!-- Members Section inside accountCardPanelClass -->
                <WorkspaceMemberSection
                  :key="activeWorkspace.publicId"
                  :workspace-id="activeWorkspace.publicId"
                  :workspace-name="workspaceItemName(activeWorkspace)"
                  :current-user-id="props.currentUserId"
                  :is-expanded="true"
                  :is-default-workspace="isDefaultWorkspaceItem(activeWorkspace)"
                />

                <!-- Quota management uses the account's server-resolved capability. -->
                <WorkspaceLimitsSection
                  :key="`limits-${activeWorkspace.publicId}`"
                  :workspace-id="activeWorkspace.publicId"
                  :can-manage="billingAccount.canManageMemberQuotas.value"
                  :is-expanded="true"
                />

                <!-- Delete row: hidden for the Default workspace (never deleted). -->
                <section
                  v-if="activeCapabilities.canDelete"
                  class="flex flex-col gap-2 rounded-control border border-border p-3"
                  :aria-label="t('workspace.switcher.archiveSectionLabel')"
                >
                  <div class="flex items-center justify-between gap-2">
                    <div class="min-w-0">
                      <p class="m-0 text-sm font-medium text-foreground">
                        {{ t('workspace.switcher.archiveTitle') }}
                      </p>
                      <p class="m-0 mt-0.5 text-xs text-muted-foreground">
                        {{ t('workspace.switcher.archiveHint') }}
                      </p>
                    </div>
                    <Button
                      type="button"
                      variant="destructive"
                      size="sm"
                      class="h-9 shrink-0"
                      :is-disabled="isDeleting"
                      :aria-label="t('workspace.switcher.archiveAction')"
                      @click="requestDelete(activeWorkspace)"
                    >
                      {{ t('workspace.switcher.archiveAction') }}
                    </Button>
                  </div>
                </section>
              </template>
            </div>

            <!-- If no workspace is selected -->
            <div
              v-else
              class="flex flex-1 items-center justify-center text-sm text-muted-foreground"
            >
              {{ t('workspace.switcher.empty') }}
            </div>
          </main>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>

  <WorkspaceCreateDialog
    v-model:is-open="isCreateDialogOpen"
    :items="workspaceItems"
    :source-workspace-id="activeWorkspaceId"
    :default-name="createDefaultName"
    :name-max-length="props.nameMaxLength"
    :validate-name="(name) => validateName(name, null)"
    :on-create="props.onCreate"
    :on-load-organizations="props.onLoadOrganizations"
    :on-create-organization="props.onCreateOrganization"
    @created="handleWorkspaceCreated"
    @busy="isCreating = $event"
  />

  <!-- 永久删除二次确认：统一走 UserConfirmDialog intent=danger（参考 BoardsPage）。Busy 提交时组件内部锁住 mask/Escape/close-control。输入工作区名匹配后才可确认，防误触。 -->
  <UserConfirmDialog
    :is-visible="isDeleteDialogVisible"
    :title="t('workspace.switcher.archiveTitle')"
    :message="t('workspace.switcher.archiveConfirm', { name: pendingDeleteName })"
    :confirm-label="t('workspace.switcher.archiveConfirmAction')"
    :expected-confirm-text="pendingDeleteName"
    :confirm-input-label="
      t('workspace.switcher.archiveConfirmInputLabel', { name: pendingDeleteName })
    "
    :confirm-input-placeholder="t('workspace.switcher.archiveConfirmInputPlaceholder')"
    intent="danger"
    :is-loading="isDeleting"
    @update:is-visible="handleDeleteConfirmVisibleUpdate"
    @confirm="confirmDelete"
  />
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, useTemplateRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { EditPencil, Plus, Xmark } from '@iconoir/vue'
import {
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from '@/components/ui/dialog'
import { dialogContentBaseClass } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import type {
  UserAccountMenuWorkspaceCreateInput,
  UserAccountMenuOrganizationItem,
  UserAccountMenuOrganizationPanel,
  UserAccountMenuWorkspaceItem,
} from './userAccountMenu.types'
import WorkspaceCreateDialog from './WorkspaceCreateDialog.vue'
import { provideWorkspaceSettingsReads } from '../../session/workspace-settings'
import { isDefaultWorkspaceItem } from './workspaceMenu'
import UserConfirmDialog from '../../../../core/components/UserConfirmDialog.vue'
import WorkspaceLimitsSection from './WorkspaceLimitsSection.vue'
import WorkspaceMemberSection from './WorkspaceMemberSection.vue'
import {
  accountCloseButtonClass,
  accountContentPaneClass,
  accountDialogContentClass,
  accountMainContentClass,
  accountNavTabClass,
} from '../shared/ui'

const props = withDefaults(
  defineProps<{
    isOpen: boolean
    items: UserAccountMenuWorkspaceItem[]
    selectedId?: string | null | undefined
    isLoading?: boolean | undefined
    loadError?: string | null | undefined
    nameMaxLength?: number | undefined
    currentUserId?: string | null | undefined
    onLoadOrganizations?: (() => Promise<UserAccountMenuOrganizationItem[]>) | undefined
    onCreateOrganization?:
      | ((name: string, idempotencyKey: string) => Promise<UserAccountMenuOrganizationItem>)
      | undefined
    // Opaque organization panel owned by the workspace shell.
    organizationPanel?: UserAccountMenuOrganizationPanel | null | undefined
    onRetry: () => void
    onCreate: (input: UserAccountMenuWorkspaceCreateInput) => Promise<UserAccountMenuWorkspaceItem>
    onRename: (workspaceId: string, name: string) => Promise<UserAccountMenuWorkspaceItem>
    onDelete: (workspaceId: string) => Promise<void>
  }>(),
  {
    selectedId: null,
    isLoading: false,
    loadError: null,
    nameMaxLength: 120,
    currentUserId: null,
    onLoadOrganizations: undefined,
    onCreateOrganization: undefined,
    organizationPanel: null,
  },
)

const emits = defineEmits<{
  (e: 'update:isOpen', value: boolean): void
}>()

const { t } = useI18n()
const toast = useAppToast()

const workspaceItems = computed(() => props.items)

// Left nav groups: Personal rows carry a null organizationId.
const personalWorkspaces = computed(() =>
  workspaceItems.value.filter((item) => item.organizationId == null),
)
const organizationWorkspaces = computed(() =>
  workspaceItems.value.filter((item) => item.organizationId != null),
)

// Settings selection stays independent from the workspace selection.
const isOrganizationSettingsSelected = ref(false)
onBeforeUnmount(() => props.organizationPanel?.setEnabled(false))

function isWorkspaceActive(item: UserAccountMenuWorkspaceItem): boolean {
  return !isOrganizationSettingsSelected.value && item.publicId === activeWorkspaceId.value
}

const activeWorkspaceId = ref<string>(props.selectedId ?? workspaceItems.value[0]?.publicId ?? '')

watch(
  [workspaceItems, () => props.selectedId],
  ([items, selectedId]) => {
    if (items.length === 0) {
      activeWorkspaceId.value = ''
      return
    }
    if (items.some((i) => i.publicId === activeWorkspaceId.value)) {
      return
    }
    if (selectedId && items.some((i) => i.publicId === selectedId)) {
      activeWorkspaceId.value = selectedId
    } else {
      activeWorkspaceId.value = items[0]?.publicId ?? ''
    }
  },
  { immediate: true },
)

const activeWorkspace = computed(() => {
  return workspaceItems.value.find((item) => item.publicId === activeWorkspaceId.value) ?? null
})

interface WorkspaceCapabilities {
  canRename: boolean
  canDelete: boolean
}

const activeCapabilities = computed<WorkspaceCapabilities>(() => {
  const item = activeWorkspace.value
  if (!item) {
    return {
      canRename: false,
      canDelete: false,
    }
  }
  const isDefault = isDefaultWorkspaceItem(item)
  return {
    canRename: !isDefault,
    canDelete: !isDefault,
  }
})

const billingAccount = provideWorkspaceSettingsReads({
  workspaceId: activeWorkspaceId,
  identity: () => props.currentUserId,
  isEnabled: () =>
    props.isOpen && !isOrganizationSettingsSelected.value && activeWorkspaceId.value !== '',
})

watch(
  [() => props.isOpen, () => props.organizationPanel],
  ([isOpen, panel]) => panel?.setEnabled(isOpen),
  { immediate: true },
)

function dismissOrganizationPanel(): void {
  isOrganizationSettingsSelected.value = false
  emits('update:isOpen', false)
}

function workspaceItemName(item: UserAccountMenuWorkspaceItem): string {
  return isDefaultWorkspaceItem(item) ? t('workspace.switcher.defaultBadge') : item.name
}

function selectWorkspace(item: UserAccountMenuWorkspaceItem) {
  isOrganizationSettingsSelected.value = false
  if (editingId.value && editingId.value !== item.publicId) {
    cancelRename()
  }
  if (
    pendingDeleteItem.value &&
    pendingDeleteItem.value.publicId !== item.publicId &&
    !isDeleting.value
  ) {
    closeDeleteDialog()
  }
  activeWorkspaceId.value = item.publicId
}

function selectOrganizationSettings() {
  if (editingId.value) {
    cancelRename()
  }
  if (pendingDeleteItem.value && !isDeleting.value) {
    closeDeleteDialog()
  }
  isOrganizationSettingsSelected.value = true
}

// Maxlength sits one char past the ceiling so over-limit input stays typable.
const nameFieldMaxLength = computed(() => props.nameMaxLength + 1)

const isCreating = ref(false)

const isCreateDialogOpen = ref(false)
const createDefaultName = ref('')

const editingId = ref<string | null>(null)
const draftName = ref('')
const isRenaming = ref(false)
const renameError = ref<string | null>(null)
const renameInputRef = useTemplateRef('renameInputRef')

function generateUniqueWorkspaceName(): string {
  const baseName = t('workspace.switcher.defaultNewName')
  const existing = new Set<string>()
  for (const item of workspaceItems.value) {
    existing.add(item.name.trim().toLowerCase())
    if (isDefaultWorkspaceItem(item)) {
      existing.add(t('workspace.switcher.defaultBadge').trim().toLowerCase())
    }
  }
  if (!existing.has(baseName.toLowerCase())) {
    return baseName
  }
  let index = 2
  while (existing.has(`${baseName} ${index}`.toLowerCase())) {
    index++
  }
  return `${baseName} ${index}`
}

function validateName(raw: string, currentWorkspaceId?: string | null): string | null {
  const next = raw.trim()
  if (!next) return t('workspace.switcher.nameEmpty')
  if (next.length > props.nameMaxLength) {
    return t('workspace.switcher.nameTooLong', { max: props.nameMaxLength })
  }
  const hasDuplicate = workspaceItems.value.some((item) => {
    if (currentWorkspaceId && item.publicId === currentWorkspaceId) return false
    const isMatchingRaw = item.name.trim().toLowerCase() === next.toLowerCase()
    const isMatchingDefault =
      isDefaultWorkspaceItem(item) &&
      t('workspace.switcher.defaultBadge').trim().toLowerCase() === next.toLowerCase()
    return isMatchingRaw || isMatchingDefault
  })
  if (hasDuplicate) {
    return t('workspace.switcher.nameExists')
  }
  return null
}

function openCreateDialog(): void {
  if (isCreating.value) return
  createDefaultName.value = generateUniqueWorkspaceName()
  isCreateDialogOpen.value = true
}

function handleWorkspaceCreated(created: UserAccountMenuWorkspaceItem): void {
  isOrganizationSettingsSelected.value = false
  activeWorkspaceId.value = created.publicId
  pendingCreateFocusId.value = created.publicId
  focusNewWorkspaceRow(created.publicId)
}

function focusNewWorkspaceRow(workspaceId: string) {
  void nextTick(() => {
    const row = document.querySelector(
      `[data-workspace-nav="${workspaceId}"]`,
    ) as HTMLElement | null
    row?.focus()
  })
}

// The list refresh lands after the confirm commit resolves: retry the focus
// once the new row renders so keyboard users land on it with Members shown.
const pendingCreateFocusId = ref<string | null>(null)

watch(workspaceItems, (items) => {
  const pending = pendingCreateFocusId.value
  if (!pending) return
  if (items.some((item) => item.publicId === pending)) {
    pendingCreateFocusId.value = null
    focusNewWorkspaceRow(pending)
  }
})

// Reload the management surface after the create dialog closes.
watch(isCreateDialogOpen, (isNowOpen) => {
  if (!isNowOpen && props.isOpen) void props.organizationPanel?.refresh()
})

function focusRenameInput(host: unknown): void {
  const holder = host as { $el?: unknown } | null
  const target = (holder?.$el ?? holder) as HTMLElement | null
  if (target instanceof HTMLInputElement) {
    target.focus()
    target.select()
  }
}

function startRename(item: UserAccountMenuWorkspaceItem) {
  if (isDefaultWorkspaceItem(item)) return
  activeWorkspaceId.value = item.publicId
  editingId.value = item.publicId
  draftName.value = item.name
  renameError.value = null
  void nextTick(() => {
    focusRenameInput(renameInputRef.value)
  })
}

function cancelRename() {
  editingId.value = null
  renameError.value = null
}

async function commitRename(item: UserAccountMenuWorkspaceItem) {
  if (isDefaultWorkspaceItem(item)) {
    cancelRename()
    return
  }
  if (editingId.value !== item.publicId || isRenaming.value) return
  const next = draftName.value.trim()
  if (next === item.name) {
    cancelRename()
    return
  }
  const violation = validateName(draftName.value, item.publicId)
  if (violation) {
    renameError.value = violation
    return
  }
  isRenaming.value = true
  renameError.value = null
  try {
    await props.onRename(item.publicId, next)
    editingId.value = null
  } catch {
    draftName.value = item.name
    toast.error(t('workspace.switcher.renameFailed'))
  } finally {
    isRenaming.value = false
  }
}

const pendingDeleteItem = ref<UserAccountMenuWorkspaceItem | null>(null)
const isDeleting = ref(false)
const isDeleteDialogVisible = computed(() => pendingDeleteItem.value !== null)
const pendingDeleteName = computed(() => pendingDeleteItem.value?.name ?? '')

function requestDelete(item: UserAccountMenuWorkspaceItem) {
  if (isDefaultWorkspaceItem(item) || isDeleting.value) return
  activeWorkspaceId.value = item.publicId
  pendingDeleteItem.value = { ...item }
}

function closeDeleteDialog() {
  pendingDeleteItem.value = null
}

// UserConfirmDialog 在 Busy 提交时已锁住全部关闭路径；页面只处理 idle
// 关闭后的清理。
function handleDeleteConfirmVisibleUpdate(isVisible: boolean) {
  if (isVisible || isDeleting.value) return
  closeDeleteDialog()
}

async function confirmDelete() {
  const item = pendingDeleteItem.value
  if (!item || isDeleting.value) return
  if (isDefaultWorkspaceItem(item)) {
    closeDeleteDialog()
    return
  }
  isDeleting.value = true
  try {
    await props.onDelete(item.publicId)
    pendingDeleteItem.value = null
  } catch (error) {
    const code = userErrorCode(error)?.toLowerCase()
    toast.error(
      code != null
        ? `${t('workspace.switcher.archiveFailed')} (${code})`
        : t('workspace.switcher.archiveFailed'),
    )
  } finally {
    isDeleting.value = false
  }
}
</script>
