<template>
  <Dialog v-model:open="isDialogOpen">
    <DialogPortal>
      <DialogOverlay />
      <DialogContent
        class="w-[min(38rem,calc(100vw-2rem))] max-w-[38rem] h-[560px] max-h-[82vh] flex flex-col p-0 overflow-hidden gap-0 rounded-overlay border border-border !bg-background shadow-floating"
        data-testid="notification-center-dialog"
      >
        <!-- Dialog Header -->
        <DialogHeader class="px-6 pt-5 pb-3 flex flex-row items-center justify-between space-y-0">
          <div class="flex items-center gap-2">
            <DialogTitle class="text-sm font-semibold text-foreground">
              {{ t('notification.modal.title') }}
            </DialogTitle>
            <span
              v-if="session.unreadCount.value > 0"
              class="flex items-center justify-center min-w-4 h-4 px-1 rounded-pill bg-destructive text-destructive-foreground text-admin-caption font-bold tabular-nums"
              data-testid="dialog-unread-badge"
            >
              {{ session.unreadCount.value }}
            </span>
          </div>

          <!-- Actions area -->
          <div class="flex items-center gap-2 mr-6">
            <button
              type="button"
              class="hit-target-32-inline text-xs font-medium text-muted-foreground hover:text-foreground disabled:opacity-40 disabled:pointer-events-none transition-colors px-2 py-1 rounded-control hover:bg-muted cursor-pointer"
              data-testid="mark-all-read-btn"
              :disabled="session.unreadCount.value === 0 || session.isMarkingAllRead.value"
              @click="handleMarkAllRead"
            >
              {{ t('notification.modal.markAllRead') }}
            </button>
            <button
              type="button"
              class="hit-target-32-inline text-xs font-medium text-muted-foreground hover:text-foreground disabled:opacity-40 disabled:pointer-events-none transition-colors px-2 py-1 rounded-control hover:bg-muted cursor-pointer"
              data-testid="clear-read-btn"
              :disabled="!hasReadNotifications || session.isArchiving.value"
              @click="handleClearRead"
            >
              {{ t('notification.modal.clearRead') }}
            </button>
          </div>
        </DialogHeader>
        <DialogDescription class="sr-only">
          {{ t('notification.modal.description') }}
        </DialogDescription>

        <!-- Minimalist Tabs Navigation -->
        <Tabs v-model="activeTab" class="flex-1 flex flex-col min-h-0">
          <div class="px-6 border-b border-border overflow-x-auto">
            <TabsList
              class="flex items-center gap-6 bg-transparent p-0 h-auto border-0 rounded-none shadow-none"
            >
              <TabsTrigger
                value="all"
                data-testid="tab-all"
                class="relative shrink-0 hit-target-32-inline pb-2.5 pt-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground data-[state=active]:text-foreground data-[state=active]:font-semibold bg-transparent shadow-none border-0 rounded-none cursor-pointer select-none"
                @click="activeTab = 'all'"
              >
                <span>{{ t('notification.modal.tabs.all') }}</span>
                <span
                  v-if="activeTab === 'all'"
                  class="absolute bottom-0 inset-x-0 h-0.5 bg-foreground"
                />
              </TabsTrigger>
              <TabsTrigger
                value="unread"
                data-testid="tab-unread"
                class="relative shrink-0 hit-target-32-inline pb-2.5 pt-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground data-[state=active]:text-foreground data-[state=active]:font-semibold bg-transparent shadow-none border-0 rounded-none cursor-pointer select-none"
                @click="activeTab = 'unread'"
              >
                <span>{{ t('notification.modal.tabs.unread') }}</span>
                <span
                  v-if="session.unreadCount.value > 0"
                  class="ml-1 inline-flex min-h-4 min-w-4 items-center justify-center rounded-pill bg-destructive px-1 text-admin-caption font-bold tabular-nums text-destructive-foreground"
                  data-testid="tab-unread-count"
                >
                  {{ session.unreadCount.value > 99 ? '99+' : session.unreadCount.value }}
                </span>
                <span
                  v-if="activeTab === 'unread'"
                  class="absolute bottom-0 inset-x-0 h-0.5 bg-foreground"
                />
              </TabsTrigger>
              <TabsTrigger
                value="invites"
                data-testid="tab-invites"
                class="relative shrink-0 hit-target-32-inline pb-2.5 pt-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground data-[state=active]:text-foreground data-[state=active]:font-semibold bg-transparent shadow-none border-0 rounded-none cursor-pointer select-none"
                @click="activeTab = 'invites'"
              >
                <span>{{ t('notification.modal.tabs.invites') }}</span>
                <span
                  v-if="activeTab === 'invites'"
                  class="absolute bottom-0 inset-x-0 h-0.5 bg-foreground"
                />
              </TabsTrigger>
              <TabsTrigger
                value="billing"
                data-testid="tab-billing"
                class="relative shrink-0 hit-target-32-inline pb-2.5 pt-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground data-[state=active]:text-foreground data-[state=active]:font-semibold bg-transparent shadow-none border-0 rounded-none cursor-pointer select-none"
                @click="activeTab = 'billing'"
              >
                <span>{{ t('notification.modal.tabs.billing') }}</span>
                <span
                  v-if="activeTab === 'billing'"
                  class="absolute bottom-0 inset-x-0 h-0.5 bg-foreground"
                />
              </TabsTrigger>
              <TabsTrigger
                value="archived"
                data-testid="tab-archived"
                class="relative shrink-0 hit-target-32-inline pb-2.5 pt-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground data-[state=active]:text-foreground data-[state=active]:font-semibold bg-transparent shadow-none border-0 rounded-none cursor-pointer select-none"
                @click="activeTab = 'archived'"
              >
                <span>{{ t('notification.modal.tabs.archived') }}</span>
                <span
                  v-if="activeTab === 'archived'"
                  class="absolute bottom-0 inset-x-0 h-0.5 bg-foreground"
                />
              </TabsTrigger>
            </TabsList>
          </div>

          <!-- Scrollable list content -->
          <div class="flex-1 min-h-0 overflow-y-auto" data-testid="notification-center-content">
            <template v-if="shouldShowDialogSkeleton">
              <div
                class="divide-y divide-border"
                data-testid="notification-center-skeleton"
                role="status"
                :aria-label="t('common.actions.loading')"
              >
                <div
                  v-for="index in 4"
                  :key="index"
                  class="flex items-start gap-3.5 px-6 py-3"
                  aria-hidden="true"
                >
                  <Skeleton class="size-5 shrink-0" />
                  <div class="min-w-0 flex-1 space-y-2 pt-0.5">
                    <Skeleton height="12px" width="45%" />
                    <Skeleton height="12px" width="85%" />
                  </div>
                </div>
              </div>
            </template>
            <template v-else-if="shouldShowDialogError">
              <div
                class="flex flex-col items-center justify-center gap-2 px-6 py-20 text-center"
                data-testid="notification-center-error"
                aria-live="polite"
              >
                <button
                  type="button"
                  class="inline-flex min-h-8 cursor-pointer items-center justify-center rounded-control px-3 text-xs font-medium text-primary transition-colors hover:bg-muted"
                  data-testid="notification-center-retry-btn"
                  @click="handleRetryList"
                >
                  {{ t('common.actions.retry') }}
                </button>
              </div>
            </template>
            <template v-else-if="session.notifications.value.length > 0">
              <!-- Today Group -->
              <div v-if="groupedNotifications.today.length > 0">
                <div
                  class="px-6 pt-4 pb-1 text-admin-micro font-medium uppercase tracking-wider text-muted-foreground select-none"
                  data-testid="timeline-group-today"
                >
                  {{ t('notification.modal.groups.today') }}
                </div>
                <TransitionGroup
                  name="notification-dialog-item"
                  tag="div"
                  class="relative divide-y divide-border"
                >
                  <NotificationItem
                    v-for="item in groupedNotifications.today"
                    :key="item.id"
                    :notification="item"
                    :on-accept-invite="handleAcceptInvite"
                    :on-decline-invite="handleDeclineInvite"
                    @read="handleMarkAsRead"
                    @archive="handleArchive"
                  />
                </TransitionGroup>
              </div>

              <!-- Yesterday Group -->
              <div v-if="groupedNotifications.yesterday.length > 0">
                <div
                  class="px-6 pt-4 pb-1 text-admin-micro font-medium uppercase tracking-wider text-muted-foreground select-none"
                  data-testid="timeline-group-yesterday"
                >
                  {{ t('notification.modal.groups.yesterday') }}
                </div>
                <TransitionGroup
                  name="notification-dialog-item"
                  tag="div"
                  class="relative divide-y divide-border"
                >
                  <NotificationItem
                    v-for="item in groupedNotifications.yesterday"
                    :key="item.id"
                    :notification="item"
                    :on-accept-invite="handleAcceptInvite"
                    :on-decline-invite="handleDeclineInvite"
                    @read="handleMarkAsRead"
                    @archive="handleArchive"
                  />
                </TransitionGroup>
              </div>

              <!-- Earlier Group -->
              <div v-if="groupedNotifications.earlier.length > 0">
                <div
                  class="px-6 pt-4 pb-1 text-admin-micro font-medium uppercase tracking-wider text-muted-foreground select-none"
                  data-testid="timeline-group-earlier"
                >
                  {{ t('notification.modal.groups.earlier') }}
                </div>
                <TransitionGroup
                  name="notification-dialog-item"
                  tag="div"
                  class="relative divide-y divide-border"
                >
                  <NotificationItem
                    v-for="item in groupedNotifications.earlier"
                    :key="item.id"
                    :notification="item"
                    :on-accept-invite="handleAcceptInvite"
                    :on-decline-invite="handleDeclineInvite"
                    @read="handleMarkAsRead"
                    @archive="handleArchive"
                  />
                </TransitionGroup>
              </div>
            </template>

            <!-- Distinct Empty State per tab -->
            <div
              v-else
              class="flex flex-col items-center justify-center py-20 px-6 text-center text-muted-foreground"
              :data-testid="`empty-${activeTab}`"
            >
              <component
                :is="emptyStateConfig.icon"
                class="size-8 stroke-1 text-muted-foreground mb-2.5"
              />
              <p class="text-xs font-medium text-foreground" data-testid="empty-title">
                {{ t(emptyStateConfig.titleKey) }}
              </p>
              <p
                class="text-admin-micro text-muted-foreground mt-1 max-w-xs"
                data-testid="empty-desc"
              >
                {{ t(emptyStateConfig.descKey) }}
              </p>
            </div>
            <div
              v-if="session.hasNextPage.value"
              class="flex flex-col items-center gap-2 px-6 py-4"
            >
              <p
                v-if="session.isFetchNextPageError.value"
                class="text-xs text-muted-foreground"
                role="alert"
              >
                {{ t('notification.modal.loadMoreFailed') }}
              </p>
              <Button
                variant="outline"
                size="sm"
                data-testid="notification-load-more"
                :disabled="session.isFetchingNextPage.value"
                @click="session.fetchNextPage()"
              >
                {{
                  session.isFetchingNextPage.value
                    ? t('common.actions.loading')
                    : t('notification.modal.loadMore')
                }}
              </Button>
            </div>
          </div>
        </Tabs>
      </DialogContent>
    </DialogPortal>
  </Dialog>

  <!-- Clear-read confirmation: bulk archive with the read count and a pointer
    to the Archived tab, where the archived items can be found again. -->
  <UserConfirmDialog
    :is-visible="isClearReadConfirmVisible"
    :title="t('notification.modal.clearReadConfirmTitle')"
    :message="t('notification.modal.clearReadConfirmDesc', { count: readItemsCount })"
    :confirm-label="t('notification.modal.clearReadConfirmAction')"
    :is-loading="session.isArchiving.value"
    @update:is-visible="isClearReadConfirmVisible = $event"
    @confirm="handleConfirmClearRead"
  />
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Archive, Bell, CheckCircle, Coins, Mail } from '@iconoir/vue'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
} from '@/components/ui/dialog'
import Tabs from '@/components/ui/tabs/Tabs.vue'
import TabsList from '@/components/ui/tabs/TabsList.vue'
import TabsTrigger from '@/components/ui/tabs/TabsTrigger.vue'
import { Skeleton } from '@/components/ui/skeleton'
import UserConfirmDialog from '../../../core/components/UserConfirmDialog.vue'
import type { NotificationDTO } from '@/api/generated-query/public/model'
import NotificationItem from './NotificationItem.vue'
import { useNotifications } from '../session/useNotifications'
import { useFailureToast } from '../../../core/composables/useFailureToast'
import { getTimelineGroup, type TimelineGroupKey } from './notificationCenter'
import type { NotificationCenterTab } from '../session/readFilters'
import { Button } from '@/components/ui/button'

interface NotificationCenterDialogProps {
  isOpen?: boolean | undefined
  initialTab?: NotificationCenterTab | undefined
}

const props = withDefaults(defineProps<NotificationCenterDialogProps>(), {
  isOpen: false,
  initialTab: 'all',
})

const emit = defineEmits<{
  'update:isOpen': [isNextOpen: boolean]
}>()

const isDialogOpen = computed({
  get: () => props.isOpen,
  set: (isNextOpen: boolean) => {
    emit('update:isOpen', isNextOpen)
  },
})

const { t } = useI18n()
const activeTab = ref<NotificationCenterTab>(props.initialTab)
const session = useNotifications({ tab: activeTab, isVisible: () => props.isOpen })
const isClearReadConfirmVisible = ref(false)

// Render the final session projection; status, decisions and feed filtering
// are owned by the session.
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

async function handleMarkAllRead() {
  await session.markAllAsRead().catch(() => {})
}

async function handleClearRead() {
  if (readItemsCount.value === 0 || session.isArchiving.value) return
  isClearReadConfirmVisible.value = true
}

async function handleConfirmClearRead() {
  isClearReadConfirmVisible.value = false
  await session.archiveRead()
}

function handleRetryList() {
  void session.refetch()
}

const hasReadNotifications = computed<boolean>(() => {
  return session.notifications.value.some((n) => n.status === 'read')
})

const readItemsCount = computed<number>(() => {
  return session.notifications.value.filter((n) => n.status === 'read').length
})

const shouldShowDialogSkeleton = computed<boolean>(() => {
  return session.isLoading.value && session.notifications.value.length === 0
})

const shouldShowDialogError = computed<boolean>(() => {
  return (
    !session.isLoading.value && session.isError.value && session.notifications.value.length === 0
  )
})

// Only initial-load failures toast; pagination retains its inline hint.
useFailureToast(shouldShowDialogError, () => t('notification.modal.loadFailed'), {
  shouldFireOnMount: false,
})

const groupedNotifications = computed<Record<TimelineGroupKey, NotificationDTO[]>>(() => {
  const groups: Record<TimelineGroupKey, NotificationDTO[]> = {
    today: [],
    yesterday: [],
    earlier: [],
  }

  // Sort descending by created_at
  const sorted = [...session.notifications.value].sort((a, b) => {
    const timeA = a.created_at ? new Date(a.created_at).getTime() : 0
    const timeB = b.created_at ? new Date(b.created_at).getTime() : 0
    return timeB - timeA
  })

  for (const item of sorted) {
    const group = getTimelineGroup(item.created_at)
    groups[group].push(item)
  }

  return groups
})

const emptyStateConfig = computed(() => {
  switch (activeTab.value) {
    case 'unread':
      return {
        icon: CheckCircle,
        titleKey: 'notification.modal.empty.unreadTitle',
        descKey: 'notification.modal.empty.unreadDesc',
      }
    case 'invites':
      return {
        icon: Mail,
        titleKey: 'notification.modal.empty.invitesTitle',
        descKey: 'notification.modal.empty.invitesDesc',
      }
    case 'billing':
      return {
        icon: Coins,
        titleKey: 'notification.modal.empty.billingTitle',
        descKey: 'notification.modal.empty.billingDesc',
      }
    case 'archived':
      return {
        icon: Archive,
        titleKey: 'notification.modal.empty.archivedTitle',
        descKey: 'notification.modal.empty.archivedDesc',
      }
    case 'all':
    default:
      return {
        icon: Bell,
        titleKey: 'notification.modal.empty.allTitle',
        descKey: 'notification.modal.empty.allDesc',
      }
  }
})
</script>

<style scoped>
.notification-dialog-item-move {
  transition: transform var(--motion-control-duration) var(--motion-control-easing);
}

.notification-dialog-item-enter-active {
  transition:
    opacity var(--motion-enter-duration) var(--motion-enter-easing),
    transform var(--motion-enter-duration) var(--motion-enter-easing);
}

.notification-dialog-item-leave-active {
  position: absolute;
  width: 100%;
  transition:
    opacity var(--motion-exit-duration) var(--motion-exit-easing),
    transform var(--motion-exit-duration) var(--motion-exit-easing);
}

.notification-dialog-item-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.notification-dialog-item-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

@media (prefers-reduced-motion: reduce) {
  .notification-dialog-item-move,
  .notification-dialog-item-enter-active,
  .notification-dialog-item-leave-active {
    transition: none;
  }

  .notification-dialog-item-enter-from,
  .notification-dialog-item-leave-to {
    transform: none;
  }
}
</style>
