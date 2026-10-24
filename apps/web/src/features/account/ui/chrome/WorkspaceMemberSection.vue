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

    <!-- Error State -->
    <LoadErrorPanel
      v-else-if="section.hasLoadError.value"
      :message="t('workspace.switcher.members.loadFailed')"
      :retry-label="t('common.actions.retry')"
      @retry="section.retryRoster()"
    />

    <!-- Loaded Content: Invite Bar & Roster List -->
    <template v-else>
      <!-- Personal Default is private (#952): no invite bar, only the
        private note. Other Personal workspaces invite freely; consumption
        charges the holder's shared Personal account. -->
      <p
        v-if="section.isDefaultWorkspace.value"
        class="m-0 rounded-control border border-border bg-muted/40 p-3 text-admin-meta leading-normal text-muted-foreground"
        aria-live="polite"
      >
        {{ t('workspace.switcher.members.defaultPrivate') }}
      </p>

      <!-- Invite Member Card (when canManage, never on Default) -->
      <section
        v-if="section.canManage.value && !section.isDefaultWorkspace.value"
        :class="accountCardPanelClass"
        aria-label="Invite Member"
      >
        <div class="flex flex-col gap-3">
          <span class="text-admin-body font-semibold text-muted-foreground">
            {{ t('workspace.switcher.members.inviteAction') }}
          </span>
          <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
            <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The invite input carries its accessible name via aria-label; the rule cannot see through the Input primitive. -->
            <Input
              v-model="section.inviteEmail.value"
              size="sm"
              class="min-w-0 flex-1 h-9 text-xs bg-background"
              :disabled="section.isPending('invite')"
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
              :is-disabled="section.isPending('invite')"
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

                  <!-- Funds handover: billing owners start a transfer from a
                  member row (never the own row); the backend still enforces
                  owner-only initiation. -->
                  <button
                    v-if="handover.canHandOverTo(entry.userId)"
                    type="button"
                    class="h-8 px-2.5 rounded-control text-admin-meta font-medium text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
                    :disabled="handover.isSaving.value"
                    :aria-label="
                      t('workspace.switcher.handover.handOverTo', {
                        name: memberDisplayName(entry),
                      })
                    "
                    @click="handover.requestHandOver(entry.userId)"
                  >
                    {{ t('workspace.switcher.handover.handOver') }}
                  </button>

                  <!-- Independent Leave/Remove text button at the row end -->
                  <button
                    v-if="
                      section.canManage.value && section.confirmingRemoveId.value !== entry.userId
                    "
                    type="button"
                    class="h-8 px-2.5 rounded-control text-admin-meta font-medium text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
                    :disabled="section.isPending(`remove:${entry.userId}`)"
                    @click="section.requestRemove(entry.userId)"
                  >
                    {{
                      entry.userId === props.currentUserId
                        ? t('workspace.switcher.members.leaveAction')
                        : t('workspace.switcher.members.removeAction')
                    }}
                  </button>
                </template>
              </div>
            </div>

            <!-- Inline Remove Confirmation Box -->
            <div
              v-if="section.confirmingRemoveId.value === entry.userId"
              class="mt-1 flex flex-col gap-2 rounded-control border border-destructive/30 bg-destructive/10 p-3"
            >
              <p class="m-0 text-xs leading-normal text-foreground">
                {{
                  t('workspace.switcher.members.removeConfirmNote', {
                    name: memberDisplayName(entry),
                  })
                }}
              </p>
              <div class="flex items-center gap-2">
                <Button
                  type="button"
                  variant="destructive"
                  size="sm"
                  class="h-7 px-3 text-xs"
                  :is-loading="section.isPending(`remove:${entry.userId}`)"
                  :is-disabled="section.isPending(`remove:${entry.userId}`)"
                  @click="section.confirmRemove(entry.userId)"
                >
                  {{ t('workspace.switcher.members.removeConfirmAction') }}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  class="h-7 px-3 text-xs"
                  @click="section.cancelRemove()"
                >
                  {{ t('workspace.switcher.members.removeCancel') }}
                </Button>
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
    </template>
  </section>
  <!-- Funds handover confirmation: ordinary danger confirm (no typing) for
    initiation, primary for acceptance. Backend still enforces owner-only
    initiation and recipient-only decisions. -->
  <UserConfirmDialog
    :is-visible="handover.confirmation.value != null"
    :title="handoverTitle"
    :message="handoverMessage"
    :confirm-label="handoverTitle"
    :intent="handover.confirmation.value?.kind === 'accept' ? 'primary' : 'danger'"
    :is-loading="handover.isSaving.value"
    @update:is-visible="handover.dismissConfirmation()"
    @confirm="handover.confirm()"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NavArrowDown, SendMail } from '@iconoir/vue'
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
import LoadErrorPanel from '../../../../core/components/LoadErrorPanel.vue'
import UserConfirmDialog from '../../../../core/components/UserConfirmDialog.vue'
import { resolveMemberAvatar, resolveMemberInitial } from '../../../../core/media/avatarUrl'
import { appOverlayMenuSideOffset } from '../../../../theme/appOverlayMenu'
import { useUserStore } from '../../../auth'
import { accountCardPanelClass } from '../shared/ui'
import { useBillingTransferSection } from '../../session/billing-transfer/useBillingTransferSection'
import { memberDisplayName, useWorkspaceMemberSection } from '../../session/workspace-settings'
import type { WorkspaceMemberRole } from '../../session/accountWorkspace'

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

const handoverTitle = computed(() => {
  const pending = handover.confirmation.value
  if (pending?.kind === 'invite') return t('workspace.switcher.handover.handOver')
  return t(`workspace.switcher.handover.${pending?.kind ?? 'accept'}`)
})
const handoverMessage = computed(() => {
  const pending = handover.confirmation.value
  if (pending?.kind === 'invite') {
    const target = section.roster.value.find((entry) => entry.userId === pending.targetUserId)
    return t('workspace.switcher.handover.initiateConfirm', {
      account: handover.account.value?.billingAccountId ?? '',
      to: target ? memberDisplayName(target) : pending.targetUserId,
    })
  }
  return t(`workspace.switcher.handover.${pending?.kind ?? 'accept'}Confirm`, {
    account: handover.account.value?.billingAccountId ?? '',
  })
})

function roleLabel(role: WorkspaceMemberRole): string {
  switch (role) {
    case 'owner':
      return t('workspace.switcher.members.roleOwner')
    case 'admin':
      return t('workspace.switcher.members.roleAdmin')
    default:
      return t('workspace.switcher.members.roleMember')
  }
}

async function onRoleChange(userId: string, value: unknown): Promise<void> {
  if (value !== 'member' && value !== 'admin' && value !== 'owner') return
  if (value === 'owner' && !section.isOwner.value) return
  await section.changeMemberRole(userId, value)
}
</script>
