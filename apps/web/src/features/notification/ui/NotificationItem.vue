<template>
  <div
    :class="[
      'group relative flex items-start gap-3.5 px-4 sm:px-6 py-3 transition-colors hover:bg-muted focus-visible:outline-none focus-visible:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset',
      isUnread ? 'cursor-pointer' : '',
    ]"
    data-testid="notification-item"
    :tabindex="isUnread ? 0 : undefined"
    :role="isUnread ? 'button' : undefined"
    :aria-label="
      isUnread ? `${t('notification.actions.markAsRead')}: ${props.notification.title}` : undefined
    "
    @click="handleClickItem"
    @keydown="handleKeydownItem"
  >
    <!-- Type Icon -->
    <div
      class="flex shrink-0 items-center justify-center pt-0.5"
      data-testid="notification-item-icon"
      role="img"
      :aria-label="typeLabel"
    >
      <component :is="itemIcon" :class="['size-5 shrink-0', itemIconClass]" />
    </div>

    <!-- Content -->
    <div class="min-w-0 flex-1">
      <div class="flex items-start justify-between gap-2">
        <div class="flex items-center gap-1.5 min-w-0 flex-1">
          <span
            v-if="isUnread"
            class="size-2 rounded-pill bg-destructive shrink-0"
            data-testid="notification-unread-dot"
            aria-hidden="true"
          />
          <h4
            :class="[
              'text-xs leading-snug break-words',
              isUnread ? 'font-semibold text-foreground' : 'font-medium text-foreground',
            ]"
            data-testid="notification-item-title"
          >
            {{ props.notification.title }}
          </h4>
        </div>
        <div class="flex shrink-0 items-center gap-1 pt-0.5">
          <time
            v-if="formattedRelativeTime"
            class="text-admin-micro text-muted-foreground"
            data-testid="notification-item-time"
          >
            {{ formattedRelativeTime }}
          </time>
          <button
            v-if="props.notification.status !== 'archived'"
            type="button"
            class="hit-target-32 inline-flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-control text-muted-foreground opacity-100 transition-opacity duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted hover:text-foreground focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring group-focus-within:opacity-100 sm:opacity-0 sm:group-hover:opacity-100"
            data-testid="archive-notification-btn"
            :title="t('notification.actions.archive')"
            :aria-label="t('notification.actions.archive')"
            @click.stop="handleArchiveItem"
          >
            <Archive class="size-4 shrink-0" />
          </button>
        </div>
      </div>

      <p
        class="mt-1 text-xs text-muted-foreground line-clamp-2 break-words"
        data-testid="notification-item-body"
      >
        {{ props.notification.body }}
      </p>

      <!-- Declarative Action Registry resolution -->
      <div v-if="actionResolution" class="mt-2.5 flex items-center gap-2">
        <template v-if="effectiveStatusPill">
          <span
            class="inline-flex items-center rounded-pill bg-muted px-2.5 py-0.5 text-admin-caption font-medium text-muted-foreground"
            :data-testid="effectiveStatusPill.testId"
          >
            {{ t(effectiveStatusPill.labelKey) }}
          </span>
        </template>
        <template v-else-if="actionResolution.actions.length > 0">
          <button
            v-for="action in actionResolution.actions"
            :key="action.id"
            type="button"
            :class="[
              'hit-target-32-inline inline-flex h-6 items-center justify-center rounded-pill px-3 text-admin-caption font-medium transition-colors disabled:pointer-events-none cursor-pointer',
              action.variant === 'outline'
                ? 'border border-border bg-background text-foreground hover:bg-muted'
                : 'bg-primary text-primary-foreground hover:bg-primary-hover',
            ]"
            :disabled="isActionPending"
            :data-testid="action.testId"
            @click.stop="handleExecuteAction(action)"
          >
            {{ t(action.labelKey) }}
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Archive, Bell, Coins, Crown, Mail } from '@iconoir/vue'
import type { NotificationDTO } from '@/api/generated-query/public/model'
import {
  resolveNotificationActions,
  type NotificationActionItem,
} from '../session/notificationActionRegistry'
import { resolveNotificationDerivedState } from '../session/notificationDisplay'

interface NotificationItemProps {
  notification: NotificationDTO
  onAcceptInvite?: (inviteId: string, notificationId: string) => Promise<boolean>
  onDeclineInvite?: (inviteId: string, notificationId: string) => Promise<boolean>
}

const props = defineProps<NotificationItemProps>()
const emit = defineEmits<{
  read: [id: string]
  archive: [id: string]
}>()

const { t } = useI18n()
const isActionPending = ref(false)

// Thin rendering: the pill is purely derived from the notification the
// session hands down (server invite_status + read/archived + local decision
// merged by the session). No local optimistic copies here; the pending
// flag only keeps in-flight buttons disabled for instant feedback.
const isUnread = computed(() => props.notification.status === 'unread')

// Maps the server notification type onto the `notification.types.*` locale
// bundle so the dead keys stay live as the icon's accessible label.
const typeLabel = computed(() => {
  switch (props.notification.type) {
    case 'workspace_invite':
      return t('notification.types.workspaceInvite')
    case 'organization_invite':
      return t('notification.types.organizationInvite')
    case 'workspace_member_joined':
      return t('notification.types.workspaceMemberJoined')
    case 'organization_member_joined':
      return t('notification.types.organizationMemberJoined')
    case 'workspace_invite_declined':
      return t('notification.types.workspaceInviteDeclined')
    case 'organization_invite_declined':
      return t('notification.types.organizationInviteDeclined')
    case 'studio_handover':
      return t('notification.types.studioHandover')
    case 'billing_credit_low':
      return t('notification.types.billingCreditLow')
    case 'billing_recharge_success':
      return t('notification.types.billingRechargeSuccess')
    default:
      return t('notification.types.system')
  }
})

const itemIcon = computed(() => {
  switch (props.notification.type) {
    case 'workspace_invite':
    case 'organization_invite':
    case 'workspace_member_joined':
    case 'organization_member_joined':
    case 'workspace_invite_declined':
    case 'organization_invite_declined':
      return Mail
    case 'studio_handover':
      return Crown
    case 'billing_credit_low':
    case 'billing_recharge_success':
      return Coins
    default:
      return Bell
  }
})

const itemIconClass = computed(() => {
  switch (props.notification.type) {
    case 'workspace_invite':
    case 'organization_invite':
    case 'workspace_member_joined':
    case 'organization_member_joined':
    case 'workspace_invite_declined':
    case 'organization_invite_declined':
      return 'text-primary'
    case 'studio_handover':
      return 'text-warning'
    case 'billing_credit_low':
      return 'text-destructive'
    case 'billing_recharge_success':
      return 'text-success'
    default:
      return 'text-muted-foreground'
  }
})

const actionResolution = computed(() => {
  return resolveNotificationActions(props.notification, {
    onAcceptInvite: props.onAcceptInvite,
    onDeclineInvite: props.onDeclineInvite,
  })
})

const effectiveStatusPill = computed(() => {
  return resolveNotificationDerivedState(props.notification).pill
})

const formattedRelativeTime = computed(() => {
  if (!props.notification.created_at) return ''
  const date = new Date(props.notification.created_at)
  if (isNaN(date.getTime())) return ''
  const diffSeconds = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000))
  if (diffSeconds < 60) {
    return t('notification.time.justNow')
  }
  const diffMinutes = Math.floor(diffSeconds / 60)
  if (diffMinutes < 60) {
    return t('notification.time.minutesAgo', { count: diffMinutes })
  }
  const diffHours = Math.floor(diffMinutes / 60)
  if (diffHours < 24) {
    return t('notification.time.hoursAgo', { count: diffHours })
  }
  const diffDays = Math.floor(diffHours / 24)
  return t('notification.time.daysAgo', { count: diffDays })
})

function handleClickItem() {
  if (isUnread.value) {
    emit('read', props.notification.id)
  }
}

function handleKeydownItem(event: KeyboardEvent) {
  // Inner action buttons keep their native keyboard behavior; only the row
  // itself is the mark-as-read target.
  if (event.target !== event.currentTarget) return
  if (!isUnread.value) return
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    handleClickItem()
  }
}

function handleArchiveItem() {
  emit('archive', props.notification.id)
}

async function handleExecuteAction(action: NotificationActionItem): Promise<void> {
  if (isActionPending.value) return
  isActionPending.value = true
  try {
    const isSuccess = await action.execute({
      notification: props.notification,
      payload: (props.notification.action_payload as Record<string, unknown> | undefined) ?? {},
    })
    if (isSuccess) {
      // The joined/declined pill lands via the session-owned invite decision;
      // the read-status move is applied by the session too.
      emit('read', props.notification.id)
    }
  } finally {
    isActionPending.value = false
  }
}
</script>
