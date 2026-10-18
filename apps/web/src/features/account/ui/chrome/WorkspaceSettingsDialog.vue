<template>
  <DialogRoot :open="props.isOpen" @update:open="emits('update:isOpen', $event)">
    <DialogPortal>
      <DialogOverlay :class="ibDialogMaskClass" />
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

            <!-- Workspace List Nav -->
            <nav
              class="flex-1 overflow-y-auto space-y-1"
              :aria-label="t('workspace.switcher.listLabel')"
            >
              <Button
                v-for="item in visibleItems"
                :key="item.publicId"
                type="button"
                variant="ghost"
                :class="accountNavTabClass(item.publicId === activeWorkspaceId)"
                :aria-current="item.publicId === activeWorkspaceId ? 'page' : undefined"
                :title="workspaceItemName(item)"
                @click="selectWorkspace(item)"
              >
                <span class="truncate">{{ workspaceItemName(item) }}</span>
              </Button>

              <p
                v-if="visibleItems.length === 0"
                class="px-2 py-4 text-xs text-muted-foreground"
                aria-live="polite"
              >
                {{ t('workspace.switcher.empty') }}
              </p>
            </nav>

            <!-- Add Workspace Button at Bottom of Sidebar -->
            <div class="mt-auto pt-4 flex flex-col gap-1.5">
              <Button
                type="button"
                variant="default"
                size="sm"
                class="w-full h-9 justify-center gap-1.5"
                :is-loading="isCreating"
                :is-disabled="isCreating"
                @click="submitCreate"
              >
                <Plus v-if="!isCreating" class="size-3.5" aria-hidden="true" />
                <span>{{
                  isCreating
                    ? t('workspace.switcher.creating')
                    : t('workspace.switcher.createAction')
                }}</span>
              </Button>
              <p
                v-if="createError"
                class="m-0 text-xs leading-normal text-destructive"
                role="alert"
              >
                {{ createError }}
              </p>
            </div>
          </aside>

          <!-- Right Content Pane (accountContentPaneClass: bg-muted/40) -->
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

            <!-- Error State -->
            <div
              v-else-if="props.loadError"
              class="flex flex-col items-start gap-3 rounded-control border border-destructive/30 bg-destructive/10 p-4 text-sm text-foreground"
              aria-live="polite"
            >
              <p class="m-0 text-sm leading-normal text-foreground">{{ props.loadError }}</p>
              <Button type="button" size="sm" @click="props.onRetry()">
                {{ t('common.actions.retry') }}
              </Button>
            </div>

            <!-- Active Workspace Detail -->
            <div v-else-if="activeWorkspace" class="flex flex-col gap-6">
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
                        type="button"
                        variant="ghost"
                        size="icon"
                        class="size-7 text-muted-foreground hover:text-foreground"
                        :is-disabled="isDefaultWorkspaceItem(activeWorkspace)"
                        :title="
                          isDefaultWorkspaceItem(activeWorkspace)
                            ? t('workspace.switcher.renameDefaultNote')
                            : t('workspace.switcher.renameAriaLabel', {
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
                    <p
                      v-if="isDefaultWorkspaceItem(activeWorkspace)"
                      class="m-0 mt-0.5 text-xs text-muted-foreground"
                    >
                      {{ t('workspace.switcher.renameDefaultNote') }}
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
              />

              <!-- Archive row: the Default workspace is never archived, so its
                button stays disabled with the protected note. -->
              <section
                class="flex flex-col gap-2 rounded-control border border-border p-3"
                :aria-label="t('workspace.switcher.archiveSectionLabel')"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="min-w-0">
                    <p class="m-0 text-sm font-medium text-foreground">
                      {{ t('workspace.switcher.archiveTitle') }}
                    </p>
                    <p class="m-0 mt-0.5 text-xs text-muted-foreground">
                      {{
                        isDefaultWorkspaceItem(activeWorkspace)
                          ? t('workspace.switcher.archiveDefaultNote')
                          : t('workspace.switcher.archiveHint')
                      }}
                    </p>
                  </div>
                  <Button
                    v-if="!confirmingArchiveId"
                    type="button"
                    variant="outline"
                    size="sm"
                    class="h-9 shrink-0"
                    :is-disabled="isDefaultWorkspaceItem(activeWorkspace) || isArchiving"
                    :title="
                      isDefaultWorkspaceItem(activeWorkspace)
                        ? t('workspace.switcher.archiveDefaultNote')
                        : undefined
                    "
                    :aria-label="t('workspace.switcher.archiveAction')"
                    @click="requestArchive(activeWorkspace)"
                  >
                    {{ t('workspace.switcher.archiveAction') }}
                  </Button>
                </div>
                <div
                  v-if="confirmingArchiveId === activeWorkspace.publicId"
                  class="flex flex-col gap-2"
                >
                  <p class="m-0 text-xs leading-normal text-foreground" role="alert">
                    {{ t('workspace.switcher.archiveConfirm', { name: activeWorkspace.name }) }}
                  </p>
                  <div class="flex items-center gap-2">
                    <Button
                      type="button"
                      size="sm"
                      class="h-9 shrink-0"
                      :is-loading="isArchiving"
                      :is-disabled="isArchiving"
                      @click="commitArchive(activeWorkspace)"
                    >
                      {{
                        isArchiving
                          ? t('workspace.switcher.archiving')
                          : t('workspace.switcher.archiveConfirmAction')
                      }}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      class="h-9 shrink-0"
                      :is-disabled="isArchiving"
                      @click="cancelArchive"
                    >
                      {{ t('common.actions.cancel') }}
                    </Button>
                  </div>
                </div>
                <p
                  v-if="archiveError"
                  class="m-0 text-xs leading-normal text-destructive"
                  role="alert"
                >
                  {{ archiveError }}
                </p>
              </section>
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
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useTemplateRef, watch } from 'vue'
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
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import { ibDialogMaskClass } from '../../../../theme/dialogMask'
import type { UserAccountMenuWorkspaceItem } from './userAccountMenu.types'
import { isDefaultWorkspaceItem, visibleWorkspaceItems } from './workspaceMenu'
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
    onRetry: () => void
    onCreate: (name: string) => Promise<UserAccountMenuWorkspaceItem>
    onRename: (workspaceId: string, name: string) => Promise<UserAccountMenuWorkspaceItem>
    onArchive: (workspaceId: string) => Promise<UserAccountMenuWorkspaceItem>
    onSelect?: ((workspaceId: string) => void) | undefined
  }>(),
  {
    selectedId: null,
    isLoading: false,
    loadError: null,
    nameMaxLength: 120,
    currentUserId: null,
    onSelect: undefined,
  },
)

const emits = defineEmits<{
  (e: 'update:isOpen', value: boolean): void
}>()

const { t } = useI18n()

const visibleItems = computed(() => visibleWorkspaceItems(props.items))

const activeWorkspaceId = ref<string>(props.selectedId ?? visibleItems.value[0]?.publicId ?? '')

watch(
  [visibleItems, () => props.selectedId],
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
  return visibleItems.value.find((item) => item.publicId === activeWorkspaceId.value) ?? null
})

function workspaceItemName(item: UserAccountMenuWorkspaceItem): string {
  return isDefaultWorkspaceItem(item) ? t('workspace.switcher.defaultBadge') : item.name
}

function selectWorkspace(item: UserAccountMenuWorkspaceItem) {
  if (editingId.value && editingId.value !== item.publicId) {
    cancelRename()
  }
  if (confirmingArchiveId.value && confirmingArchiveId.value !== item.publicId) {
    cancelArchive()
  }
  activeWorkspaceId.value = item.publicId
}

// Native maxlength sits one char past the contract ceiling so over-limit
// input stays typable and surfaces the inline nameTooLong copy via validateName.
const nameFieldMaxLength = computed(() => props.nameMaxLength + 1)

const isCreating = ref(false)
const createError = ref<string | null>(null)

const editingId = ref<string | null>(null)
const draftName = ref('')
const isRenaming = ref(false)
const renameError = ref<string | null>(null)
const renameInputRef = useTemplateRef('renameInputRef')

function generateUniqueWorkspaceName(): string {
  const baseName = t('workspace.switcher.defaultNewName')
  const existing = new Set<string>()
  for (const item of visibleItems.value) {
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
  const hasDuplicate = visibleItems.value.some((item) => {
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

async function submitCreate() {
  if (isCreating.value) return
  const name = generateUniqueWorkspaceName()
  isCreating.value = true
  createError.value = null
  try {
    const created = await props.onCreate(name)
    activeWorkspaceId.value = created.publicId
  } catch (error) {
    const code = userErrorCode(error)?.toLowerCase()
    createError.value =
      code != null
        ? `${t('workspace.switcher.createFailed')} (${code})`
        : t('workspace.switcher.createFailed')
  } finally {
    isCreating.value = false
  }
}

function startRename(item: UserAccountMenuWorkspaceItem) {
  if (isDefaultWorkspaceItem(item)) return
  activeWorkspaceId.value = item.publicId
  editingId.value = item.publicId
  draftName.value = item.name
  renameError.value = null
  void nextTick(() => {
    const host = renameInputRef.value as unknown as { $el?: unknown } | null
    const input = (host?.$el ?? host) as HTMLElement | null
    if (input instanceof HTMLInputElement) {
      input.focus()
      input.select()
    }
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
    renameError.value = t('workspace.switcher.renameFailed')
  } finally {
    isRenaming.value = false
  }
}

const confirmingArchiveId = ref<string | null>(null)
const isArchiving = ref(false)
const archiveError = ref<string | null>(null)

function requestArchive(item: UserAccountMenuWorkspaceItem) {
  if (isDefaultWorkspaceItem(item) || isArchiving.value) return
  activeWorkspaceId.value = item.publicId
  confirmingArchiveId.value = item.publicId
  archiveError.value = null
}

function cancelArchive() {
  confirmingArchiveId.value = null
  archiveError.value = null
}

async function commitArchive(item: UserAccountMenuWorkspaceItem) {
  if (isDefaultWorkspaceItem(item)) {
    cancelArchive()
    return
  }
  if (confirmingArchiveId.value !== item.publicId || isArchiving.value) return
  isArchiving.value = true
  archiveError.value = null
  try {
    await props.onArchive(item.publicId)
    confirmingArchiveId.value = null
  } catch (error) {
    const code = userErrorCode(error)?.toLowerCase()
    archiveError.value =
      code != null
        ? `${t('workspace.switcher.archiveFailed')} (${code})`
        : t('workspace.switcher.archiveFailed')
  } finally {
    isArchiving.value = false
  }
}
</script>
