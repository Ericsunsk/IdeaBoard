<template>
  <section
    class="flex flex-col gap-5 min-w-0"
    :aria-label="t('workspace.management.title', { name: displayName })"
  >
    <p v-if="isLoading" class="text-xs text-muted-foreground" role="status">
      {{ t('common.actions.loading') }}
    </p>
    <p v-else-if="loadError" class="text-xs text-destructive" role="alert">
      {{ loadError }}
      <Button type="button" variant="ghost" size="sm" @click="refresh">{{
        t('common.actions.retry')
      }}</Button>
    </p>
    <section
      v-else-if="!organization"
      class="flex flex-col gap-2"
      :aria-label="t('workspace.switcher.organization.new')"
    >
      <label for="organization-create-name" class="text-admin-content font-medium">{{
        t('workspace.switcher.organization.name')
      }}</label>
      <div class="flex items-center gap-2">
        <Input
          id="organization-create-name"
          v-model="creationName"
          class="h-9"
          :disabled="isCreatingOrganization"
          :invalid="organizationError !== null"
          :maxlength="nameFieldMaxLength"
          @keydown.enter="createOrganization"
        />
        <Button
          type="button"
          size="sm"
          :is-loading="isCreatingOrganization"
          :is-disabled="isCreatingOrganization"
          @click="createOrganization"
          >{{ t('workspace.switcher.organization.confirm') }}</Button
        >
      </div>
      <p v-if="organizationError" class="text-xs text-destructive" role="alert">
        {{ organizationError }}
      </p>
    </section>
    <template v-else>
      <header class="flex flex-col gap-2">
        <div v-if="isRenaming" class="flex flex-col gap-2">
          <div class="flex items-center gap-2">
            <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The rename input carries its accessible name via aria-label -->
            <Input
              ref="renameInputRef"
              v-model="draftName"
              size="sm"
              class="h-9 max-w-xs text-admin-content bg-background"
              :invalid="renameError !== null"
              :disabled="isRenamingBusy"
              :aria-label="
                t('workspace.switcher.renameAriaLabel', {
                  name: displayName,
                })
              "
              :maxlength="nameFieldMaxLength"
              @keydown.enter="commitRename"
              @keydown.esc.stop.prevent="cancelRename"
              @keydown.escape.stop.prevent="cancelRename"
            />
            <Button
              type="button"
              size="sm"
              class="h-9 shrink-0"
              :is-loading="isRenamingBusy"
              :is-disabled="isRenamingBusy"
              @click="commitRename"
            >
              {{ t('common.actions.save') }}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              class="h-9 shrink-0"
              :is-disabled="isRenamingBusy"
              @click="cancelRename"
            >
              {{ t('common.actions.cancel') }}
            </Button>
          </div>
          <p
            v-if="renameError"
            class="m-0 text-admin-meta leading-normal text-destructive"
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
                {{ displayName }}
              </h2>
              <Button
                v-if="canManage"
                type="button"
                variant="ghost"
                size="icon"
                class="size-7 text-muted-foreground hover:text-foreground"
                :title="
                  t('workspace.switcher.renameAriaLabel', {
                    name: displayName,
                  })
                "
                :aria-label="
                  t('workspace.switcher.renameAriaLabel', {
                    name: displayName,
                  })
                "
                @click="startRename"
              >
                <EditPencil class="size-3.5" aria-hidden="true" />
              </Button>
            </div>
            <p class="m-0 mt-1 text-admin-meta text-muted-foreground font-mono">
              ID: {{ organization.publicId }}
            </p>
          </div>
        </div>
      </header>
      <!-- Closed status badge if permanently closed -->
      <div
        v-if="isClosed"
        class="flex items-center gap-2 rounded-control border border-destructive/20 bg-destructive/10 px-3 py-2 text-xs font-medium text-destructive"
        role="status"
      >
        {{ t('workspace.management.statusClosed') }}
      </div>

      <!-- Invite Member Card (when canManage & !isClosed) -->
      <section
        v-if="canManage"
        class="rounded-panel border border-border bg-card p-4 shadow-raised sm:p-5"
        :aria-label="t('workspace.management.inviteTitle')"
      >
        <div class="flex flex-col gap-3">
          <span class="text-admin-body font-semibold text-muted-foreground">
            {{ t('workspace.management.inviteTitle') }}
          </span>
          <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
            <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- Accessible name via aria-label. -->
            <Input
              v-model="inviteEmail"
              size="sm"
              class="h-9 min-w-0 flex-1 text-xs bg-background"
              :disabled="isSaving"
              :aria-label="t('workspace.switcher.members.emailLabel')"
              :placeholder="t('workspace.switcher.members.emailPlaceholder')"
              inputmode="email"
              autocomplete="email"
              @keydown.enter="invite"
            />
            <Button
              type="button"
              size="sm"
              class="h-9 shrink-0 px-4"
              :is-loading="isSaving"
              :is-disabled="isSaving || !inviteEmail.trim()"
              @click="invite"
            >
              <SendMail class="size-4" aria-hidden="true" />
              {{
                isSaving
                  ? t('workspace.switcher.members.inviting')
                  : t('workspace.switcher.members.inviteAction')
              }}
            </Button>
          </div>
          <p v-if="inviteError" class="m-0 text-xs leading-normal text-destructive" role="alert">
            {{ inviteError }}
          </p>
        </div>
      </section>

      <!-- Members Roster Card -->
      <section
        class="rounded-panel border border-border bg-card p-4 shadow-raised sm:p-5"
        aria-labelledby="org-member-roster-title"
      >
        <div class="mb-4 flex items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <h3
              id="org-member-roster-title"
              class="m-0 text-admin-body font-semibold text-foreground"
            >
              {{ t('workspace.switcher.members.title') }}
            </h3>
            <span
              class="rounded-pill border border-border bg-muted/50 px-2 py-0.5 text-admin-caption font-medium text-muted-foreground"
            >
              {{ members.length }}
            </span>
          </div>
          <Button
            v-if="canLeave"
            type="button"
            variant="outline"
            size="sm"
            class="h-8 shrink-0 gap-1.5 text-xs text-muted-foreground hover:text-destructive hover:border-destructive/30"
            :is-disabled="isLeaving"
            @click="isLeaveArmed = true"
          >
            <LogOut class="size-3.5" aria-hidden="true" />
            {{
              isLeaving ? t('workspace.management.leaving') : t('workspace.management.leaveAction')
            }}
          </Button>
        </div>

        <div class="flex flex-wrap items-center gap-4 py-2">
          <DropdownMenu v-for="member in members" :key="member.userId">
            <DropdownMenuTrigger as-child :disabled="!hasActions(member)">
              <button
                type="button"
                class="group relative inline-flex items-center justify-center rounded-pill cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default"
                :aria-label="memberDisplayName(member)"
                :title="memberDisplayName(member)"
              >
                <Avatar
                  :class="[
                    'size-10 shrink-0 transition-[transform,box-shadow] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] group-hover:scale-105 cursor-default shadow-sm',
                    member.role === 'owner' || member.role === 'admin'
                      ? 'ring-2 ring-primary ring-offset-2 ring-offset-background'
                      : 'ring-1 ring-border/70 group-hover:ring-primary/60',
                  ]"
                >
                  <AvatarImage
                    v-if="resolveMemberAvatar(member, userStore.user)"
                    :src="resolveMemberAvatar(member, userStore.user)!"
                    :alt="memberDisplayName(member)"
                  />
                  <AvatarFallback
                    class="bg-muted text-foreground font-semibold text-xs select-none uppercase"
                  >
                    {{ resolveMemberInitial(member) }}
                  </AvatarFallback>
                </Avatar>

                <!-- Owner badge (matching boards share dialog) -->
                <span
                  v-if="member.role === 'owner'"
                  class="pointer-events-none absolute -bottom-1 -right-1 flex size-4 items-center justify-center rounded-pill bg-primary text-xs font-bold text-primary-foreground shadow-xs ring-2 ring-background"
                  :title="t('workspace.switcher.members.roleOwner')"
                  aria-hidden="true"
                >
                  ★
                </span>

                <!-- Admin badge -->
                <span
                  v-else-if="member.role === 'admin'"
                  class="pointer-events-none absolute -bottom-1 -right-1 flex size-4 items-center justify-center rounded-pill bg-primary text-primary-foreground shadow-xs ring-2 ring-background"
                  :title="t('workspace.switcher.members.roleAdmin')"
                  aria-hidden="true"
                >
                  <Shield class="size-2.5 stroke-[2.5]" />
                </span>

                <!-- Floating Tooltip showing email on hover -->
                <div
                  class="pointer-events-none absolute bottom-full left-1/2 -translate-x-1/2 mb-2 z-admin-tooltip whitespace-nowrap rounded-control bg-popover px-2.5 py-1 text-admin-caption font-medium text-popover-foreground shadow-floating border border-border opacity-0 transition-opacity duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] group-hover:opacity-100 group-data-[state=open]:hidden"
                  role="tooltip"
                >
                  {{ memberDisplayName(member) }}
                </div>
              </button>
            </DropdownMenuTrigger>

            <DropdownMenuContent
              v-if="hasActions(member)"
              align="center"
              side="bottom"
              class="min-w-[124px] p-1 shadow-floating bg-popover rounded-control border border-border"
            >
              <!-- Make Admin (when member is regular member, and actor is owner) -->
              <DropdownMenuItem
                v-if="canPromoteToAdmin(member)"
                class="cursor-pointer text-xs py-1.5 px-2.5"
                @select="changeRole(member, 'admin')"
              >
                {{ t('workspace.switcher.members.setAdmin') }}
              </DropdownMenuItem>

              <!-- Demote to Member (when member is admin, and actor is owner) -->
              <DropdownMenuItem
                v-if="canDemoteToMember(member)"
                class="cursor-pointer text-xs py-1.5 px-2.5"
                @select="changeRole(member, 'member')"
              >
                {{ t('workspace.switcher.members.setMember') }}
              </DropdownMenuItem>

              <!-- Remove from organization -->
              <DropdownMenuItem
                v-if="canRemoveMember(member)"
                class="cursor-pointer text-xs py-1.5 px-2.5 text-destructive focus:text-destructive focus:bg-destructive/10"
                @select="memberToRemove = member"
              >
                {{ t('workspace.switcher.members.removeAction') }}
              </DropdownMenuItem>

              <!-- Leave organization (self) -->
              <DropdownMenuItem
                v-if="canSelfLeave(member)"
                class="cursor-pointer text-xs py-1.5 px-2.5 text-destructive focus:text-destructive focus:bg-destructive/10"
                @select="isLeaveArmed = true"
              >
                {{ t('workspace.management.leaveAction') }}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        <p v-if="!canManage" class="mt-3 m-0 text-xs text-muted-foreground">
          {{ t('workspace.switcher.members.readOnlyNote') }}
        </p>

        <!-- Pending Invitations (when canManage && pendingInvites > 0) -->
        <div
          v-if="canManage && pendingInvites.length > 0"
          class="mt-4 pt-3 border-t border-border flex flex-col gap-2"
          data-testid="org-pending-invites"
        >
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-muted-foreground">
              {{ t('workspace.switcher.members.pendingTitle') }}
            </span>
            <span class="text-admin-caption text-muted-foreground tabular-nums">
              {{ pendingInvites.length }}
            </span>
          </div>
          <ul class="m-0 flex flex-col gap-1.5 list-none p-0">
            <li
              v-for="inv in pendingInvites"
              :key="inv.id"
              class="flex items-center justify-between gap-2 rounded-control p-1.5 text-xs hover:bg-muted/40 transition-colors"
            >
              <div class="flex items-center gap-2 min-w-0 flex-1">
                <span class="truncate text-foreground font-medium" :title="inv.inviteeEmail">
                  {{ inv.inviteeEmail }}
                </span>
                <span
                  class="rounded-pill bg-muted px-1.5 py-0.5 text-admin-caption font-medium text-muted-foreground"
                >
                  {{ inv.role }}
                </span>
              </div>
              <Button
                v-if="canManage"
                type="button"
                variant="ghost"
                size="sm"
                class="h-6 px-2 text-admin-caption text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                :is-disabled="isRevokingInvite"
                @click="revokeInvite(inv.id)"
              >
                {{ t('workspace.switcher.members.revokeAction') }}
              </Button>
            </li>
          </ul>
        </div>
      </section>

      <!-- Danger Zone: Close Organization (owner only) -->
      <section
        v-if="canClose"
        class="flex flex-col gap-2 rounded-control border border-border p-3"
        :aria-label="t('workspace.management.closeTitle')"
      >
        <div class="flex items-center justify-between gap-2">
          <div class="min-w-0">
            <p class="m-0 text-sm font-medium text-foreground">
              {{ t('workspace.management.closeTitle') }}
            </p>
            <p class="m-0 mt-0.5 text-xs text-muted-foreground">
              {{ t('workspace.management.closeHint') }}
            </p>
          </div>
          <Button
            type="button"
            variant="destructive"
            size="sm"
            class="h-9 shrink-0"
            :is-disabled="isClosing"
            :aria-label="t('workspace.management.closeAction')"
            @click="isCloseArmed = true"
          >
            {{ t('workspace.management.closeAction') }}
          </Button>
        </div>
      </section>
    </template>
  </section>

  <UserConfirmDialog
    :is-visible="isCloseArmed"
    :title="t('workspace.management.closeTitle')"
    :message="t('workspace.management.closeHint')"
    :confirm-label="t('workspace.management.closeConfirmAction')"
    :expected-confirm-text="displayName"
    :confirm-input-label="t('workspace.management.closeConfirmInputLabel', { name: displayName })"
    :confirm-input-placeholder="t('workspace.management.closeConfirmInputPlaceholder')"
    :is-loading="isClosing"
    intent="danger"
    @update:is-visible="isCloseArmed = $event"
    @confirm="close"
  />

  <UserConfirmDialog
    :is-visible="isLeaveArmed"
    :title="t('workspace.management.leaveTitle')"
    :message="t('workspace.management.leaveConfirm', { name: displayName })"
    :confirm-label="t('workspace.management.leaveConfirmAction')"
    intent="danger"
    :is-loading="isLeaving"
    @update:is-visible="isLeaveArmed = $event"
    @confirm="leave()"
  />

  <UserConfirmDialog
    :is-visible="memberToRemove !== null"
    :title="t('workspace.switcher.members.removeAction')"
    :message="
      memberToRemove
        ? t('workspace.switcher.members.removeConfirmNote', {
            name: memberDisplayName(memberToRemove),
          })
        : ''
    "
    :confirm-label="t('workspace.switcher.members.removeConfirmAction')"
    intent="danger"
    :is-loading="isSaving"
    @update:is-visible="!$event && (memberToRemove = null)"
    @confirm="confirmRemoveMember"
  />
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useTemplateRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { EditPencil, LogOut, SendMail, Shield } from '@iconoir/vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import UserConfirmDialog from '../../../../core/components/UserConfirmDialog.vue'
import { resolveMemberAvatar, resolveMemberInitial } from '../../../../core/media/avatarUrl'
import { useUserStore } from '../../../auth'
import {
  WORKSPACE_NAME_MAX_LENGTH,
  type OrganizationMemberSummary,
} from '../../selection/workspaceResource'
import type { OrganizationManagement } from '../session/useManagement'

const props = defineProps<{ management: OrganizationManagement }>()
const emits = defineEmits<{ (e: 'dismiss'): void }>()
const { t } = useI18n()
const userStore = useUserStore()
const {
  organization,
  members,
  pendingInvites,
  isLoading,
  loadError,
  isClosed,
  canManage,
  canLeave,
  canClose,
  inviteEmail,
  inviteError,
  isSaving,
  isRevokingInvite,
  isLeaving,
  isClosing,
  organizationName: creationName,
  organizationError,
  isCreatingOrganization,
  draftName,
  isRenaming,
  isRenamingBusy,
  renameError,
  refresh,
  createOrganization,
  startRename,
  cancelRename,
  commitRename,
  canSelfLeave,
  canPromoteToAdmin,
  canDemoteToMember,
  canRemoveMember,
  hasActions,
  changeRole,
  invite,
  revokeInvite,
} = props.management
const displayName = computed(() => organization.value?.name ?? '')
const nameFieldMaxLength = WORKSPACE_NAME_MAX_LENGTH + 1
const isLeaveArmed = ref(false)
const isCloseArmed = ref(false)
const memberToRemove = ref<OrganizationMemberSummary | null>(null)
const renameInputRef = useTemplateRef('renameInputRef')
watch(isRenaming, (isEditing) => {
  if (!isEditing) return
  void nextTick(() => {
    const holder = renameInputRef.value as { $el?: unknown } | null
    const input = holder?.$el
    if (input instanceof HTMLInputElement) {
      input.focus()
      input.select()
    }
  })
})
watch(props.management.exitVersion, () => emits('dismiss'))
function memberDisplayName(member: OrganizationMemberSummary): string {
  return member.email || member.userId
}
async function confirmRemoveMember(): Promise<void> {
  const member = memberToRemove.value
  if (member) await props.management.removeMember(member)
  memberToRemove.value = null
}
async function leave(): Promise<void> {
  await props.management.leave()
  isLeaveArmed.value = false
}
async function close(): Promise<void> {
  if (!isCloseArmed.value) return
  await props.management.close()
  isCloseArmed.value = false
}
</script>
