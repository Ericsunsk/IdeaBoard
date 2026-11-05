<template>
  <section
    v-show="props.isExpanded"
    class="flex flex-col gap-5 min-w-0"
    :aria-label="t('workspace.switcher.members.listLabel', { name: props.workspaceName })"
  >
    <!-- Loading State -->
    <div
      v-if="section.isLoading.value"
      class="flex min-h-32 flex-col items-center justify-center gap-2"
      aria-live="polite"
    >
      <Spinner class="size-6 text-primary" />
      <span class="text-xs text-muted-foreground">
        {{ t('common.actions.loading') }}
      </span>
    </div>

    <div
      v-else-if="section.hasLoadError.value"
      class="flex min-h-32 flex-col items-center justify-center gap-2"
      aria-live="polite"
    >
      <Button
        type="button"
        variant="outline"
        size="sm"
        class="h-8 shrink-0 px-3"
        @click="section.retryRoster()"
      >
        {{ t('common.actions.retry') }}
      </Button>
    </div>

    <!-- Loaded Content: Invite Bar & Roster List -->
    <template v-else>
      <!-- Personal Default is private (#952): no invite bar. Other Personal
        workspaces invite freely; consumption bills each submitter's own
        Personal account (payer = actor, #973). -->
      <!-- Invite Member Card (when canManage, never on Default) -->
      <section
        v-if="section.canManage.value && !section.isDefaultWorkspace.value"
        :class="accountCardPanelClass"
        :aria-label="t('workspace.switcher.members.inviteTitle')"
      >
        <div class="flex flex-col gap-3">
          <div class="flex items-center justify-between">
            <span class="text-admin-body font-semibold text-muted-foreground">
              {{ t('workspace.switcher.members.inviteTitle') }}
            </span>
            <span class="text-xs text-muted-foreground font-medium">
              {{ section.collaboratorCount.value }} / 5
            </span>
          </div>
          <p v-if="section.isLimitReached.value" class="m-0 text-xs text-muted-foreground">
            {{ t('workspace.switcher.members.memberLimitReached') }}
          </p>
          <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
            <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The invite input carries its accessible name via aria-label; the rule cannot see through the Input primitive. -->
            <Input
              v-model="section.inviteEmail.value"
              size="sm"
              class="min-w-0 flex-1 h-9 text-xs bg-background"
              :disabled="section.isPending('invite') || section.isLimitReached.value"
              :aria-label="t('workspace.switcher.members.emailLabel')"
              :placeholder="t('workspace.switcher.members.emailPlaceholder')"
              inputmode="email"
              autocomplete="email"
              @keydown.enter="section.inviteMember()"
            />
            <Button
              type="button"
              size="sm"
              class="h-9 shrink-0 px-4"
              :is-loading="section.isPending('invite')"
              :is-disabled="section.isPending('invite') || section.isLimitReached.value"
              @click="section.inviteMember()"
            >
              <SendMail class="size-4" aria-hidden="true" />
              {{
                section.isPending('invite')
                  ? t('workspace.switcher.members.inviting')
                  : t('workspace.switcher.members.inviteAction')
              }}
            </Button>
          </div>
        </div>
      </section>

      <!-- Ownership Transfer Active Banners (only rendered when there is a live pending invite) -->
      <div
        v-if="handover.addressedInvite.value"
        class="flex items-center gap-2.5 rounded-control border border-primary/30 bg-primary/10 p-3 text-xs"
        role="status"
      >
        <Crown class="size-4 shrink-0 text-primary" aria-hidden="true" />
        <div class="min-w-0">
          <p class="m-0 font-medium text-foreground">
            {{ t('workspace.switcher.handover.recipientNotice') }}
          </p>
          <p class="m-0 mt-0.5 text-muted-foreground truncate">
            {{
              t('workspace.switcher.handover.inviteParties', {
                from: handover.memberName(handover.addressedInvite.value.fromUserId),
                to: handover.memberName(handover.addressedInvite.value.toUserId),
              })
            }}
          </p>
        </div>
      </div>

      <div
        v-else-if="outgoingLiveInvite"
        class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 rounded-control border border-border bg-muted/40 p-3 text-xs"
        role="status"
      >
        <div class="flex items-center gap-2.5 min-w-0">
          <Crown class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          <div class="min-w-0">
            <p class="m-0 font-medium text-foreground">
              {{
                t('workspace.switcher.handover.inviteParties', {
                  from: handover.memberName(outgoingLiveInvite.fromUserId),
                  to: handover.memberName(outgoingLiveInvite.toUserId),
                })
              }}
            </p>
            <p class="m-0 mt-0.5 text-muted-foreground">
              {{ t('workspace.switcher.handover.expiresNote') }}
            </p>
          </div>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          class="h-7 px-2.5 text-xs shrink-0 text-muted-foreground hover:text-destructive hover:bg-destructive/10"
          :is-disabled="handover.isSaving.value"
          @click="handover.requestDecision(outgoingLiveInvite, 'cancel')"
        >
          {{ t('workspace.switcher.handover.cancel') }}
        </Button>
      </div>

      <!-- Member Roster Card -->
      <section :class="accountCardPanelClass" aria-labelledby="member-roster-title">
        <div class="flex items-center justify-between mb-3">
          <h3 id="member-roster-title" class="m-0 text-admin-body font-semibold text-foreground">
            {{ t('workspace.switcher.members.title') }}
          </h3>
          <span v-if="!section.canManage.value" class="text-xs text-muted-foreground">
            {{ t('workspace.switcher.members.readOnlyNote') }}
          </span>
        </div>

        <!-- Member Roster List -->
        <ul class="m-0 flex flex-col gap-2 list-none p-0">
          <li
            v-for="entry in section.roster.value"
            :key="entry.userId"
            class="flex flex-col gap-2 rounded-control p-2 hover:bg-muted/40 transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
          >
            <div class="flex items-center justify-between gap-3">
              <!-- Left: Avatar + Identity -->
              <div class="flex items-center gap-3 min-w-0 flex-1">
                <Avatar class="size-8 shrink-0">
                  <AvatarImage
                    v-if="resolveMemberAvatar(entry, userStore.user)"
                    :src="resolveMemberAvatar(entry, userStore.user)!"
                    :alt="memberDisplayName(entry)"
                  />
                  <AvatarFallback
                    class="bg-primary/10 text-primary font-semibold text-xs border border-primary/20 select-none uppercase"
                  >
                    {{ resolveMemberInitial(entry) }}
                  </AvatarFallback>
                </Avatar>
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2">
                    <span
                      class="text-xs font-semibold text-foreground truncate"
                      :title="memberDisplayName(entry)"
                    >
                      {{ memberDisplayName(entry) }}
                    </span>
                    <span
                      v-if="entry.userId === props.currentUserId"
                      class="shrink-0 rounded-pill bg-muted px-1.5 py-0.5 text-xs font-medium text-muted-foreground"
                    >
                      {{ t('workspace.switcher.members.currentUser') }}
                    </span>
                  </div>
                </div>
              </div>

              <!-- Right: Role Control + Action -->
              <div class="flex items-center gap-2 shrink-0">
                <template
                  v-if="handover.addressedInvite.value && entry.userId === props.currentUserId"
                >
                  <Button
                    type="button"
                    size="sm"
                    class="h-8 px-3"
                    :is-disabled="handover.isSaving.value"
                    :aria-label="t('workspace.switcher.handover.accept')"
                    @click="handover.requestDecision(handover.addressedInvite.value, 'accept')"
                  >
                    {{ t('workspace.switcher.handover.accept') }}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    class="h-8 px-3"
                    :is-disabled="handover.isSaving.value"
                    :aria-label="t('workspace.switcher.handover.reject')"
                    @click="handover.requestDecision(handover.addressedInvite.value, 'reject')"
                  >
                    {{ t('workspace.switcher.handover.reject') }}
                  </Button>
                </template>
                <template v-else>
                  <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The trigger carries its accessible name via :aria-label; the rule cannot see through the Select primitive. -->
                  <Select
                    v-if="section.canManage.value"
                    :model-value="entry.role"
                    :disabled="section.isPending(`role:${entry.userId}`)"
                    @update:model-value="onRoleChange(entry.userId, $event)"
                  >
                    <SelectTrigger
                      size="sm"
                      class="w-auto min-w-24 cursor-pointer gap-1.5"
                      :aria-label="
                        t('workspace.switcher.members.roleForMember', {
                          userId: memberDisplayName(entry),
                        })
                      "
                    >
                      <SelectValue>{{ roleLabel(entry.role) }}</SelectValue>
                      <SelectIcon>
                        <NavArrowDown class="size-3.5 shrink-0" aria-hidden="true" />
                      </SelectIcon>
                    </SelectTrigger>
                    <SelectContent
                      position="popper"
                      align="end"
                      :side-offset="appOverlayMenuSideOffset"
                    >
                      <SelectViewport>
                        <SelectItem value="member">
                          <SelectItemText>{{
                            t('workspace.switcher.members.roleMember')
                          }}</SelectItemText>
                        </SelectItem>
                        <SelectItem value="admin">
                          <SelectItemText>{{
                            t('workspace.switcher.members.roleAdmin')
                          }}</SelectItemText>
                        </SelectItem>
                        <SelectItem v-if="section.isOwner.value" value="owner">
                          <SelectItemText>{{
                            t('workspace.switcher.members.roleOwner')
                          }}</SelectItemText>
                        </SelectItem>
                      </SelectViewport>
                    </SelectContent>
                  </Select>
                  <span
                    v-else
                    class="shrink-0 rounded-pill px-2.5 py-0.5 text-xs font-medium bg-muted text-muted-foreground"
                  >
                    {{ roleLabel(entry.role) }}
                  </span>

                  <!-- Ownership transfer: workspace owners start a transfer from a
                  member row (never the own row); the backend still enforces
                  owner-only initiation. -->
                  <Button
                    v-if="handover.canHandOverTo(entry.userId)"
                    type="button"
                    variant="ghost"
                    size="icon"
                    class="size-8 text-muted-foreground hover:text-foreground"
                    :is-disabled="handover.isSaving.value"
                    :title="
                      t('workspace.switcher.handover.handOverTo', {
                        name: memberDisplayName(entry),
                      })
                    "
                    :aria-label="
                      t('workspace.switcher.handover.handOverTo', {
                        name: memberDisplayName(entry),
                      })
                    "
                    @click="handover.requestHandOver(entry.userId)"
                  >
                    <Crown class="size-4 shrink-0" aria-hidden="true" />
                  </Button>

                  <!-- Independent Leave/Remove icon button at the row end (never on Default workspace for current user) -->
                  <Button
                    v-if="
                      section.canManage.value &&
                      !(section.isDefaultWorkspace.value && entry.userId === props.currentUserId)
                    "
                    type="button"
                    variant="ghost"
                    size="icon"
                    class="size-8 text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                    :is-disabled="section.isPending(`remove:${entry.userId}`)"
                    :title="
                      entry.userId === props.currentUserId
                        ? t('workspace.switcher.members.leaveAction')
                        : t('workspace.switcher.members.removeAction')
                    "
                    :aria-label="
                      entry.userId === props.currentUserId
                        ? t('workspace.switcher.members.leaveAction')
                        : t('workspace.switcher.members.removeAction')
                    "
                    @click="section.requestRemove(entry.userId)"
                  >
                    <LogOut
                      v-if="entry.userId === props.currentUserId"
                      class="size-4 shrink-0"
                      aria-hidden="true"
                    />
                    <Trash v-else class="size-4 shrink-0" aria-hidden="true" />
                  </Button>
                </template>
              </div>
            </div>
          </li>
        </ul>

        <p
          v-if="section.roster.value.length === 0"
          class="py-6 text-center text-xs text-muted-foreground"
          aria-live="polite"
        >
          {{ t('workspace.switcher.members.empty') }}
        </p>
      </section>

      <!-- Pending Member Invites Card (when canManage && pendingInvites > 0) -->
      <section
        v-if="section.canManage.value && section.pendingInvites.value.length > 0"
        :class="accountCardPanelClass"
        aria-labelledby="pending-invites-title"
      >
        <div class="flex items-center justify-between mb-3">
          <h3 id="pending-invites-title" class="m-0 text-admin-body font-semibold text-foreground">
            {{ t('workspace.switcher.members.pendingTitle') }}
          </h3>
          <span class="text-xs text-muted-foreground font-medium">
            {{ section.pendingInvites.value.length }}
          </span>
        </div>

        <ul class="m-0 flex flex-col gap-2 list-none p-0">
          <li
            v-for="invite in section.pendingInvites.value"
            :key="invite.id"
            class="flex items-center justify-between gap-3 rounded-control p-2 hover:bg-muted/40 transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
          >
            <!-- Invitee Email & Expires -->
            <div class="flex flex-col min-w-0 flex-1">
              <span
                class="text-xs font-semibold text-foreground truncate"
                :title="invite.invitee_email"
              >
                {{ invite.invitee_email }}
              </span>
              <span class="text-admin-caption text-muted-foreground">
                {{ formatExpires(invite.expires_at) }}
              </span>
            </div>

            <!-- Role badge & Action buttons -->
            <div class="flex items-center gap-2 shrink-0">
              <span
                class="shrink-0 rounded-pill px-2.5 py-0.5 text-xs font-medium bg-muted text-muted-foreground"
              >
                {{ roleLabel(invite.role) }}
              </span>

              <Button
                v-if="section.getInviteLink(invite)"
                type="button"
                variant="ghost"
                size="sm"
                class="h-8 px-2 text-xs"
                :title="t('workspace.switcher.members.copyInviteLink')"
                @click="section.copyInviteLink(invite)"
              >
                <LinkIcon class="size-3.5 mr-1" aria-hidden="true" />
                {{ t('workspace.switcher.members.copyInviteLink') }}
              </Button>

              <Button
                type="button"
                variant="outline"
                size="sm"
                class="h-8 px-2.5 text-xs text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                :is-loading="section.isPending(`revoke:${invite.id}`)"
                :is-disabled="section.isPending(`revoke:${invite.id}`)"
                @click="section.revokeInvite(invite.id)"
              >
                {{ t('workspace.switcher.members.revokeAction') }}
              </Button>
            </div>
          </li>
        </ul>
      </section>
    </template>
  </section>
  <!-- Ownership transfer confirmation: ordinary danger confirm (no typing) for
    initiation, primary for acceptance. Backend still enforces owner-only
    initiation and recipient-only decisions. -->
  <ConfirmDialog
    surface="user"
    :is-visible="handover.confirmation.value != null"
    :title="handoverTitle"
    :message="handoverMessage"
    :confirm-label="handoverTitle"
    :intent="handover.confirmation.value?.kind === 'accept' ? 'primary' : 'danger'"
    :is-loading="handover.isSaving.value"
    @update:is-visible="handover.dismissConfirmation()"
    @confirm="handover.confirm()"
  />

  <!-- Member removal confirmation modal dialog -->
  <ConfirmDialog
    surface="user"
    :is-visible="section.confirmingRemoveId.value != null"
    :title="removeDialogTitle"
    :message="removeDialogMessage"
    :confirm-label="t('workspace.switcher.members.removeConfirmAction')"
    intent="danger"
    :is-loading="
      section.confirmingRemoveId.value != null &&
      section.isPending(`remove:${section.confirmingRemoveId.value}`)
    "
    @update:is-visible="section.cancelRemove()"
    @confirm="
      section.confirmingRemoveId.value && section.confirmRemove(section.confirmingRemoveId.value)
    "
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Crown, Link as LinkIcon, LogOut, NavArrowDown, SendMail, Trash } from '@iconoir/vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectItemText,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import ConfirmDialog from '@/core/components/ConfirmDialog.vue'
import { useFailureToast } from '@/core/composables/useFailureToast'
import { resolveMemberAvatar, resolveMemberInitial } from '@/core/media/avatarUrl'
import { appOverlayMenuSideOffset } from '@/theme/appOverlayMenu'
import { useUserStore } from '@/features/auth'
import { accountCardPanelClass } from '@/features/account'
import { useBillingTransferSection } from '@/features/account'
import {
  memberDisplayName,
  useWorkspaceMemberSection,
} from '@/features/account/session/workspace-settings'
import type { WorkspaceMemberRole } from '@/features/account/session/accountWorkspace'

const props = defineProps<{
  workspaceId: string
  workspaceName: string
  currentUserId?: string | null | undefined
  isExpanded: boolean
  // Personal Default marker from the workspace list (#952): when true the
  // invite bar hides and the private note shows instead.
  isDefaultWorkspace?: boolean | undefined
}>()

const { t } = useI18n()
const userStore = useUserStore()

const section = useWorkspaceMemberSection({
  workspaceId: () => props.workspaceId,
  currentUserId: () => props.currentUserId,
  isExpanded: () => props.isExpanded,
  isDefaultWorkspace: () => props.isDefaultWorkspace ?? false,
})

const handover = useBillingTransferSection({
  workspaceId: () => props.workspaceId,
  currentUserId: () => props.currentUserId,
})

const outgoingLiveInvite = computed(
  () => handover.liveInvites.value.find((invite) => handover.canDecide(invite, 'cancel')) ?? null,
)

useFailureToast(
  () => section.hasLoadError.value,
  () => t('workspace.switcher.members.loadFailed'),
)

const handoverTitle = computed(() => {
  const pending = handover.confirmation.value
  if (pending?.kind === 'invite') return t('workspace.switcher.handover.handOver')
  return t(`workspace.switcher.handover.${pending?.kind ?? 'accept'}`)
})
const handoverMessage = computed(() => {
  const pending = handover.confirmation.value
  if (!pending) return ''
  if (pending.kind === 'invite') {
    const target = section.roster.value.find((entry) => entry.userId === pending.targetUserId)
    return t('workspace.switcher.handover.initiateConfirm', {
      to: target ? memberDisplayName(target) : pending.targetUserId,
    })
  }
  if (pending.kind === 'cancel') {
    const target = section.roster.value.find((entry) => entry.userId === pending.invite.toUserId)
    return t('workspace.switcher.handover.cancelConfirm', {
      to: target ? memberDisplayName(target) : pending.invite.toUserId,
    })
  }
  return t(`workspace.switcher.handover.${pending.kind}Confirm`)
})

const targetRemoveMember = computed(() => {
  const id = section.confirmingRemoveId.value
  return id ? (section.roster.value.find((entry) => entry.userId === id) ?? null) : null
})

const removeDialogTitle = computed(() =>
  section.confirmingRemoveId.value === props.currentUserId
    ? t('workspace.switcher.members.leaveAction')
    : t('workspace.switcher.members.removeAction'),
)

const removeDialogMessage = computed(() => {
  if (
    section.confirmingRemoveId.value != null &&
    section.confirmingRemoveId.value === props.currentUserId
  ) {
    return t('workspace.switcher.members.leaveConfirmNote')
  }
  const target = targetRemoveMember.value
  const name = target ? memberDisplayName(target) : (section.confirmingRemoveId.value ?? '')
  return t('workspace.switcher.members.removeConfirmNote', { name })
})

function roleLabel(role: WorkspaceMemberRole | string): string {
  switch (role) {
    case 'owner':
      return t('workspace.switcher.members.roleOwner')
    case 'admin':
      return t('workspace.switcher.members.roleAdmin')
    default:
      return t('workspace.switcher.members.roleMember')
  }
}

function formatExpires(expiresAt: string): string {
  try {
    const target = new Date(expiresAt).getTime()
    const now = Date.now()
    const diff = target - now
    if (diff <= 0) return t('workspace.switcher.handover.status.expired')
    const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
    return t('workspace.switcher.members.expiresInDays', { count: days })
  } catch {
    return ''
  }
}

async function onRoleChange(userId: string, value: unknown): Promise<void> {
  if (value !== 'member' && value !== 'admin' && value !== 'owner') return
  if (value === 'owner' && !section.isOwner.value) return
  await section.changeMemberRole(userId, value)
}
</script>
