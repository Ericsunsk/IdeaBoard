<template>
  <Popover v-model:open="isPopoverOpen">
    <PopoverTrigger as-child>
      <slot name="trigger">
        <span class="sr-only">Notifications trigger</span>
      </slot>
    </PopoverTrigger>
    <PopoverContent
      align="end"
      :side-offset="8"
      class="w-80 sm:w-96 p-0 overflow-hidden shadow-admin-popover rounded-panel bg-popover border border-border"
      data-testid="notification-popover-content"
    >
      <!-- Header -->
      <div class="flex items-center justify-between border-b border-border px-4 py-2.5">
        <h3 class="text-sm font-semibold text-foreground">
          {{ t('notification.popover.title') }}
        </h3>
        <button
          type="button"
          class="hit-target-32 flex size-7 items-center justify-center rounded-control text-muted-foreground hover:text-foreground hover:bg-muted transition-colors cursor-pointer"
          data-testid="expand-notifications-btn"
          :title="t('notification.popover.viewAll')"
          :aria-label="t('notification.popover.viewAll')"
          @click="handleOpenCenterDialog"
        >
          <Menu class="size-4 shrink-0" />
        </button>
      </div>

      <!-- Body / Scrollable List -->
      <div class="max-h-96 overflow-y-auto">
        <div
          v-if="shouldShowListSkeleton"
          class="divide-y divide-border"
          data-testid="notification-skeleton"
          role="status"
          :aria-label="t('common.actions.loading')"
        >
          <div
            v-for="index in 3"
            :key="index"
            class="flex items-start gap-3.5 px-4 py-3"
            aria-hidden="true"
          >
            <Skeleton class="size-5 shrink-0" />
            <div class="min-w-0 flex-1 space-y-2 pt-0.5">
              <Skeleton height="12px" width="55%" />
              <Skeleton height="12px" width="90%" />
            </div>
          </div>
        </div>
        <div
          v-else-if="shouldShowListError"
          class="flex flex-col items-center justify-center gap-2 px-4 py-10 text-center"
          data-testid="notification-list-error"
        >
          <p class="m-0 text-xs text-muted-foreground">
            {{ t('notification.popover.loadFailed') }}
          </p>
          <button
            type="button"
            class="inline-flex min-h-8 cursor-pointer items-center justify-center rounded-control px-3 text-xs font-medium text-primary transition-colors hover:bg-muted"
            data-testid="notification-retry-btn"
            @click="handleRetryList"
          >
            {{ t('common.actions.retry') }}
          </button>
        </div>
        <TransitionGroup
          v-else-if="sortedNotifications.length > 0"
          name="notification-popover-item"
          tag="div"
          class="relative divide-y divide-border"
        >
          <NotificationItem
            v-for="item in sortedNotifications"
            :key="item.id"
            :notification="item"
            :on-accept-invite="handleAcceptInvite"
            :on-decline-invite="handleDeclineInvite"
            @read="handleMarkAsRead"
            @archive="handleArchive"
          />
        </TransitionGroup>
        <div
          v-else
          class="flex flex-col items-center justify-center py-10 px-4 text-center text-muted-foreground"
          data-testid="notification-empty"
        >
          <Bell class="size-8 stroke-1 text-muted-foreground mb-2" />
          <p class="text-xs">
            {{ t('notification.popover.emptyAll') }}
          </p>
        </div>
      </div>
    </PopoverContent>
  </Popover>

  <!-- Notification Center Dialog -->
  <NotificationCenterDialog v-model:is-open="isCenterDialogOpen" />
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Bell, Menu } from '@iconoir/vue'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Skeleton } from '@/components/ui/skeleton'
import type { NotificationDTO } from '@/api/generated-query/public/model'
import NotificationItem from './NotificationItem.vue'
import NotificationCenterDialog from './NotificationCenterDialog.vue'
import { useNotifications } from '../session/useNotifications'

interface NotificationPopoverProps {
  isOpen?: boolean | undefined
}

const props = withDefaults(defineProps<NotificationPopoverProps>(), {
  isOpen: undefined,
})
const emit = defineEmits<{
  'update:isOpen': [value: boolean]
}>()

const isLocalOpen = ref(false)
const isCenterDialogOpen = ref(false)

function handleOpenCenterDialog() {
  isPopoverOpen.value = false
  isCenterDialogOpen.value = true
}

const isPopoverOpen = computed({
  get: () => {
    return props.isOpen !== undefined ? props.isOpen : isLocalOpen.value
  },
  set: (isNextOpen: boolean) => {
    isLocalOpen.value = isNextOpen
    emit('update:isOpen', isNextOpen)
  },
})

const { t } = useI18n()
const session = useNotifications({ isVisible: () => isPopoverOpen.value })

const MAX_POPOVER_ITEMS = 5

// Thin rendering over the session-owned view (same contract as the center
// dialog): instant feedback with no local optimistic copies.
function handleMarkAsRead(id: string) {
  void session.markAsRead(id).catch(() => {})
}

function handleArchive(id: string) {
  void session.archive(id).catch(() => {})
}

async function handleAcceptInvite(inviteId: string, notificationId?: string): Promise<boolean> {
  return session.acceptInvite(inviteId, notificationId)
}

async function handleDeclineInvite(inviteId: string, notificationId?: string): Promise<boolean> {
  return session.declineInvite(inviteId, notificationId)
}

const sortedNotifications = computed<NotificationDTO[]>(() => {
  const items = [...session.notifications.value]
  return items
    .sort((a, b) => {
      // Unread items come first
      const aUnread = a.status === 'unread' ? 1 : 0
      const bUnread = b.status === 'unread' ? 1 : 0
      if (aUnread !== bUnread) {
        return bUnread - aUnread
      }
      const timeA = a.created_at ? new Date(a.created_at).getTime() : 0
      const timeB = b.created_at ? new Date(b.created_at).getTime() : 0
      return timeB - timeA
    })
    .slice(0, MAX_POPOVER_ITEMS)
})

const shouldShowListSkeleton = computed(
  () => session.isLoading.value && sortedNotifications.value.length === 0,
)
const shouldShowListError = computed(
  () => !session.isLoading.value && session.isError.value && sortedNotifications.value.length === 0,
)

function handleRetryList() {
  void session.refetch()
}
</script>

<style scoped>
.notification-popover-item-move {
  transition: transform var(--motion-control-duration) var(--motion-control-easing);
}

.notification-popover-item-enter-active {
  transition:
    opacity var(--motion-enter-duration) var(--motion-enter-easing),
    transform var(--motion-enter-duration) var(--motion-enter-easing);
}

.notification-popover-item-leave-active {
  position: absolute;
  width: 100%;
  transition:
    opacity var(--motion-exit-duration) var(--motion-exit-easing),
    transform var(--motion-exit-duration) var(--motion-exit-easing);
}

.notification-popover-item-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.notification-popover-item-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

@media (prefers-reduced-motion: reduce) {
  .notification-popover-item-move,
  .notification-popover-item-enter-active,
  .notification-popover-item-leave-active {
    transition: none;
  }

  .notification-popover-item-enter-from,
  .notification-popover-item-leave-to {
    transform: none;
  }
}
</style>
