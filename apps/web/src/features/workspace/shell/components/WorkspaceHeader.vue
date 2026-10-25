<template>
  <header
    :class="[
      'z-admin-overlay flex flex-wrap items-center justify-between gap-x-4 pointer-events-none pb-2',
      sticky ? 'sticky top-0' : 'relative',
    ]"
  >
    <div class="pointer-events-auto inline-flex min-w-0 max-w-[calc(100%-64px)] items-center">
      <slot name="brand">
        <StudioName />
      </slot>
    </div>

    <slot name="navigation">
      <WorkspaceNavigation class="pointer-events-auto" :navigation="props.navigation" />
    </slot>

    <slot name="account">
      <UserAccountMenu
        class="pointer-events-auto"
        :avatar-url="accountControls.viewModel.avatarUrl.value"
        :user-email="accountControls.viewModel.userEmail.value"
        :balance="accountControls.viewModel.balance.value"
        :items="accountControls.viewModel.menuItems.value"
        :support-channels="supportChannels"
        :workspace="workspaceSwitcher.menuProps.value"
      >
        <template #actions>
          <NotificationBell v-if="Boolean(userStore.user)" />
        </template>
      </UserAccountMenu>
    </slot>
  </header>
</template>

<script setup lang="ts">
import { defineAsyncComponent } from 'vue'
import { useRouter } from 'vue-router'
import StudioName from '../../../../core/components/StudioName.vue'
import UserAccountMenu from '../../../account/ui/chrome/UserAccountMenu.vue'
import { useSupportChannels } from '../../../../core/composables/useSupportChannels'
import { HOME_PATH } from '../../routePaths'
import { useLogoutRedirect } from '../../../../core/composables/useLogoutRedirect'
import { useUserStore } from '../../../auth'
import WorkspaceNavigation from './WorkspaceNavigation.vue'
import {
  useWorkspaceAccountControls,
  type WorkspaceNavigationAdapter,
} from '../useWorkspaceAccountControls'
import { useWorkspaceSwitcher } from '../useWorkspaceSwitcher'

interface WorkspaceHeaderProps {
  sticky?: boolean
  navigation?: WorkspaceNavigationAdapter | null
}

const props = withDefaults(defineProps<WorkspaceHeaderProps>(), {
  sticky: true,
  navigation: null,
})

const NotificationBell = defineAsyncComponent(
  () => import('../../../notification/ui/NotificationBell.vue'),
)

const router = useRouter()
const { channels: supportChannels } = useSupportChannels()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })

const accountControls = useWorkspaceAccountControls({
  navigation: props.navigation ?? {
    navigate: (path) => router.push(path),
    logout: () => logoutAndRedirect(HOME_PATH),
  },
})
const workspaceSwitcher = useWorkspaceSwitcher()
</script>
