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
    <div
      v-else-if="section.hasLoadError.value"
      class="flex flex-col items-start gap-2.5 rounded-panel border border-destructive/30 bg-destructive/10 p-4 text-sm text-foreground"
      aria-live="polite"
    >
      <p class="m-0 text-sm leading-normal text-foreground">
        {{ t('workspace.switcher.members.loadFailed') }}
      </p>
      <Button type="button" size="sm" @click="section.retryRoster()">
        {{ t('common.actions.retry') }}
      </Button>
    </div>

    <!-- Loaded Content: Invite Bar & Roster List -->
    <template v-else>
      <!-- Invite Member Card (when canManage) -->
      <section
        v-if="section.canManage.value"
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
              :invalid="section.inviteError.value !== null"
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
              {{
                section.isPending('invite')
                  ? t('workspace.switcher.members.inviting')
                  : t('workspace.switcher.members.inviteAction')
              }}
            </Button>
          </div>
          <p
            v-if="section.inviteError.value"
            class="m-0 text-xs leading-normal text-destructive"
            role="alert"
          >
            {{ section.inviteError.value }}
          </p>
        </div>
      </section>

      <!-- Member Roster Card -->
      <section :class="accountCardPanelClass" aria-labelledby="member-roster-title">
        <div class="flex items-center justify-between mb-3">
          <div class="flex items-center gap-2">
            <h3 id="member-roster-title" class="m-0 text-admin-body font-semibold text-foreground">
              {{ t('workspace.switcher.members.title') }}
            </h3>
            <span
              class="rounded-pill bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground font-mono"
            >
              {{ section.roster.value.length }}
            </span>
          </div>
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
                    v-if="userAvatarSrc(entry)"
                    :src="userAvatarSrc(entry)!"
                    :alt="memberDisplayName(entry)"
                  />
                  <AvatarFallback
                    class="bg-primary/10 text-primary font-semibold text-xs border border-primary/20 select-none uppercase"
                  >
                    {{ memberInitial(entry) }}
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

              <!-- Right: Role Control + Action (Laid out flat) -->
              <div class="flex items-center gap-2 shrink-0">
                <!-- Flat Segmented Group (Member / Admin / Owner / Leave|Remove) -->
                <div
                  v-if="section.canManage.value"
                  class="inline-flex items-center rounded-control bg-muted/60 p-0.5 text-xs border border-border/40"
                  role="radiogroup"
                  :aria-label="
                    t('workspace.switcher.members.roleForMember', {
                      userId: memberDisplayName(entry),
                    })
                  "
                >
                  <button
                    type="button"
                    role="radio"
                    :aria-checked="entry.role === 'member'"
                    :class="[
                      'h-7 px-2.5 rounded-control text-xs font-medium transition-[color,background-color,box-shadow] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer select-none',
                      entry.role === 'member'
                        ? 'bg-background text-foreground shadow-xs font-semibold'
                        : 'text-muted-foreground hover:text-foreground hover:bg-background/40',
                    ]"
                    :disabled="section.isPending(`role:${entry.userId}`)"
                    @click="onRoleSelect(entry.userId, 'member')"
                  >
                    {{ t('workspace.switcher.members.roleMember') }}
                  </button>
                  <button
                    type="button"
                    role="radio"
                    :aria-checked="entry.role === 'admin'"
                    :class="[
                      'h-7 px-2.5 rounded-control text-xs font-medium transition-[color,background-color,box-shadow] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer select-none',
                      entry.role === 'admin'
                        ? 'bg-background text-foreground shadow-xs font-semibold'
                        : 'text-muted-foreground hover:text-foreground hover:bg-background/40',
                    ]"
                    :disabled="section.isPending(`role:${entry.userId}`)"
                    @click="onRoleSelect(entry.userId, 'admin')"
                  >
                    {{ t('workspace.switcher.members.roleAdmin') }}
                  </button>
                  <button
                    type="button"
                    role="radio"
                    :aria-checked="entry.role === 'owner'"
                    :class="[
                      'h-7 px-2.5 rounded-control text-xs font-medium transition-[color,background-color,box-shadow] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] select-none',
                      entry.role === 'owner'
                        ? 'bg-background text-foreground shadow-xs font-semibold cursor-default'
                        : 'text-muted-foreground hover:text-foreground hover:bg-background/40 cursor-pointer',
                      !section.isOwner.value ? 'opacity-40 !cursor-not-allowed' : '',
                    ]"
                    :disabled="!section.isOwner.value || section.isPending(`role:${entry.userId}`)"
                    :title="
                      !section.isOwner.value
                        ? t('workspace.switcher.members.noPermission')
                        : undefined
                    "
                    @click="section.isOwner.value && onRoleSelect(entry.userId, 'owner')"
                  >
                    {{ t('workspace.switcher.members.roleOwner') }}
                  </button>

                  <span class="h-3.5 w-px bg-border/60 mx-1" aria-hidden="true" />

                  <!-- Remove/Leave Flat Button -->
                  <button
                    v-if="section.confirmingRemoveId.value !== entry.userId"
                    type="button"
                    class="h-7 px-2.5 rounded-control text-xs font-medium text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
                    :disabled="section.isPending(`remove:${entry.userId}`)"
                    @click="section.requestRemove(entry.userId)"
                  >
                    {{
                      entry.userId === props.currentUserId
                        ? t('workspace.switcher.members.leaveAction')
                        : t('workspace.switcher.members.removeAction')
                    }}
                  </button>
                </div>
                <span
                  v-else
                  class="shrink-0 rounded-pill px-2.5 py-0.5 text-xs font-medium bg-muted text-muted-foreground"
                >
                  {{ roleLabel(entry.role) }}
                </span>
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
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { resolveSizedAvatarUrl } from '../../../../core/media/avatarUrl'
import { useUserStore } from '../../../auth'
import { accountCardPanelClass } from '../shared/ui'
import { memberDisplayName, useWorkspaceMemberSection } from './useWorkspaceMemberSection'
import type { WorkspaceMemberEntry, WorkspaceMemberRole } from '../../session/accountWorkspace'

const props = defineProps<{
  workspaceId: string
  workspaceName: string
  currentUserId?: string | null | undefined
  isExpanded: boolean
}>()

const { t } = useI18n()
const userStore = useUserStore()

function memberInitial(entry: WorkspaceMemberEntry): string {
  if (entry.email) {
    return entry.email.charAt(0).toUpperCase()
  }
  return entry.userId.replace(/^usr_/, '').charAt(0).toUpperCase() || 'U'
}

function userAvatarSrc(entry: WorkspaceMemberEntry): string | undefined {
  if (entry.userId === userStore.user?.public_id && userStore.user?.avatar) {
    return resolveSizedAvatarUrl(userStore.user.avatar, 64)
  }
  return undefined
}

const section = useWorkspaceMemberSection({
  workspaceId: () => props.workspaceId,
  currentUserId: () => props.currentUserId,
  isExpanded: () => props.isExpanded,
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

async function onRoleSelect(userId: string, role: WorkspaceMemberRole): Promise<void> {
  const current = section.roster.value.find((entry) => entry.userId === userId)?.role
  if (current === role) return
  await section.changeMemberRole(userId, role)
}
</script>
