<template>
  <DialogShell
    :is-open="isCreateDialogOpen"
    :title="t('workspace.switcher.createDialogTitle')"
    :description="t('workspace.switcher.createDialogNameLabel')"
    :size="accountPreset.size"
    :is-busy="isCreating"
    :has-close="accountPreset.hasClose"
    :close-label="t('common.actions.close')"
    content-class="bg-popover w-[min(420px,calc(100vw-32px))]"
    @update:open="handleCreateOpenUpdate"
  >
    <div class="min-h-0 overflow-y-auto">
      <div class="p-6">
        <h3 class="m-0 text-admin-heading font-semibold text-foreground">
          {{ t('workspace.switcher.createDialogTitle') }}
        </h3>
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

        <!-- Scope selector is only shown when the user belongs to at least one organization -->
        <div v-if="hasOrganizations" class="mt-5 flex flex-col gap-2">
          <span class="text-admin-content font-medium text-foreground">
            {{ t('workspace.switcher.scopeLabel') }}
          </span>
          <div
            role="group"
            :aria-label="t('workspace.switcher.scopeLabel')"
            class="grid grid-cols-2 gap-1 rounded-control bg-muted p-1"
          >
            <button
              type="button"
              :aria-pressed="createScopeId === 'personal'"
              :disabled="isCreating"
              class="h-8 flex items-center justify-center rounded-control px-3 text-sm font-medium transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer select-none disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              :class="
                createScopeId === 'personal'
                  ? 'bg-background text-foreground font-semibold shadow-xs'
                  : 'text-muted-foreground hover:text-foreground'
              "
              data-scope="personal"
              @click="createScopeId = 'personal'"
            >
              {{ t('workspace.switcher.scopePersonalTitle') }}
            </button>
            <button
              type="button"
              :aria-pressed="createScopeId !== 'personal'"
              :disabled="isCreating"
              class="h-8 flex items-center justify-center rounded-control px-3 text-sm font-medium transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer select-none disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              :class="
                createScopeId !== 'personal'
                  ? 'bg-background text-foreground font-semibold shadow-xs'
                  : 'text-muted-foreground hover:text-foreground'
              "
              data-scope="organization"
              @click="selectOrganizationScope"
            >
              <span class="truncate">{{ organizationScopeName }}</span>
            </button>
          </div>
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
            :is-disabled="isCreating"
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
  </DialogShell>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useTemplateRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import DialogShell from '../../../../core/components/DialogShell.vue'
import { dialogShellPresets } from '../../../../core/components/dialogShell'
import { useAppToast } from '../../../../core/composables/useAppToast'
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
const accountPreset = dialogShellPresets.account
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
  hasOrganizations,
  canSubmitCreate,
  prepare,
  fundingInput,
} = useWorkspaceCreation({
  workspaces: () => props.items,
  activeWorkspaceId: () => props.sourceWorkspaceId,
  isCreating,
  nameMaxLength: () => props.nameMaxLength,
  onLoadOrganizations: props.onLoadOrganizations,
})
const organizationScopeName = computed(() => {
  if (createOrganizationId.value) {
    const org = createScopeOptions.value.find(
      (item) => item.publicId === createOrganizationId.value,
    )
    if (org?.name) return org.name
  }
  return createScopeOptions.value[0]?.name ?? t('workspace.switcher.scopeOrganizationTitle')
})

function selectOrganizationScope(): void {
  const target = createScopeOptions.value[0]
  if (target) {
    createScopeId.value = target.publicId
  }
}
watch(
  isCreating,
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
  if (isCreating.value) return
  emits('update:isOpen', false)
}
function handleCreateOpenUpdate(isOpen: boolean): void {
  if (!isOpen) emits('update:isOpen', false)
}
</script>
