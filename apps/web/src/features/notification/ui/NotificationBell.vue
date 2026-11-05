<template>
  <NotificationPopover>
    <template #trigger>
      <button
        type="button"
        :class="bellButtonClass"
        :aria-label="accessibleAriaLabel"
        :title="accessibleAriaLabel"
        data-testid="notification-bell-btn"
      >
        <Bell :class="pageChromeClasses.icon" />
        <span
          v-if="session.hasUnread.value"
          class="absolute top-1.5 right-1.5 size-2 rounded-pill bg-destructive ring-2 ring-background"
          data-testid="notification-unread-badge"
        />
      </button>
    </template>
  </NotificationPopover>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Bell } from '@iconoir/vue'
import { pageChromeClasses } from '@/core/lib/pageChrome'
import { cn } from '@/lib/utils'
import NotificationPopover from './NotificationPopover.vue'
import { useNotifications } from '../session/useNotifications'

const { t } = useI18n()
const session = useNotifications({ isVisible: false })

const bellButtonClass = cn(
  pageChromeClasses.roundControl,
  'page-chrome-icon-button',
  'hit-target-32',
  'relative',
)

const accessibleAriaLabel = computed(() => {
  const count = session.unreadCount.value
  if (count > 0) {
    return t('notification.bell.ariaLabelUnread', { count })
  }
  return t('notification.bell.ariaLabel')
})
</script>
