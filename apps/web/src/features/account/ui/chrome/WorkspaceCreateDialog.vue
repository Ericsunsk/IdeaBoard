<template>
  <DialogRoot :open="isCreateDialogOpen" @update:open="handleCreateOpenUpdate">
    <DialogPortal>
      <DialogOverlay @click="handleCreateMaskClick" />
      <DialogContent
        class="bg-popover"
        :style="{ width: 'min(420px, calc(100vw - 32px))' }"
        :aria-label="t('workspace.switcher.createDialogTitle')"
        :aria-busy="isCreating || isCreatingOrganization || undefined"
        @escape-key-down="handleCreateEscape"
        @pointer-down-outside="blockCreateOutsideDismiss"
      >
        <div class="min-h-0 overflow-y-auto">
          <DialogDescription class="sr-only">
            {{ t('workspace.switcher.createDialogNameLabel') }}
          </DialogDescription>
          <div class="p-6">
            <DialogTitle as="h3" class="m-0 text-admin-heading font-semibold text-foreground">
              {{ t('workspace.switcher.createDialogTitle') }}
            </DialogTitle>
            <div class="mt-5 mb-0 flex flex-col gap-2">
              <label :for="createInputId" class="text-admin-content font-medium text-foreground">
                {{ t('workspace.switcher.createDialogNameLabel') }}
              </label>
              <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The create input carries its accessible name via aria-label -->
              <Input
                :id="createInputId"
                ref="createInputRef"
                v-model="createName"
                size="sm"
                class="h-9 text-admin-content bg-background"
                :invalid="createNameError !== null"
                :disabled="isCreating"
                :aria-label="t('workspace.switcher.createDialogNameLabel')"
                :maxlength="nameFieldMaxLength"
                @keydown.enter="submitCreateFromDialog"
                @keydown.esc.stop.prevent="cancelCreateDialog"
                @keydown.escape.stop.prevent="cancelCreateDialog"
              />
              <p
                v-if="createNameError"
                class="m-0 text-admin-meta leading-normal text-destructive"
                role="alert"
              >
                {{ createNameError }}
              </p>
            </div>
            <div v-if="props.onLoadOrganizations" class="mt-5 flex flex-col gap-2">
              <label
                for="workspace-create-scope"
                class="text-admin-content font-medium text-foreground"
                >{{ t('workspace.switcher.organization.scope') }}</label
              >
              <select
                id="workspace-create-scope"
                v-model="createScopeId"
                class="h-9 rounded-control border border-input bg-background px-3 text-admin-content text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                :disabled="isCreating || isCreatingOrganization"
              >
                <option value="personal">
                  {{ t('workspace.switcher.organization.personal') }}
                </option>
                <option
                  v-for="organization in createScopeOptions"
                  :key="organization.publicId"
                  :value="organization.publicId"
                >
                  {{ organization.name }}
                </option>
              </select>
              <p
                v-if="isOrganizationsLoading"
                class="m-0 text-admin-meta text-muted-foreground"
                role="status"
              >
                {{ t('common.actions.loading') }}
              </p>
              <div v-if="organizationLoadError" class="flex flex-col gap-2">
                <p class="m-0 text-admin-meta text-destructive" role="alert">
                  {{ organizationLoadError }}
                </p>
                <Button type="button" size="sm" variant="outline" @click="loadOrganizations()">{{
                  t('common.actions.retry')
                }}</Button>
              </div>
              <Button
                v-if="props.onCreateOrganization && !isOrganizationFormOpen"
                type="button"
                size="sm"
                variant="outline"
                class="self-start"
                :is-disabled="isCreating"
                @click="isOrganizationFormOpen = true"
                >{{ t('workspace.switcher.organization.new') }}</Button
              >
              <div
                v-if="isOrganizationFormOpen"
                class="flex flex-col gap-2 rounded-control border border-border p-3"
              >
                <label
                  for="workspace-organization-name"
                  class="text-admin-meta font-medium text-foreground"
                  >{{ t('workspace.switcher.organization.name') }}</label
                >
                <Input
                  id="workspace-organization-name"
                  v-model="organizationName"
                  :aria-label="t('workspace.switcher.organization.name')"
                  :disabled="isCreatingOrganization"
                  :maxlength="nameFieldMaxLength"
                />
                <p class="m-0 text-admin-meta text-muted-foreground">
                  {{ t('workspace.switcher.organization.setupNote') }}
                </p>
                <p
                  v-if="organizationError"
                  class="m-0 text-admin-meta text-destructive"
                  role="alert"
                >
                  {{ organizationError }}
                </p>
                <div class="flex gap-2">
                  <Button
                    type="button"
                    size="sm"
                    :is-loading="isCreatingOrganization"
                    :is-disabled="isCreatingOrganization"
                    @click="createOrganization()"
                    >{{ t('workspace.switcher.organization.confirm') }}</Button
                  >
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    :is-disabled="isCreatingOrganization"
                    @click="isOrganizationFormOpen = false"
                    >{{ t('common.actions.cancel') }}</Button
                  >
                </div>
              </div>
            </div>
            <div class="mt-5 mb-0 flex flex-col gap-2">
              <span class="text-admin-content font-medium text-foreground">
                {{ t('workspace.switcher.scopeLabel') }}
              </span>
              <div class="rounded-control border border-border p-3">
                <span class="block text-admin-content font-medium text-foreground">
                  {{
                    createOrganizationId
                      ? t('workspace.switcher.scopeOrganizationTitle')
                      : t('workspace.switcher.scopePersonalTitle')
                  }}
                </span>
                <span class="mt-0.5 block text-admin-meta text-muted-foreground">
                  {{ createScopeHint }}
                </span>
              </div>
            </div>
            <div class="mt-6 flex justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                :is-disabled="isCreating || isCreatingOrganization"
                class="rounded-pill px-4 py-2 text-admin-content font-semibold text-muted-foreground hover:bg-muted hover:text-foreground"
                @click="cancelCreateDialog"
              >
                {{ t('common.actions.cancel') }}
              </Button>
              <Button
                type="button"
                :is-loading="isCreating"
                :is-disabled="!canSubmitCreate"
                class="rounded-pill px-4 py-2 text-admin-content font-semibold disabled:opacity-50 bg-primary text-primary-foreground hover:bg-primary-hover"
                @click="submitCreateFromDialog"
              >
                {{
                  isCreating
                    ? t('workspace.switcher.creating')
                    : t('workspace.switcher.createDialogConfirm')
                }}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useTemplateRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAppToast } from '../../../../core/composables/useAppToast'
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from '../../../../core/composables/useOverlayInteractionLifecycle'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import { useWorkspaceCreation } from '../../session/useWorkspaceCreation'
import type {
  UserAccountMenuOrganizationItem,
  UserAccountMenuWorkspaceCreateInput,
  UserAccountMenuWorkspaceItem,
} from './userAccountMenu.types'

const props = withDefaults(
  defineProps<{
    isOpen: boolean
    items: UserAccountMenuWorkspaceItem[]
    sourceWorkspaceId: string
    defaultName: string
    nameMaxLength: number
    validateName: (name: string) => string | null
    onCreate: (input: UserAccountMenuWorkspaceCreateInput) => Promise<UserAccountMenuWorkspaceItem>
    onLoadOrganizations?: (() => Promise<UserAccountMenuOrganizationItem[]>) | undefined
    onCreateOrganization?:
      | ((name: string, idempotencyKey: string) => Promise<UserAccountMenuOrganizationItem>)
      | undefined
  }>(),
  { onLoadOrganizations: undefined, onCreateOrganization: undefined },
)
const emits = defineEmits<{
  'update:isOpen': [value: boolean]
  created: [workspace: UserAccountMenuWorkspaceItem]
  busy: [value: boolean]
}>()
const { t } = useI18n()
const toast = useAppToast()
const isCreateDialogOpen = computed(() => props.isOpen)
const isCreating = ref(false)
const createName = ref('')
const createNameError = ref<string | null>(null)
const createInputId = 'workspace-create-name-input'
const createInputRef = useTemplateRef('createInputRef')
const nameFieldMaxLength = computed(() => props.nameMaxLength + 1)
const {
  createScopeHint,
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
  loadOrganizations,
  createOrganization,
  prepare,
  fundingInput,
} = useWorkspaceCreation({
  workspaces: () => props.items,
  activeWorkspaceId: () => props.sourceWorkspaceId,
  isCreating,
  nameMaxLength: () => props.nameMaxLength,
  onLoadOrganizations: props.onLoadOrganizations,
  onCreateOrganization: props.onCreateOrganization,
})
const { canDismiss: canDismissCreate, requestClose: requestCreateClose } =
  useOverlayInteractionLifecycle({
    isVisible: isCreateDialogOpen,
    busy: () => isCreating.value || isCreatingOrganization.value,
    surface: defineDialogSurfaceIntent(),
    onVisibilityChange: (isVisible) => {
      if (!isVisible) emits('update:isOpen', false)
    },
  })
watch(
  () => isCreating.value || isCreatingOrganization.value,
  (isBusy) => emits('busy', isBusy),
)
watch(isCreateDialogOpen, (isOpen) => {
  if (!isOpen) return
  createName.value = props.defaultName
  createNameError.value = null
  prepare()
  void nextTick(() => {
    const host = createInputRef.value as unknown as { $el?: unknown } | null
    const input = (host?.$el ?? host) as HTMLElement | null
    if (input instanceof HTMLInputElement) {
      input.focus()
      input.select()
    }
  })
})
async function submitCreateFromDialog(): Promise<void> {
  if (!canSubmitCreate.value) return
  const trimmed = createName.value.trim()
  if (!trimmed) {
    toast.error(t('workspace.switcher.nameEmpty'))
    return
  }
  const violation = props.validateName(createName.value)
  if (violation) {
    createNameError.value = violation
    return
  }
  const funding = fundingInput()
  isCreating.value = true
  createNameError.value = null
  try {
    const created = await props.onCreate({ name: trimmed, ...funding })
    emits('created', created)
    emits('update:isOpen', false)
  } catch (error) {
    const code = userErrorCode(error)?.toLowerCase()
    const message =
      code != null
        ? `${t('workspace.switcher.createFailed')} (${code})`
        : t('workspace.switcher.createFailed')
    createNameError.value = message
    toast.error(message)
  } finally {
    isCreating.value = false
  }
}
function cancelCreateDialog(): void {
  requestCreateClose('close-control')
}
function handleCreateOpenUpdate(isOpen: boolean): void {
  if (!isOpen) requestCreateClose('close-control')
}
function handleCreateMaskClick(): void {
  requestCreateClose('mask')
}
function handleCreateEscape(event: KeyboardEvent): void {
  if (!canDismissCreate('escape')) event.preventDefault()
}
function blockCreateOutsideDismiss(event: Event): void {
  event.preventDefault()
}
</script>
