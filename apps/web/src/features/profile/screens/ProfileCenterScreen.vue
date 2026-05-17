<template>
  <div class="center-page">
    <div class="mesh-bg">
      <div class="mesh-blob blob-1"></div>
      <div class="mesh-blob blob-2"></div>
    </div>

    <div class="center-container">
      <aside class="center-sidebar">
        <div class="sidebar-header">
          <div class="logo-text" @click="router.push('/user/home')">IdeaBoard</div>
        </div>

        <nav class="sidebar-nav">
          <div
            v-for="item in navItems"
            :key="item.key"
            class="nav-item"
            :class="{ active: activeTab === item.key }"
            @click="activeTab = item.key"
          >
            <div class="nav-icon">
              <svg
                width="20"
                height="20"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
              >
                <component
                  :is="shape.tag"
                  v-for="(shape, shapeIndex) in item.shapes"
                  :key="`${item.key}-${shapeIndex}`"
                  v-bind="shape.attrs"
                />
              </svg>
            </div>
            <span>{{ item.label }}</span>
          </div>
        </nav>

        <div class="sidebar-footer">
          <button
            v-if="userStore.user?.role === 'admin'"
            class="action-btn admin-btn"
            @click="router.push('/admin/users')"
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
            </svg>
            管理后台
          </button>

          <button class="action-btn back-btn" @click="router.back()">
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
            >
              <polyline points="15 18 9 12 15 6" />
            </svg>
            返回上级
          </button>

          <button class="action-btn logout-btn" @click="handleLogout">
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
              <polyline points="16 17 21 12 16 7" />
              <line x1="21" y1="12" x2="9" y2="12" />
            </svg>
            退出登录
          </button>
        </div>
      </aside>

      <main class="center-main">
        <div class="main-header">
          <div class="user-brief">
            <div class="avatar-big">
              <div class="avatar-face">
                <img v-if="userStore.user?.avatar" :src="userStore.user.avatar" />
                <div v-else class="avatar-placeholder">
                  {{ userStore.user?.username?.[0]?.toUpperCase() }}
                </div>
              </div>
              <button
                class="avatar-edit-btn"
                :disabled="avatarUploading"
                title="更换头像"
                @click="openAvatarPicker"
              >
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                  <path
                    d="M14.5 4h-5L8 6H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-3l-1.5-2z"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  />
                  <circle cx="12" cy="13" r="3.2" stroke-width="2" />
                </svg>
              </button>
              <input
                ref="avatarInputRef"
                class="hidden-file-input"
                type="file"
                accept="image/*"
                :disabled="avatarUploading"
                @change="onAvatarFileChange"
              />
            </div>
            <div class="user-info-text">
              <h1>{{ userStore.user?.username }}</h1>
              <p>{{ userStore.user?.role === 'admin' ? '管理员' : '创作者' }}</p>
            </div>
          </div>
          <div class="stats-row">
            <div class="stat-card">
              <span class="stat-value">{{ userStore.user?.uid || '-' }}</span>
              <span class="stat-label">会员编号</span>
            </div>
          </div>
        </div>

        <div class="main-body">
          <Transition name="fade-slide" mode="out-in">
            <div v-if="activeTab === 'profile'" key="profile" class="tab-content">
              <div class="content-card">
                <h2 class="section-title">个人基本资料</h2>
                <div class="profile-grid">
                  <div class="profile-field">
                    <label>用户账号</label>
                    <div class="field-value">{{ userStore.user?.username }}</div>
                  </div>
                  <div class="profile-field">
                    <label>账号状态</label>
                    <div class="field-value">
                      <n-tag
                        :type="userStore.user?.status === 1 ? 'success' : 'warning'"
                        size="small"
                        round
                        ghost
                      >
                        {{ userStore.user?.status === 1 ? '正常使用中' : '已封禁' }}
                      </n-tag>
                    </div>
                  </div>
                  <div class="profile-field">
                    <label>加入时间</label>
                    <div class="field-value">
                      {{ formattedJoinedAt }}
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div v-else-if="activeTab === 'security'" key="security" class="tab-content">
              <div class="content-card">
                <h2 class="section-title">账号安全设置</h2>
                <n-form ref="pwdFormRef" :model="pwdForm" :rules="pwdRules" label-placement="top">
                  <div class="security-grid">
                    <n-form-item label="当前原密码" path="oldPassword">
                      <n-input
                        v-model:value="pwdForm.oldPassword"
                        type="password"
                        placeholder="请输入当前使用的密码"
                        show-password-on="click"
                        class="profile-input"
                      />
                    </n-form-item>
                    <div class="new-password-row">
                      <n-form-item label="设置新密码" path="newPassword">
                        <n-input
                          v-model:value="pwdForm.newPassword"
                          type="password"
                          placeholder="不少于6位字符"
                          show-password-on="click"
                          class="profile-input"
                        />
                      </n-form-item>
                      <n-form-item label="重复新密码" path="confirmPassword">
                        <n-input
                          v-model:value="pwdForm.confirmPassword"
                          type="password"
                          placeholder="请再次输入新密码"
                          show-password-on="click"
                          class="profile-input"
                        />
                      </n-form-item>
                    </div>
                  </div>
                  <n-button
                    type="primary"
                    block
                    :loading="pwdLoading"
                    class="profile-btn"
                    @click="handleUpdatePassword"
                  >
                    确认更新密码
                  </n-button>
                </n-form>
              </div>
            </div>
          </Transition>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { uploadApi } from '../../../shared/api/upload'
import { NTag } from 'naive-ui'
import { formatDisplayErrorMessage } from '../../../shared/utils/errorMessages'

const router = useRouter()
const userStore = useUserStore()
const appToast = useAppToast()

const activeTab = ref('profile')

type NavShape = {
  tag: 'circle' | 'path' | 'polyline' | 'line' | 'rect'
  attrs: Record<string, string | number>
}

type NavItem = {
  key: 'profile' | 'security'
  label: string
  shapes: NavShape[]
}

const navItems = [
  {
    key: 'profile',
    label: '个人资料',
    shapes: [
      { tag: 'circle', attrs: { cx: 12, cy: 12, r: 10 } },
      { tag: 'circle', attrs: { cx: 12, cy: 10, r: 3 } },
      { tag: 'path', attrs: { d: 'M7 20.662V19a2 2 0 0 1 2-2h6a2 2 0 0 1 2 2v1.662' } },
    ],
  },
  {
    key: 'security',
    label: '账号安全',
    shapes: [
      { tag: 'rect', attrs: { x: 3, y: 11, width: 18, height: 11, rx: 2, ry: 2 } },
      { tag: 'path', attrs: { d: 'M7 11V7a5 5 0 0 1 10 0v4' } },
    ],
  },
] satisfies NavItem[]
const avatarInputRef = ref<HTMLInputElement | null>(null)
const avatarUploading = ref(false)
const formattedJoinedAt = computed(() => formatDateWithDots(userStore.user?.created_at))

const pwdFormRef = ref()
const pwdLoading = ref(false)
const pwdForm = ref({ oldPassword: '', newPassword: '', confirmPassword: '' })
const pwdRules = {
  oldPassword: { required: true, message: '请输入当前原密码', trigger: 'blur' },
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '密码至少6个字符', trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: '请确认新密码', trigger: 'blur' },
    {
      validator: (_rule: any, value: string) => value === pwdForm.value.newPassword,
      message: '两次输入密码不一致',
      trigger: 'blur',
    },
  ],
}

function formatDateWithDots(value?: string) {
  const datePart = value?.trim().split(/[T\s]/)[0]
  if (!datePart) return '-'
  const [year, month, day] = datePart.split('-')
  if (!year || !month || !day) return datePart.replaceAll('-', '.')
  return `${year}.${month}.${day}`
}

async function handleUpdatePassword() {
  try {
    await pwdFormRef.value?.validate()
    pwdLoading.value = true
    await userStore.updatePassword(pwdForm.value.oldPassword, pwdForm.value.newPassword)
    appToast.success('密码更新成功，请重新登录')
    pwdForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
    await router.push('/user/login')
  } catch (error: any) {
    if (error.message) appToast.error(error.message)
  } finally {
    pwdLoading.value = false
  }
}

async function handleLogout() {
  await userStore.logout('/user/home')
}

function openAvatarPicker() {
  avatarInputRef.value?.click()
}

async function onAvatarFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (!file.type.startsWith('image/')) {
    appToast.warning('仅支持上传图片文件')
    return
  }
  try {
    avatarUploading.value = true
    const uploadRes = await uploadApi.uploadAvatar(file)
    const avatarUrl = uploadRes.data?.avatar?.trim()
    if (!avatarUrl) {
      throw new Error('头像上传失败，请稍后重试')
    }
    userStore.setAvatarLocal(avatarUrl)
    appToast.success('头像已更新')
  } catch (error) {
    appToast.error(formatDisplayErrorMessage(error, '头像更新失败'))
  } finally {
    avatarUploading.value = false
  }
}

onMounted(() => {
  userStore.fetchUserInfo()
})
</script>

<style scoped>
.center-page {
  width: 100%;
  height: 100vh;
  background: #f8f9fd;
  color: #1f2430;
  font-family: -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Segoe UI', sans-serif;
  overflow: hidden;
  position: relative;
}

.mesh-bg {
  position: fixed;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  overflow: hidden;
  background:
    radial-gradient(50% 35% at 30% 18%, rgba(146, 120, 230, 0.14) 0%, rgba(146, 120, 230, 0) 100%),
    radial-gradient(circle, #e2e4e8 1.5px, transparent 1.5px);
  background-size:
    100% 100%,
    24px 24px;
}
.mesh-blob {
  display: none;
}

.center-container {
  max-width: 1200px;
  height: 90vh;
  margin: 5vh auto;
  background: rgba(255, 255, 255, 0.82);
  backdrop-filter: blur(16px);
  border-radius: 24px;
  border: 1px solid #d9dfec;
  display: flex;
  overflow: hidden;
  position: relative;
  z-index: 1;
}

/* Sidebar */
.center-sidebar {
  width: 260px;
  border-right: 1px solid rgba(0, 0, 0, 0.05);
  padding: 40px 24px;
  display: flex;
  flex-direction: column;
}

.sidebar-header {
  margin-bottom: 48px;
}
.logo-text {
  font-size: 22px;
  font-weight: 700;
  color: #2a3142;
  cursor: pointer;
  background: linear-gradient(135deg, #7f68d5, #9f8fe6);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.sidebar-nav {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-radius: 14px;
  cursor: pointer;
  color: #86868b;
  font-size: 15px;
  font-weight: 500;
  transition: all 0.2s;
}

.nav-item:hover {
  background: #eef1fa;
  color: #2c3448;
}
.nav-item.active {
  background: #7a6bd5;
  color: #fff;
}
.nav-icon {
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.sidebar-footer {
  margin-top: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.action-btn {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border: none;
  background: transparent;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
  border-radius: 12px;
  transition: all 0.2s;
}

.back-btn {
  color: #4d556a;
  background: #f1f3f9;
}
.back-btn:hover {
  background: #e8ecf7;
  color: #2a3142;
}

.admin-btn {
  color: #fff;
  background: linear-gradient(135deg, #7f68d5, #9f8fe6);
}
.admin-btn:hover {
  background: linear-gradient(135deg, #6c54c4, #8b7ad6);
}

.logout-btn {
  color: #ff3b30;
}
.logout-btn:hover {
  background: rgba(255, 59, 48, 0.05);
}

/* Main Content */
.center-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 44px 52px;
}

.center-main {
  scrollbar-width: thin;
  scrollbar-color: rgba(146, 120, 230, 0.38) transparent;
}

.center-main::-webkit-scrollbar {
  width: 8px;
}

.center-main::-webkit-scrollbar-track {
  background: transparent;
}

.center-main::-webkit-scrollbar-thumb {
  background: rgba(146, 120, 230, 0.34);
  border-radius: 999px;
}

.center-main::-webkit-scrollbar-thumb:hover {
  background: rgba(122, 107, 213, 0.5);
}

.main-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 40px;
}

.user-brief {
  display: flex;
  align-items: center;
  gap: 20px;
}
.avatar-big {
  position: relative;
  width: 80px;
  height: 80px;
  border-radius: 50%;
  background: #fff;
  border: 4px solid #fff;
  overflow: visible;
}
.avatar-face {
  width: 100%;
  height: 100%;
  border-radius: 50%;
  overflow: hidden;
}
.avatar-edit-btn {
  position: absolute;
  right: -4px;
  bottom: -4px;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 1px solid #d9dfec;
  background: rgba(255, 255, 255, 0.98);
  color: #6b7290;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition:
    color 0.2s ease,
    border-color 0.2s ease,
    background-color 0.2s ease;
}
.avatar-edit-btn:hover:not(:disabled) {
  border-color: #c8d0e2;
  background: #f8faff;
  color: #4a5169;
}
.avatar-edit-btn:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}
.hidden-file-input {
  display: none;
}
.avatar-face img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.avatar-placeholder {
  width: 100%;
  height: 100%;
  background: linear-gradient(135deg, #9278e6, #c084fc);
  color: #fff;
  font-size: 32px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}

.user-info-text h1 {
  font-size: 28px;
  font-weight: 700;
  margin: 0;
  letter-spacing: -0.02em;
}
.user-info-text p {
  color: #86868b;
  margin: 4px 0 0;
  font-size: 15px;
}

.stats-row {
  display: flex;
  gap: 24px;
}
.stat-card {
  background: rgba(255, 255, 255, 0.5);
  padding: 12px 20px;
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  min-width: 100px;
  border: 1px solid rgba(255, 255, 255, 0.8);
}
.stat-value {
  font-size: 20px;
  font-weight: 700;
  color: #1d1d1f;
}
.stat-label {
  font-size: 13px;
  color: #86868b;
}

.main-body {
  flex: 1;
}

.content-card {
  background: #ffffff;
  padding: 34px;
  border-radius: 18px;
  border: 1px solid #e2e7f3;
}

.section-title {
  font-size: 20px;
  font-weight: 600;
  margin: 0 0 32px;
  color: #1d1d1f;
}

/* Fields */
.profile-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 32px;
}
.profile-field label {
  display: block;
  font-size: 13px;
  color: #86868b;
  margin-bottom: 8px;
  font-weight: 500;
}
.field-value {
  font-size: 16px;
  color: #1d1d1f;
  font-weight: 500;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}
.card-header .section-title {
  margin-bottom: 0;
}

.refresh-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  background: #f5f5f7;
  border: none;
  padding: 8px 16px;
  border-radius: 20px;
  color: #1d1d1f;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}
.refresh-btn:hover {
  background: #e8e8ed;
}
.refresh-btn:disabled {
  opacity: 0.6;
}
.spinning {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.security-grid {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-bottom: 32px;
}
.new-password-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
}

/* Custom Naive UI Styles */
:deep(.n-input.profile-input) {
  --n-border: 1px solid #d2d2d7 !important;
  --n-border-hover: 1px solid #9278e6 !important;
  --n-border-focus: 1px solid #9278e6 !important;
  --n-color: #f5f5f7 !important;
  --n-color-focus: #fff !important;
  --n-text-color: #1d1d1f !important;
  --n-placeholder-color: #86868b !important;
  --n-caret-color: #9278e6 !important;
  border-radius: 12px;
  height: 48px;
}

:deep(.n-input.profile-input input) {
  color: #1d1d1f !important;
  height: 48px !important;
}

:deep(.n-form-item-label__text) {
  font-size: 14px;
  font-weight: 500;
  color: #1d1d1f !important;
  margin-left: 4px;
}

.profile-btn {
  background: #9278e6 !important;
  color: #fff !important;
  height: 48px !important;
  border-radius: 14px !important;
  font-weight: 600 !important;
  font-size: 15px !important;
  border: none !important;
}

.profile-table :deep(.n-data-table-th) {
  background: transparent;
  color: #86868b;
  font-size: 13px;
  font-weight: 500;
  border-bottom: 1px solid #f5f5f7;
}

.profile-table :deep(.n-data-table-td) {
  padding: 16px 12px;
  color: #1d1d1f !important;
  border-bottom: 1px solid #f5f5f7;
}

.profile-table :deep(.n-pagination-item) {
  color: #1d1d1f !important;
}
.profile-table :deep(.n-pagination-item--active) {
  color: #fff !important;
  background-color: #7a6bd5 !important;
  border-color: #7a6bd5 !important;
}

/* Transitions */
.fade-slide-enter-active,
.fade-slide-leave-active {
  transition: all 0.3s ease;
}
.fade-slide-enter-from {
  opacity: 0;
  transform: translateX(20px);
}
.fade-slide-leave-to {
  opacity: 0;
  transform: translateX(-20px);
}
</style>
