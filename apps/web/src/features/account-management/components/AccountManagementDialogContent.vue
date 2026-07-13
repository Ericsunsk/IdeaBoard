<template>
  <section :class="mainContentClass" aria-label="账户管理">
    <Button
      type="button"
      text
      rounded
      class="!absolute !top-4 !right-4 !z-20 !flex !h-8 !w-8 !min-w-8 !items-center !justify-center !rounded-[var(--radius-ib-pill)] !border !border-transparent !bg-transparent !p-0 !text-ib-text-muted !shadow-none !transition !duration-[var(--duration-ib-standard)] hover:!border-ib-border-soft hover:!bg-ib-surface-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]"
      aria-label="关闭账户管理"
      @click="emit('close')"
    >
      <Xmark class="h-4 w-4" />
    </Button>

    <aside
      class="app-scrollbar flex min-h-0 flex-col overflow-y-auto bg-ib-surface px-4 py-5 max-[820px]:max-h-[48dvh] max-[820px]:px-4 max-[820px]:py-4"
    >
      <div class="mb-7 flex min-w-0 items-center gap-2.5 px-1" :aria-label="projectName">
        <span :class="accountBrandMarkClass">
          <SiteBrandLogo
            :src="logoURL"
            :fallback-text="fallbackText"
            :class="brandMarkClasses.logo"
          />
        </span>
        <span
          class="min-w-0 truncate text-[19px] leading-[29px] font-bold tracking-normal text-ib-text"
        >
          {{ projectName }}
        </span>
      </div>

      <nav
        class="grid min-w-0 gap-5 max-[820px]:grid-cols-3 max-[820px]:gap-3 max-[640px]:grid-cols-1"
        aria-label="账户管理导航"
      >
        <section
          v-for="group in navGroups"
          :key="group.key"
          class="min-w-0"
          :aria-labelledby="`account-nav-${group.key}`"
        >
          <p
            :id="`account-nav-${group.key}`"
            class="mb-2 px-2 text-[12px] leading-5 font-semibold tracking-normal text-ib-text-subtle"
          >
            {{ group.label }}
          </p>
          <div class="grid gap-1.5 max-[640px]:grid-cols-2">
            <Button
              v-for="item in group.items"
              :key="item.key"
              type="button"
              text
              class="!flex !h-11 !w-full !justify-start !gap-2 !rounded-[var(--radius-ib-control)] !border-0 !px-2.5 !text-[14px] !font-semibold !shadow-none focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]"
              :class="
                activeTab === item.key
                  ? '!bg-ib-creative-accent-soft !text-ib-creative-accent-pressed'
                  : '!bg-transparent !text-ib-text-muted hover:!bg-ib-surface-hover hover:!text-ib-text'
              "
              :aria-current="activeTab === item.key ? 'page' : undefined"
              @click="activeTab = item.key"
            >
              <component :is="item.icon" class="h-4 w-4 shrink-0" />
              <span class="truncate">{{ item.label }}</span>
            </Button>
          </div>
        </section>
      </nav>

      <div class="mt-auto pt-4">
        <Button
          type="button"
          text
          :disabled="logoutLoading"
          class="!flex !h-11 !w-full !justify-start !gap-2 !rounded-[var(--radius-ib-control)] !border-0 !bg-transparent !px-2.5 !text-[14px] !font-semibold !text-ib-danger !shadow-none !transition !duration-[var(--duration-ib-standard)] hover:!bg-ib-danger/5 hover:!text-ib-danger focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-60"
          :aria-label="logoutLoading ? '正在退出登录' : '退出登录'"
          @click="handleLogout"
        >
          <LogOut class="h-4 w-4 shrink-0" />
          <span class="truncate">{{ logoutLoading ? '退出中...' : '退出登录' }}</span>
        </Button>
      </div>
    </aside>

    <main
      :class="[profilePaneClass, { 'is-scrolling': isProfileMainScrolling }]"
      @scroll.passive="handleProfileMainScroll"
    >
      <Transition
        mode="out-in"
        enter-active-class="transition-opacity duration-150 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition-opacity duration-100 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <div v-if="activeTab === 'profile'" key="profile" class="grid gap-7">
          <section class="min-w-0" aria-label="账户资料概览">
            <div class="flex min-w-0 items-center gap-5 max-[520px]:items-start">
              <div
                class="relative size-[88px] shrink-0 overflow-visible rounded-[var(--radius-ib-pill)] bg-ib-surface-raised shadow-[var(--shadow-ib-control)] max-[520px]:size-20"
              >
                <Avatar
                  v-if="userStore.user?.avatar"
                  :image="userStore.user.avatar"
                  shape="circle"
                  class="!h-full !w-full"
                />
                <Avatar
                  v-else
                  :label="userInitial"
                  shape="circle"
                  class="!h-full !w-full !bg-[linear-gradient(135deg,var(--color-ib-creative-accent),var(--color-ib-creative-accent-secondary))] !text-[34px] !font-bold !text-ib-text-inverse max-[520px]:!text-[28px]"
                />
                <Button
                  type="button"
                  text
                  rounded
                  class="!absolute !right-0 !bottom-0 !flex !size-9 !min-w-9 !items-center !justify-center !rounded-[var(--radius-ib-pill)] !border !border-ib-border !bg-ib-surface !p-0 !text-ib-text-muted !shadow-[var(--shadow-ib-control)] !transition !duration-[var(--duration-ib-standard)] hover:!border-ib-border-hover hover:!bg-ib-surface-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-60 max-[520px]:!size-8 max-[520px]:!min-w-8"
                  :disabled="avatarUploading"
                  aria-label="更换头像"
                  title="更换头像"
                  @click="openAvatarPicker"
                >
                  <Camera class="h-4 w-4" />
                </Button>
                <input
                  ref="avatarInputRef"
                  class="hidden"
                  type="file"
                  accept="image/*"
                  :disabled="avatarUploading"
                  @change="onAvatarFileChange"
                />
              </div>

              <div class="min-w-0">
                <p
                  class="m-0 truncate text-[30px] leading-tight font-bold tracking-normal text-ib-text max-[520px]:text-[24px]"
                >
                  {{ userStore.user?.username }}
                </p>
                <p
                  class="mt-1 mb-0 truncate font-mono text-[14px] leading-6 font-semibold text-ib-text-subtle"
                >
                  {{ userStore.user?.uid || '-' }}
                </p>
                <div class="mt-3 flex min-w-0 flex-wrap items-center gap-2">
                  <Tag
                    :value="userStatusLabel"
                    :severity="userStatusSeverity"
                    rounded
                    class="!bg-ib-surface !px-2.5 !py-1 !text-[13px] !font-semibold"
                  />
                  <Tag
                    :value="userRoleLabel"
                    :severity="userRoleSeverity"
                    rounded
                    class="!bg-ib-surface !px-2.5 !py-1 !text-[13px] !font-semibold"
                  />
                </div>
              </div>
            </div>
          </section>

          <section class="min-w-0" aria-label="账号信息">
            <dl class="m-0 border-y border-ib-border-soft">
              <div
                v-for="field in profileFields"
                :key="field.label"
                class="grid min-h-14 grid-cols-[148px_minmax(0,1fr)] items-center gap-4 border-b border-ib-border-soft py-3 last:border-b-0 max-[640px]:grid-cols-1 max-[640px]:gap-1"
              >
                <dt class="text-[13px] font-semibold text-ib-text-subtle">
                  {{ field.label }}
                </dt>
                <dd class="m-0 min-w-0">
                  <span
                    class="block truncate text-[15px] font-semibold text-ib-text"
                    :class="field.mono ? 'font-mono text-[14px]' : ''"
                    :title="field.value"
                  >
                    {{ field.value }}
                  </span>
                </dd>
              </div>
            </dl>
          </section>
        </div>

        <UserCreditsPane
          v-else-if="activeTab === 'billing'"
          key="billing"
          :active="activeTab === 'billing'"
        />

        <UserUsagePane
          v-else-if="activeTab === 'usage'"
          key="usage"
          :active="activeTab === 'usage'"
        />

        <div v-else-if="activeTab === 'security'" key="security">
          <form class="grid gap-5" novalidate @submit.prevent="handleUpdatePassword">
            <label :class="passwordFieldClass">
              <span>当前原密码</span>
              <Password
                v-model="pwdForm.oldPassword"
                autocomplete="current-password"
                :feedback="false"
                fluid
                unstyled
                :pt="ibPasswordPt"
                :input-props="{
                  autocomplete: 'current-password',
                  'aria-describedby': getPasswordDescribedby(
                    'oldPassword',
                    'profile-old-password-error',
                  ),
                }"
                :invalid="Boolean(pwdErrors.oldPassword)"
                :input-class="[
                  passwordInputClass,
                  isPasswordInlineError('oldPassword') ? passwordInputErrorClass : '',
                ]"
                :placeholder="
                  isPasswordInlineError('oldPassword')
                    ? displayErrorMessage(pwdErrors.oldPassword)
                    : '请输入当前使用的密码'
                "
                toggle-mask
                @blur="validatePasswordField('oldPassword')"
                @update:model-value="clearPasswordFieldError('oldPassword')"
              />
              <Message
                v-if="shouldShowPasswordMessage('oldPassword')"
                id="profile-old-password-error"
                unstyled
                :pt="ibFormErrorMessagePt"
                severity="error"
                size="small"
                :closable="false"
              >
                {{ displayErrorMessage(pwdErrors.oldPassword) }}
              </Message>
            </label>

            <div class="grid grid-cols-2 gap-5 max-[720px]:grid-cols-1">
              <label :class="passwordFieldClass">
                <span>设置新密码</span>
                <Password
                  v-model="pwdForm.newPassword"
                  autocomplete="new-password"
                  :feedback="false"
                  fluid
                  unstyled
                  :pt="ibPasswordPt"
                  :input-props="{
                    autocomplete: 'new-password',
                    'aria-describedby': getPasswordDescribedby(
                      'newPassword',
                      'profile-new-password-error',
                    ),
                  }"
                  :invalid="Boolean(pwdErrors.newPassword)"
                  :input-class="[
                    passwordInputClass,
                    isPasswordInlineError('newPassword') ? passwordInputErrorClass : '',
                  ]"
                  :placeholder="
                    isPasswordInlineError('newPassword')
                      ? displayErrorMessage(pwdErrors.newPassword)
                      : '不少于6位字符'
                  "
                  toggle-mask
                  @blur="validatePasswordField('newPassword')"
                  @update:model-value="handleNewPasswordInput"
                />
                <Message
                  v-if="shouldShowPasswordMessage('newPassword')"
                  id="profile-new-password-error"
                  unstyled
                  :pt="ibFormErrorMessagePt"
                  severity="error"
                  size="small"
                  :closable="false"
                >
                  {{ displayErrorMessage(pwdErrors.newPassword) }}
                </Message>
              </label>

              <label :class="passwordFieldClass">
                <span>重复新密码</span>
                <Password
                  v-model="pwdForm.confirmPassword"
                  autocomplete="new-password"
                  :feedback="false"
                  fluid
                  unstyled
                  :pt="ibPasswordPt"
                  :input-props="{
                    autocomplete: 'new-password',
                    'aria-describedby': getPasswordDescribedby(
                      'confirmPassword',
                      'profile-confirm-password-error',
                    ),
                  }"
                  :invalid="Boolean(pwdErrors.confirmPassword)"
                  :input-class="[
                    passwordInputClass,
                    isPasswordInlineError('confirmPassword') ? passwordInputErrorClass : '',
                  ]"
                  :placeholder="
                    isPasswordInlineError('confirmPassword')
                      ? displayErrorMessage(pwdErrors.confirmPassword)
                      : '请再次输入新密码'
                  "
                  toggle-mask
                  @blur="validatePasswordField('confirmPassword')"
                  @update:model-value="clearPasswordFieldError('confirmPassword')"
                />
                <Message
                  v-if="shouldShowPasswordMessage('confirmPassword')"
                  id="profile-confirm-password-error"
                  unstyled
                  :pt="ibFormErrorMessagePt"
                  severity="error"
                  size="small"
                  :closable="false"
                >
                  {{ displayErrorMessage(pwdErrors.confirmPassword) }}
                </Message>
              </label>
            </div>

            <Button
              type="submit"
              :label="pwdLoading ? '更新中...' : '确认更新密码'"
              :loading="pwdLoading"
              :disabled="pwdLoading"
              class="!h-12 !w-full !rounded-[var(--radius-ib-panel)] !border-0 !bg-ib-creative-accent-hover !text-[15px] !font-semibold !text-ib-text-inverse !shadow-[var(--shadow-ib-control)] transition hover:!-translate-y-0.5 hover:!bg-ib-creative-accent-pressed hover:!shadow-[var(--shadow-ib-panel)] active:!translate-y-0 focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-70"
            />
          </form>
        </div>

        <AccountComingSoonPane
          v-else-if="upcomingPage"
          :key="activeTab"
          :category="upcomingPage.category"
          :title="upcomingPage.title"
          :icon="upcomingPage.icon"
        />
      </Transition>
    </main>
  </section>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import { computed, onMounted, onUnmounted, ref, watch, type Component } from 'vue'
import { useAvatarUploadWorkflow } from '../useAvatarUploadWorkflow'
import { usePasswordChangeWorkflow } from '../usePasswordChangeWorkflow'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Tag from 'primevue/tag'
import {
  Camera,
  Coins,
  CreditCard,
  Gift,
  Lock,
  LogOut,
  Page,
  StatsUpSquare,
  User,
  Xmark,
} from '@iconoir/vue'
import SiteBrandLogo from '../../../shared/components/SiteBrandLogo.vue'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { USER_HOME_PATH } from '../../../shared/constants/routePaths'
import { brandMarkClasses, createBrandMarkClass } from '../../../shared/lib/brandMark'
import {
  ibFormErrorMessagePt,
  ibFormFieldClass,
  ibFormInputClass,
  ibFormInputErrorClass,
  ibPasswordPt,
} from '../../../shared/theme/formControls'
import { useProfileFeedback } from '../useProfileFeedback'
import AccountComingSoonPane from './AccountComingSoonPane.vue'
import UserCreditsPane from './UserCreditsPane.vue'
import UserUsagePane from './UserUsagePane.vue'
import type { AccountManagementTab } from '../useAccountManagementDialogController'

type NavItem = {
  key: AccountManagementTab
  label: string
  icon: Component
}
type NavGroup = {
  key: 'account' | 'subscription' | 'benefits'
  label: string
  items: NavItem[]
}
type UpcomingTab = Extract<AccountManagementTab, 'recharge' | 'subscriptions' | 'rewards'>
type UpcomingPage = {
  category: string
  title: string
  icon: Component
}

type TagSeverity = 'success' | 'info' | 'warn' | 'danger' | 'secondary'
type ProfileField = {
  label: string
  value: string
  mono?: boolean
}

const props = withDefaults(defineProps<{ initialTab?: AccountManagementTab }>(), {
  initialTab: 'profile',
})
const emit = defineEmits<{
  close: []
}>()
const userStore = useUserStore()
const { fallbackText, logoURL, projectName } = useSiteBranding()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const profileFeedback = useProfileFeedback()
const {
  form: pwdForm,
  errors: pwdErrors,
  loading: pwdLoading,
  validateField: validatePasswordField,
  clearFieldError: clearPasswordFieldError,
  handleNewPasswordInput,
  submit: handleUpdatePassword,
  isInlineError: isPasswordInlineError,
  shouldShowMessage: shouldShowPasswordMessage,
  getDescribedby: getPasswordDescribedby,
  displayErrorMessage,
} = usePasswordChangeWorkflow()
const { uploading: avatarUploading, handleFileChange: onAvatarFileChange } =
  useAvatarUploadWorkflow()

const activeTab = ref<AccountManagementTab>(props.initialTab)
watch(
  () => props.initialTab,
  (tab) => {
    activeTab.value = tab
  },
)
const avatarInputRef = ref<HTMLInputElement | null>(null)
const isProfileMainScrolling = ref(false)
const logoutLoading = ref(false)
let profileMainScrollTimer: number | undefined

const navGroups = [
  {
    key: 'account',
    label: '账户设置',
    items: [
      { key: 'profile', label: '个人资料', icon: User },
      { key: 'security', label: '账号安全', icon: Lock },
    ],
  },
  {
    key: 'subscription',
    label: '订阅与套餐',
    items: [
      { key: 'recharge', label: '充值积分', icon: Coins },
      { key: 'subscriptions', label: '订阅套餐', icon: CreditCard },
    ],
  },
  {
    key: 'benefits',
    label: '权益和账单',
    items: [
      { key: 'rewards', label: '奖励中心', icon: Gift },
      { key: 'billing', label: '账单记录', icon: Page },
      { key: 'usage', label: '用量统计', icon: StatsUpSquare },
    ],
  },
] satisfies NavGroup[]

const upcomingPages: Record<UpcomingTab, UpcomingPage> = {
  recharge: { category: '订阅与套餐', title: '充值积分', icon: Coins },
  subscriptions: { category: '订阅与套餐', title: '订阅套餐', icon: CreditCard },
  rewards: { category: '权益和账单', title: '奖励中心', icon: Gift },
}

const upcomingPage = computed(() =>
  isUpcomingTab(activeTab.value) ? upcomingPages[activeTab.value] : null,
)

const mainContentClass =
  'relative grid h-[min(78dvh,720px)] min-h-0 grid-cols-[234px_minmax(0,1fr)] overflow-hidden bg-ib-surface-raised max-[820px]:h-[calc(100dvh-64px)] max-[820px]:grid-cols-1 max-[820px]:grid-rows-[auto_minmax(0,1fr)]'
const profilePaneClass =
  'app-scrollbar min-h-0 overflow-x-hidden overflow-y-auto rounded-l-[var(--radius-ib-overlay)] bg-ib-surface-muted px-8 py-7 pr-14 max-[820px]:rounded-l-none max-[820px]:rounded-t-[var(--radius-ib-overlay)] max-[820px]:px-4 max-[820px]:py-5 max-[820px]:pr-14'
const accountBrandMarkClass = createBrandMarkClass(brandMarkClasses.sizes.account)
const passwordFieldClass = ibFormFieldClass
const passwordInputClass = ibFormInputClass
const passwordInputErrorClass = ibFormInputErrorClass

function isUpcomingTab(tab: AccountManagementTab): tab is UpcomingTab {
  return tab in upcomingPages
}

const formattedJoinedAt = computed(() => formatDateWithDots(userStore.user?.created_at))
const formattedUpdatedAt = computed(() => formatDateWithDots(userStore.user?.updated_at))
const userStatusLabel = computed(() => (userStore.user?.status === 1 ? '正常使用中' : '已封禁'))
const userStatusSeverity = computed<TagSeverity>(() =>
  userStore.user?.status === 1 ? 'success' : 'danger',
)
const userRoleLabel = computed(() => (userStore.user?.role === 'admin' ? '管理员' : '创作者'))
const userRoleSeverity = computed<TagSeverity>(() =>
  userStore.user?.role === 'admin' ? 'info' : 'secondary',
)
const userInitial = computed(() => {
  const username = userStore.user?.username?.trim()
  return username?.[0]?.toUpperCase() || 'U'
})
const profileFields = computed<ProfileField[]>(() => [
  {
    label: '登录账号',
    value: userStore.user?.username || '-',
  },
  {
    label: '认证 ID',
    value: userStore.user?.auth_user_id?.trim() || '-',
    mono: true,
  },
  {
    label: '加入时间',
    value: formattedJoinedAt.value,
  },
  {
    label: '最近更新',
    value: formattedUpdatedAt.value,
  },
])

function formatDateWithDots(value?: string) {
  const datePart = value?.trim().split(/[T\s]/)[0]
  if (!datePart) return '-'
  const [year, month, day] = datePart.split('-')
  if (!year || !month || !day) return datePart.replaceAll('-', '.')
  return `${year}.${month}.${day}`
}

function openAvatarPicker() {
  avatarInputRef.value?.click()
}

function handleProfileMainScroll() {
  isProfileMainScrolling.value = true
  if (profileMainScrollTimer) window.clearTimeout(profileMainScrollTimer)
  profileMainScrollTimer = window.setTimeout(() => {
    isProfileMainScrolling.value = false
    profileMainScrollTimer = undefined
  }, 700)
}

async function handleLogout() {
  if (logoutLoading.value) return

  logoutLoading.value = true
  try {
    if (await logoutAndRedirect(USER_HOME_PATH)) {
      emit('close')
    } else {
      profileFeedback.logoutFailed()
    }
  } finally {
    logoutLoading.value = false
  }
}

onMounted(() => {
  void userStore.fetchUserInfo()
})

onUnmounted(() => {
  if (profileMainScrollTimer) {
    window.clearTimeout(profileMainScrollTimer)
    profileMainScrollTimer = undefined
  }
})
</script>
