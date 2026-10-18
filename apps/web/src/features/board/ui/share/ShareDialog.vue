<template>
  <DialogRoot :open="panel.isOpen.value" @update:open="handleOpenUpdate">
    <DialogPortal>
      <DialogOverlay :class="ibDialogMaskClass" @click="handleMaskClick" />
      <DialogContent
        :class="cn(dialogContentBaseClass, shareDialogContentClass)"
        :show-close="true"
        :close-label="t('board.shareDialog.close')"
        :aria-busy="isDialogBusy"
        @escape-key-down="handleEscape"
        @pointer-down-outside="blockOutsideDismiss"
      >
        <div class="min-h-0 overflow-y-auto">
          <DialogDescription class="sr-only">
            {{ t('board.shareDialog.title') }}
          </DialogDescription>
          <div :class="shareDialogClasses.shell">
            <div :class="shareDialogClasses.header">
              <DialogTitle as="h2" :class="shareDialogClasses.title">
                {{ dialogTitle }}
              </DialogTitle>
            </div>

            <div
              v-if="dialog.isOpening.value"
              :class="shareDialogClasses.loading"
              aria-live="polite"
            >
              <Spinner :class="shareDialogClasses.spinner" />
            </div>

            <div
              v-else-if="dialog.openError.value"
              :class="shareDialogClasses.errorBox"
              aria-live="polite"
            >
              <p>{{ t('board.shareDialog.membersUnavailable') }}</p>
              <Button type="button" size="sm" @click="dialog.retryOpen()">
                {{ t('board.shareDialog.retry') }}
              </Button>
            </div>

            <template v-else-if="dialog.facts.value">
              <template v-if="dialog.canManage.value">
                <section
                  :class="shareDialogClasses.section"
                  :aria-label="t('board.shareDialog.workspace')"
                >
                  <h3 :class="shareDialogClasses.sectionTitle">
                    {{ t('board.shareDialog.workspace') }}
                  </h3>
                  <div
                    :class="shareDialogClasses.accessButtonGroup"
                    role="group"
                    :aria-label="t('board.shareDialog.workspaceLabel')"
                  >
                    <Button
                      type="button"
                      size="sm"
                      :variant="isAccessOn ? 'default' : 'outline'"
                      :is-disabled="dialog.isBusy.value"
                      :is-loading="dialog.isBusy.value"
                      :aria-pressed="isAccessOn ? 'true' : 'false'"
                      class="flex-1 active:scale-[0.98] transition-transform"
                      @click="onToggleAccessSwitch"
                    >
                      {{
                        isAccessOn
                          ? t('board.shareDialog.workspaceOff')
                          : t('board.shareDialog.workspaceOn')
                      }}
                    </Button>

                    <Button
                      type="button"
                      size="sm"
                      :variant="dialog.workspaceAccess.value === 'viewer' ? 'default' : 'outline'"
                      :is-disabled="dialog.isBusy.value || !isAccessOn"
                      :is-loading="dialog.isBusy.value"
                      :aria-pressed="dialog.workspaceAccess.value === 'viewer' ? 'true' : 'false'"
                      class="flex-1 active:scale-[0.98] transition-transform"
                      @click="onSelectWorkspaceAccess('viewer')"
                    >
                      {{ t('board.shareDialog.roleViewer') }}
                    </Button>

                    <Button
                      type="button"
                      size="sm"
                      :variant="dialog.workspaceAccess.value === 'editor' ? 'default' : 'outline'"
                      :is-disabled="dialog.isBusy.value || !isAccessOn"
                      :is-loading="dialog.isBusy.value"
                      :aria-pressed="dialog.workspaceAccess.value === 'editor' ? 'true' : 'false'"
                      class="flex-1 active:scale-[0.98] transition-transform"
                      @click="onSelectWorkspaceAccess('editor')"
                    >
                      {{ t('board.shareDialog.roleEditor') }}
                    </Button>
                  </div>
                </section>

                <div
                  v-if="dialog.hasMembersError.value"
                  :class="shareDialogClasses.errorBox"
                  aria-live="polite"
                >
                  <p>{{ t('board.shareDialog.membersUnavailable') }}</p>
                  <Button type="button" size="sm" @click="dialog.retryQueries()">
                    {{ t('board.shareDialog.retry') }}
                  </Button>
                </div>

                <template v-else>
                  <section
                    :class="shareDialogClasses.section"
                    :aria-label="t('board.shareDialog.members')"
                  >
                    <h3 :class="shareDialogClasses.sectionTitle">
                      {{ t('board.shareDialog.members') }}
                    </h3>

                    <div
                      :class="shareDialogClasses.avatarRow"
                      role="group"
                      :aria-label="t('board.shareDialog.members')"
                    >
                      <template v-for="row in dialog.memberRows.value" :key="row.userId">
                        <button
                          type="button"
                          :class="[
                            shareDialogClasses.avatarButton,
                            row.isCreator && 'cursor-default',
                          ]"
                          :aria-pressed="row.isCreator || row.isSelected ? 'true' : 'false'"
                          :aria-label="memberAriaLabel(row)"
                          :title="memberTitle(row)"
                          :disabled="row.isCreator || !isAccessOn || dialog.isBusy.value"
                          @mouseenter="hoveredUserId = row.userId"
                          @mouseleave="hoveredUserId = null"
                          @focus="hoveredUserId = row.userId"
                          @blur="hoveredUserId = null"
                          @click="dialog.toggleMember(row.userId)"
                        >
                          <div class="relative flex items-center justify-center">
                            <Avatar
                              :class="[
                                'size-10 transition-[transform,box-shadow,opacity,filter] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]',
                                row.isCreator || row.isSelected
                                  ? 'ring-2 ring-primary ring-offset-2 ring-offset-background'
                                  : 'border border-border opacity-40 grayscale hover:opacity-80 hover:grayscale-0',
                              ]"
                            >
                              <AvatarImage
                                v-if="resolveMemberAvatar(row, userStore.user)"
                                :src="resolveMemberAvatar(row, userStore.user)!"
                                :alt="memberDisplayName(row)"
                              />
                              <AvatarFallback class="text-xs font-semibold uppercase">
                                {{ memberInitial(row) }}
                              </AvatarFallback>
                            </Avatar>

                            <span
                              v-if="row.isCreator"
                              :class="shareDialogClasses.avatarCreatorBadge"
                              :title="t('board.shareDialog.creator')"
                              aria-hidden="true"
                            >
                              ★
                            </span>

                            <span
                              v-else-if="row.isSelected"
                              :class="shareDialogClasses.avatarGrantedBadge"
                              aria-hidden="true"
                            >
                              <Check class="size-2.5 stroke-[3]" />
                            </span>

                            <span
                              v-if="dialog.isBusy.value"
                              class="absolute inset-0 flex items-center justify-center rounded-pill bg-background/60"
                              aria-hidden="true"
                            >
                              <Spinner class="size-4 text-primary" />
                            </span>
                          </div>
                        </button>
                      </template>
                    </div>

                    <p :class="shareDialogClasses.emptyNote" aria-live="polite">
                      {{ activeMemberHint }}
                    </p>
                  </section>
                </template>
              </template>

              <template v-else>
                <div :class="shareDialogClasses.section">
                  <p :class="shareDialogClasses.statusLine">{{ ownPermissionCopy }}</p>
                  <p :class="shareDialogClasses.statusLine">{{ statusCopy }}</p>
                  <p :class="shareDialogClasses.statusLine">{{ generalCopy }}</p>
                  <p :class="shareDialogClasses.statusLine">
                    {{ t('board.shareDialog.readOnlyNote') }}
                  </p>
                </div>
              </template>
            </template>
          </div>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check } from '@iconoir/vue'
import { Button } from '@/components/ui/button'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import {
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from '@/components/ui/dialog'
import { dialogContentBaseClass } from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from '../../../../core/composables/useOverlayInteractionLifecycle'
import { ibDialogMaskClass } from '../../../../theme/dialogMask'
import type { ShareMemberRow } from '../../session/share/useShareRoster'
import { useShareDialog, type ShareDialogDeps } from '../../session/share/useShareDialog'
import { useUserStore } from '../../../auth'
import type { SharePanel } from '../../session/share/useSharePanel'
import { resolveMemberAvatar } from '../../../../core/media/avatarUrl'
import { shareDialogClasses, shareDialogContentClass } from './skin'

const props = defineProps<{
  panel: SharePanel
  ensureDraftSaved?: ((boardId: string) => Promise<boolean>) | undefined
  deps?: ShareDialogDeps | undefined
}>()

const { t } = useI18n()
const userStore = useUserStore()
const dialog = useShareDialog({
  panel: props.panel,
  ...(props.ensureDraftSaved ? { ensureDraftSaved: props.ensureDraftSaved } : {}),
  ...(props.deps ?? {}),
})

const {
  ariaBusy: isDialogBusy,
  canDismiss,
  completeClose,
  handleVisibilityChange,
  requestClose,
} = useOverlayInteractionLifecycle({
  isVisible: () => props.panel.isOpen.value,
  busy: () => dialog.isBusy.value,
  surface: defineDialogSurfaceIntent(),
  onVisibilityChange: (isVisible) => {
    if (!isVisible) props.panel.close()
  },
})

function closeDialogView() {
  if (handleVisibilityChange(false)) afterDialogHidden()
}

function handleOpenUpdate(isOpen: boolean) {
  if (!isOpen) closeDialogView()
}

function handleMaskClick() {
  if (requestClose('mask')) afterDialogHidden()
}

function handleEscape(event: KeyboardEvent) {
  if (!canDismiss('escape')) event.preventDefault()
}

function blockOutsideDismiss(event: Event) {
  event.preventDefault()
}

function afterDialogHidden() {
  completeClose()
  props.panel.handleHidden()
}

const dialogTitle = computed(() => t('board.shareDialog.title'))

const statusCopy = computed(() => {
  switch (dialog.accessState.value) {
    case 'workspace':
      return t('board.shareDialog.statusWorkspace')
    default:
      return t('board.shareDialog.statusPrivate')
  }
})

const generalCopy = computed(() => {
  switch (dialog.workspaceAccess.value) {
    case 'editor':
      return t('board.shareDialog.generalEditor')
    case 'viewer':
      return t('board.shareDialog.generalViewer')
    default:
      return t('board.shareDialog.generalNone')
  }
})

const ownPermissionCopy = computed(() => {
  const facts = dialog.facts.value
  if (facts?.isCreator) return t('board.shareDialog.ownCreator')
  if (facts?.capabilities.write) return t('board.shareDialog.ownEditor')
  return t('board.shareDialog.ownViewer')
})

const hoveredUserId = ref<string | null>(null)

function memberRoleLabel(): string {
  return dialog.workspaceAccess.value === 'editor'
    ? t('board.shareDialog.roleEditor')
    : t('board.shareDialog.roleViewer')
}

const activeMemberHint = computed(() => {
  if (hoveredUserId.value) {
    const target = dialog.memberRows.value.find((r) => r.userId === hoveredUserId.value)
    if (target) {
      const name = memberDisplayName(target)
      if (target.isCreator) {
        return `${name} · ${t('board.shareDialog.creator')}`
      }
      if (target.isSelected) {
        const roleLabel = memberRoleLabel()
        return `${name} · ${roleLabel}`
      }
      return `${name} · ${t('board.shareDialog.accessNone')}`
    }
  }
  return t('board.shareDialog.memberAvatarHint')
})

function memberDisplayName(row: ShareMemberRow): string {
  return row.email || row.userId
}

function memberInitial(row: ShareMemberRow): string {
  if (row.email) {
    return row.email.charAt(0).toUpperCase()
  }
  return row.userId.replace(/^usr_/, '').charAt(0).toUpperCase() || 'U'
}

function memberTitle(row: ShareMemberRow): string {
  const name = memberDisplayName(row)
  if (row.isCreator) return `${name} (${t('board.shareDialog.creator')})`
  if (row.isSelected) {
    const roleLabel = memberRoleLabel()
    return `${name} (${roleLabel})`
  }
  return `${name} · ${t('board.shareDialog.accessNone')}`
}

function memberAriaLabel(row: ShareMemberRow): string {
  const name = memberDisplayName(row)
  if (row.isCreator) return `${name}, ${t('board.shareDialog.creator')}`
  if (row.isSelected) {
    const roleLabel = memberRoleLabel()
    return `${name}, ${roleLabel}`
  }
  return `${name}, ${t('board.shareDialog.accessNone')}`
}

const isAccessOn = computed(() => dialog.workspaceAccess.value !== 'none')
async function onToggleAccessSwitch(): Promise<void> {
  await dialog.setWorkspaceAccess(isAccessOn.value ? 'none' : 'viewer')
}

async function onSelectWorkspaceAccess(value: 'viewer' | 'editor'): Promise<void> {
  await dialog.setWorkspaceAccess(value)
}
</script>
