<template>
  <section
    class="flex flex-col gap-3 rounded-panel border border-border bg-card p-4 shadow-raised sm:p-5"
    :aria-label="t('workspace.management.title', { name: props.organizationName })"
  >
    <div class="flex items-center justify-between gap-2">
      <div class="min-w-0">
        <p class="m-0 text-admin-content font-medium text-foreground">
          {{ t('workspace.management.title', { name: props.organizationName }) }}
        </p>
        <p class="m-0 mt-0.5 text-admin-meta text-muted-foreground">
          {{ t('workspace.management.managementNote') }}
        </p>
        <p v-if="isClosed" class="m-0 mt-1 text-admin-meta font-medium text-muted-foreground">
          {{ t('workspace.management.statusClosed') }}
        </p>
      </div>
      <Button
        v-if="props.canLeave && !isClosed"
        type="button"
        variant="outline"
        size="sm"
        class="h-8 shrink-0"
        :is-disabled="isLeaving"
        @click="leave"
      >
        {{ isLeaving ? t('workspace.management.leaving') : t('workspace.management.leaveAction') }}
      </Button>
    </div>

    <p v-if="loadError" class="m-0 text-admin-meta text-destructive" role="alert">
      {{ loadError }}
      <Button type="button" variant="ghost" size="sm" class="h-6 px-2" @click="reload">
        {{ t('common.actions.retry') }}
      </Button>
    </p>

    <ul v-else class="m-0 flex list-none flex-col gap-1.5 p-0">
      <li
        v-for="member in members"
        :key="member.userId"
        class="flex items-center justify-between gap-2 rounded-control bg-muted/40 px-2 py-1.5"
      >
        <div class="flex min-w-0 items-center gap-2">
          <span class="truncate font-mono text-admin-meta text-foreground">{{
            member.userId
          }}</span>
          <span
            class="shrink-0 rounded-pill border border-border px-1.5 py-0.5 text-admin-caption text-muted-foreground"
          >
            {{ roleLabel(member.role) }}
          </span>
          <span
            v-if="member.userId === props.currentUserId"
            class="shrink-0 text-admin-caption text-muted-foreground"
          >
            {{ t('workspace.switcher.members.currentUser') }}
          </span>
        </div>
        <div class="flex shrink-0 items-center gap-1">
          <select
            v-if="canManageRole(member)"
            class="h-7 rounded-control border border-border bg-background text-admin-meta"
            :value="member.role"
            :disabled="isSaving"
            :aria-label="t('workspace.switcher.members.roleForMember', { userId: member.userId })"
            @change="changeRole(member, ($event.target as HTMLSelectElement).value)"
          >
            <option value="member">{{ t('workspace.switcher.members.roleMember') }}</option>
            <option value="admin">{{ t('workspace.switcher.members.roleAdmin') }}</option>
          </select>
          <Button
            v-if="canRemove(member)"
            type="button"
            variant="ghost"
            size="sm"
            class="h-7 px-2 text-destructive"
            :is-disabled="isSaving"
            @click="remove(member)"
          >
            {{ t('workspace.switcher.members.removeAction') }}
          </Button>
        </div>
      </li>
    </ul>

    <div
      v-if="props.canManage && !isClosed"
      class="flex flex-col gap-2 border-t border-border pt-3"
    >
      <span class="text-admin-meta font-semibold text-muted-foreground">
        {{ t('workspace.management.inviteTitle') }}
      </span>
      <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
        <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- Accessible name via aria-label. -->
        <Input
          v-model="inviteUserId"
          size="sm"
          class="h-8 min-w-0 flex-1 font-mono text-admin-meta"
          :disabled="isSaving"
          :aria-label="t('workspace.management.invitePlaceholder')"
          :placeholder="t('workspace.management.invitePlaceholder')"
          @keydown.enter="invite"
        />
        <select
          v-model="inviteRole"
          class="h-8 rounded-control border border-border bg-background text-admin-meta"
          :disabled="isSaving"
          :aria-label="
            t('workspace.switcher.members.roleForMember', { userId: inviteUserId || 'new' })
          "
        >
          <option value="member">{{ t('workspace.switcher.members.roleMember') }}</option>
          <option v-if="props.actorRole === 'owner'" value="admin">
            {{ t('workspace.switcher.members.roleAdmin') }}
          </option>
        </select>
        <Button
          type="button"
          size="sm"
          class="h-8 shrink-0"
          :is-loading="isSaving"
          :is-disabled="isSaving || !inviteUserId.trim()"
          @click="invite"
        >
          {{ t('workspace.switcher.members.inviteAction') }}
        </Button>
      </div>
      <p v-if="actionError" class="m-0 text-admin-meta text-destructive" role="alert">
        {{ actionError }}
      </p>
      <p class="m-0 text-admin-meta text-muted-foreground">
        {{ t('workspace.management.workspaceGrantNote') }}
      </p>
    </div>
    <p v-else class="m-0 text-admin-meta text-muted-foreground">
      {{ t('workspace.switcher.members.readOnlyNote') }}
    </p>

    <div
      v-if="shouldShowDangerZone"
      class="flex flex-col gap-2 border-t border-border pt-3"
      :aria-label="t('workspace.management.closeTitle')"
    >
      <p v-if="isClosed" class="m-0 text-admin-meta text-muted-foreground" role="status">
        {{ t('workspace.management.closedNote') }}
      </p>
      <template v-else>
        <span class="text-admin-meta font-semibold text-muted-foreground">
          {{ t('workspace.management.closeTitle') }}
        </span>
        <p class="m-0 text-admin-meta text-muted-foreground">
          {{ t('workspace.management.closeHint') }}
        </p>
        <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- Accessible name via aria-label. -->
        <Input
          v-model="closeReason"
          size="sm"
          class="h-8 min-w-0 text-admin-meta"
          :disabled="isClosing"
          :maxlength="280"
          :aria-label="t('workspace.management.closeReasonPlaceholder')"
          :placeholder="t('workspace.management.closeReasonPlaceholder')"
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          class="h-8 shrink-0 self-start text-destructive"
          :is-disabled="isClosing"
          @click="isCloseArmed = true"
        >
          {{ t('workspace.management.closeAction') }}
        </Button>
      </template>
      <p v-if="closureError" class="m-0 text-admin-meta text-destructive" role="alert">
        {{ closureError }}
      </p>
    </div>
  </section>
  <UserConfirmDialog
    :is-visible="isCloseArmed"
    :title="t('workspace.management.closeTitle')"
    :message="t('workspace.management.closeHint')"
    :confirm-label="t('workspace.management.closeConfirmAction')"
    :expected-confirm-text="props.organizationName"
    :confirm-input-label="
      t('workspace.management.closeConfirmInputLabel', { name: props.organizationName })
    "
    :confirm-input-placeholder="t('workspace.management.closeConfirmInputPlaceholder')"
    :is-loading="isClosing"
    intent="danger"
    @update:is-visible="isCloseArmed = $event"
    @confirm="close"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import UserConfirmDialog from '../../../../core/components/UserConfirmDialog.vue'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import type {
  OrganizationMemberSummary,
  OrganizationRole,
  OrganizationStatus,
} from '../../selection/workspaceResource'

const props = withDefaults(
  defineProps<{
    organizationId: string
    organizationName: string
    organizationStatus?: OrganizationStatus | undefined
    closedAt?: string | null | undefined
    actorRole?: OrganizationRole | null | undefined
    currentUserId?: string | null | undefined
    canManage?: boolean | undefined
    canLeave?: boolean | undefined
    onLoadMembers: (organizationId: string) => Promise<OrganizationMemberSummary[]>
    onInviteMember: (
      organizationId: string,
      input: { userId: string; role: 'member' | 'admin' },
    ) => Promise<OrganizationMemberSummary>
    onUpdateMember: (
      organizationId: string,
      memberId: string,
      input: { role: 'member' | 'admin' },
    ) => Promise<OrganizationMemberSummary>
    onRemoveMember: (organizationId: string, memberId: string) => Promise<void>
    onLeave?: ((organizationId: string, memberId: string) => Promise<void>) | undefined
    onCloseOrganization?:
      | ((
          organizationId: string,
          input: { reason?: string | undefined },
        ) => Promise<{ status?: string | undefined }>)
      | undefined
  }>(),
  {
    organizationStatus: 'active',
    closedAt: null,
    actorRole: null,
    currentUserId: null,
    canManage: false,
    canLeave: false,
    onLeave: undefined,
    onCloseOrganization: undefined,
  },
)

const { t } = useI18n()

// Closure is owner-driven; the dialog re-reads server truth on its next load.
const statusRef = ref<OrganizationStatus>(props.organizationStatus)
watch(
  () => props.organizationStatus,
  (next) => {
    statusRef.value = next
  },
)
const isClosed = computed(() => statusRef.value === 'closed')
const shouldShowDangerZone = computed(
  () => props.actorRole === 'owner' && props.onCloseOrganization !== undefined,
)

const members = ref<OrganizationMemberSummary[]>([])
const loadError = ref<string | null>(null)
const actionError = ref<string | null>(null)
const isSaving = ref(false)
const isLeaving = ref(false)
const inviteUserId = ref('')
const inviteRole = ref<'member' | 'admin'>('member')
const closeReason = ref('')
const isCloseArmed = ref(false)
const isClosing = ref(false)
const closureError = ref<string | null>(null)

function roleLabel(role: OrganizationRole): string {
  switch (role) {
    case 'owner':
      return t('workspace.switcher.members.roleOwner')
    case 'admin':
      return t('workspace.switcher.members.roleAdmin')
    default:
      return t('workspace.switcher.members.roleMember')
  }
}

function canManageRole(member: OrganizationMemberSummary): boolean {
  if (isClosed.value) return false
  if (!props.canManage) return false
  if (member.role === 'owner') return false
  if (member.userId === props.currentUserId && member.role === 'admin') return false
  if (props.actorRole !== 'owner' && member.role === 'admin') return false
  return true
}

function canRemove(member: OrganizationMemberSummary): boolean {
  if (isClosed.value) return false
  if (member.role === 'owner') return false
  if (member.userId === props.currentUserId) return props.canLeave
  return props.canManage === true
}

async function reload(): Promise<void> {
  loadError.value = null
  try {
    members.value = await props.onLoadMembers(props.organizationId)
  } catch {
    loadError.value = t('workspace.switcher.members.loadFailed')
  }
}

const ACTION_ERROR_RULES: Array<[string, string]> = [
  ['organization_limit_reached', 'workspace.management.limitReached'],
  ['organization_closed', 'workspace.management.closeBlocked'],
  ['organization_closure_blocked', 'workspace.management.closeBlocked'],
  ['membership_transition_forbidden', 'workspace.switcher.members.noPermission'],
  ['organization_member_not_found', 'workspace.switcher.members.memberUnknown'],
  ['not_found', 'workspace.switcher.members.memberUnknown'],
]

function toActionError(error: unknown): string {
  const code = userErrorCode(error)?.toLowerCase() ?? ''
  const hit = ACTION_ERROR_RULES.find(([substr]) => code.includes(substr))
  return t(hit?.[1] ?? 'workspace.switcher.members.actionFailed')
}

function toClosureError(error: unknown, fallback: string): string {
  const code = userErrorCode(error)?.toLowerCase() ?? ''
  if (code.includes('organization_closure_blocked')) return t('workspace.management.closeBlocked')
  return fallback
}

async function close(): Promise<void> {
  if (!props.onCloseOrganization || !isCloseArmed.value || isClosed.value || isClosing.value) return
  isClosing.value = true
  closureError.value = null
  try {
    const closed = await props.onCloseOrganization(props.organizationId, {
      ...(closeReason.value.trim() ? { reason: closeReason.value.trim() } : {}),
    })
    if (closed?.status === 'closed') statusRef.value = 'closed'
    isCloseArmed.value = false
    closeReason.value = ''
  } catch (error) {
    isCloseArmed.value = false
    closureError.value = toClosureError(error, t('workspace.management.closeFailed'))
  } finally {
    isClosing.value = false
  }
}

async function invite(): Promise<void> {
  const userId = inviteUserId.value.trim()
  if (!userId || isSaving.value) return
  isSaving.value = true
  actionError.value = null
  try {
    const added = await props.onInviteMember(props.organizationId, {
      userId,
      role: inviteRole.value,
    })
    if (!members.value.some((m) => m.userId === added.userId)) members.value.push(added)
    inviteUserId.value = ''
  } catch (error) {
    actionError.value = toActionError(error)
  } finally {
    isSaving.value = false
  }
}

async function changeRole(member: OrganizationMemberSummary, role: string): Promise<void> {
  if (role !== 'member' && role !== 'admin') return
  isSaving.value = true
  actionError.value = null
  try {
    const updated = await props.onUpdateMember(props.organizationId, member.userId, { role })
    members.value = members.value.map((m) => (m.userId === member.userId ? updated : m))
  } catch (error) {
    actionError.value = toActionError(error)
  } finally {
    isSaving.value = false
  }
}

async function remove(member: OrganizationMemberSummary): Promise<void> {
  if (member.userId === props.currentUserId) {
    await leave(member)
    return
  }
  isSaving.value = true
  actionError.value = null
  try {
    await props.onRemoveMember(props.organizationId, member.userId)
    members.value = members.value.filter((m) => m.userId !== member.userId)
  } catch (error) {
    actionError.value = toActionError(error)
  } finally {
    isSaving.value = false
  }
}

async function leave(self?: OrganizationMemberSummary): Promise<void> {
  const target = self ?? members.value.find((m) => m.userId === props.currentUserId)
  if (!target || isLeaving.value) return
  isLeaving.value = true
  actionError.value = null
  try {
    if (props.onLeave) await props.onLeave(props.organizationId, target.userId)
    else await props.onRemoveMember(props.organizationId, target.userId)
    members.value = members.value.filter((m) => m.userId !== target.userId)
  } catch (error) {
    actionError.value = toActionError(error)
  } finally {
    isLeaving.value = false
  }
}

onMounted(() => {
  void reload()
})

watch(
  () => props.organizationId,
  () => {
    void reload()
  },
)
</script>
