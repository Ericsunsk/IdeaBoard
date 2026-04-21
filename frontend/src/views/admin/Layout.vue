<template>
  <router-view v-if="isLoginRoute" />
  <div v-else class="admin-layout-shell">
    <div class="mesh-bg">
      <div class="mesh-blob blob-1"></div>
      <div class="mesh-blob blob-2"></div>
      <div class="mesh-blob blob-3"></div>
    </div>

    <div class="admin-shell-frame">
      <aside v-if="!isMobile" class="admin-shell-sider" :class="{ collapsed }">
        <div class="brand-area" @click="collapsed = !collapsed">
          <div class="brand-mark">F</div>
          <div v-if="!collapsed" class="brand-copy">
            <p class="brand-title">FENG Admin</p>
            <p class="brand-subtitle">Control Center</p>
          </div>
        </div>
        <n-menu
          v-model:value="activeKey"
          :collapsed="collapsed"
          :collapsed-width="76"
          :collapsed-icon-size="18"
          :options="menuOptions"
          @update:value="handleMenuUpdate"
        />
        <div class="sider-footer" :class="{ 'is-collapsed': collapsed }">
          <span class="status-dot"></span>
          <span v-if="!collapsed">System Online</span>
        </div>
      </aside>

      <main class="admin-shell-main">
        <header class="admin-shell-header">
          <div class="header-left">
            <n-button quaternary circle class="shell-action-btn" @click="handleToggleMenu">
              <template #icon>
                <span>{{ isMobile ? '☰' : collapsed ? '☰' : '⟨' }}</span>
              </template>
            </n-button>
            <div class="title-wrap">
              <h1 class="page-title">{{ currentMeta.title }}</h1>
              <p class="page-subtitle">{{ currentMeta.desc }}</p>
            </div>
          </div>
          <div class="header-right">
            <div class="header-state">
              <span class="state-key">Role</span>
              <span class="state-value">Admin</span>
            </div>
            <n-dropdown :options="userOptions" @select="handleUserAction">
              <div class="user-badge">
                <n-avatar size="small" round>{{ userStore.user?.username?.charAt(0).toUpperCase() }}</n-avatar>
                <span>{{ userStore.user?.username }}</span>
              </div>
            </n-dropdown>
          </div>
        </header>

        <section class="admin-shell-content">
          <router-view />
        </section>
      </main>
    </div>
  </div>

  <n-drawer v-model:show="mobileMenuOpen" :width="268" placement="left" class="admin-mobile-drawer">
    <n-drawer-content title="FENG Admin" closable>
      <n-menu v-model:value="activeKey" :options="menuOptions" @update:value="handleMobileMenuSelect" />
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { ref, computed, h, watch, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUserStore } from '../../stores/user'
import {
  NMenu,
  NDropdown,
  NAvatar,
  NButton,
  NIcon,
  NDrawer,
  NDrawerContent,
} from 'naive-ui'
import {
  PeopleOutline,
  LinkOutline,
  KeyOutline,
  PulseOutline,
  DocumentTextOutline,
  ColorPaletteOutline,
  PersonCircleOutline,
  ImageOutline,
  AppsOutline,
  ChatbubbleEllipsesOutline,
  CloudOutline,
  CutOutline,
} from '@vicons/ionicons5'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const collapsed = ref(false)
const activeKey = ref('users')
const mobileMenuOpen = ref(false)
const isMobile = ref(false)
const isLoginRoute = computed(() => route.path === '/admin/login')

watch(
  () => route.path,
  (path) => {
    const match = path.match(/^\/admin\/([\w-]+)/)
    if (match) {
      activeKey.value = match[1]
    }
  },
  { immediate: true }
)

const pageMetaMap: Record<string, { title: string; desc: string }> = {
  users: { title: '用户管理', desc: '维护管理员可控的账户与权限状态。' },
  channels: { title: '渠道商管理', desc: '配置上游渠道、模型池与 API 接入。' },
  accounts: { title: '账号池管理', desc: '监控账号并发、超时与健康度。' },
  monitor: { title: '任务监控', desc: '追踪队列压力、任务状态与资源负载。' },
  'drawing-records': { title: '绘图记录', desc: '审计最近任务、结果与失败详情。' },
  'style-cards': { title: '风格卡片', desc: '管理用户侧风格素材与上架状态。' },
  'face-library': { title: '人脸库', desc: '维护人脸模板资源与分类。' },
  'bg-library': { title: '背景库', desc: '维护背景模板资源与分类。' },
  'pattern-style-library': { title: '风格模型库', desc: '管理图案模型样式与展示素材。' },
  'feature-prompts': { title: '功能提示词', desc: '配置工作流系统提示词模板。' },
  'oss-config': { title: 'Kodo 存储', desc: '对象存储配置、域名和上传策略。' },
  'seg-config': { title: '分割抠图', desc: '配置分割服务开关与密钥。' },
}

const renderMenuIcon = (iconComp: any) => () => h(NIcon, { class: 'menu-icon' }, { default: () => h(iconComp) })

const menuOptions = [
  {
    label: '运营总览',
    key: 'group-ops',
    type: 'group',
    children: [
      { label: '用户管理', key: 'users', icon: renderMenuIcon(PeopleOutline) },
      { label: '渠道商管理', key: 'channels', icon: renderMenuIcon(LinkOutline) },
      { label: 'Banana 账号池', key: 'accounts', icon: renderMenuIcon(KeyOutline) },
      { label: '任务监控', key: 'monitor', icon: renderMenuIcon(PulseOutline) },
      { label: '绘图记录', key: 'drawing-records', icon: renderMenuIcon(DocumentTextOutline) },
    ],
  },
  {
    label: '素材资产',
    key: 'group-assets',
    type: 'group',
    children: [
      { label: '风格卡片', key: 'style-cards', icon: renderMenuIcon(ColorPaletteOutline) },
      { label: '人脸库', key: 'face-library', icon: renderMenuIcon(PersonCircleOutline) },
      { label: '背景库', key: 'bg-library', icon: renderMenuIcon(ImageOutline) },
      { label: '风格模型库', key: 'pattern-style-library', icon: renderMenuIcon(AppsOutline) },
      { label: '功能提示词', key: 'feature-prompts', icon: renderMenuIcon(ChatbubbleEllipsesOutline) },
    ],
  },
  {
    label: '系统配置',
    key: 'group-system',
    type: 'group',
    children: [
      { label: 'Kodo 存储', key: 'oss-config', icon: renderMenuIcon(CloudOutline) },
      { label: '分割抠图', key: 'seg-config', icon: renderMenuIcon(CutOutline) },
    ],
  },
] as any

const currentMeta = computed(() => {
  return pageMetaMap[activeKey.value] || { title: '管理后台', desc: '控制台' }
})

function handleMenuUpdate(key: string) {
  router.push(`/admin/${key}`)
}

function handleMobileMenuSelect(key: string) {
  handleMenuUpdate(key)
  mobileMenuOpen.value = false
}

function handleToggleMenu() {
  if (isMobile.value) {
    mobileMenuOpen.value = true
    return
  }
  collapsed.value = !collapsed.value
}

function syncViewport() {
  isMobile.value = window.innerWidth < 1080
  if (isMobile.value) {
    collapsed.value = true
  }
}

const userOptions = [
  { label: '个人中心', key: 'profile' },
  { type: 'divider', key: 'd1' },
  { label: '退出登录', key: 'logout' }
]

function handleUserAction(key: string) {
  if (key === 'logout') {
    userStore.logout('/admin/login')
  } else if (key === 'profile') {
    router.push('/user/center')
  }
}

onMounted(() => {
  syncViewport()
  window.addEventListener('resize', syncViewport)
})

onUnmounted(() => {
  window.removeEventListener('resize', syncViewport)
})
</script>

<style scoped>
.admin-layout-shell {
  min-height: 100vh;
  position: relative;
  background: #f5f7ff;
  overflow: hidden;
  padding: 18px;
}

.mesh-bg {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.mesh-blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(48px);
  opacity: 0.5;
}

.blob-1 {
  width: 460px;
  height: 460px;
  background: #8374ff;
  top: -170px;
  left: -120px;
}

.blob-2 {
  width: 400px;
  height: 400px;
  background: #58a2ff;
  right: -110px;
  top: 80px;
}

.blob-3 {
  width: 340px;
  height: 340px;
  background: #9ec6ff;
  bottom: -120px;
  left: 32%;
}

.admin-shell-frame {
  position: relative;
  z-index: 1;
  min-height: calc(100vh - 36px);
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 14px;
}

.admin-shell-sider {
  width: 258px;
  display: flex;
  flex-direction: column;
  border: 1px solid rgba(140, 155, 209, 0.28);
  border-radius: 28px;
  background: rgba(255, 255, 255, 0.74);
  backdrop-filter: blur(18px);
  box-shadow: 0 24px 64px rgba(76, 90, 154, 0.14);
}

.admin-shell-sider.collapsed {
  width: 76px;
}

.brand-area {
  height: 74px;
  padding: 0 16px;
  display: flex;
  align-items: center;
  gap: 12px;
  cursor: pointer;
  border-bottom: 1px solid rgba(140, 155, 209, 0.24);
}

.brand-mark {
  width: 38px;
  height: 38px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  font-weight: 700;
  color: #ffffff;
  background: linear-gradient(145deg, #7161ef, #5390fb);
  box-shadow: 0 10px 24px rgba(113, 97, 239, 0.32);
}

.brand-title {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  color: #273156;
  letter-spacing: 0.02em;
}

.brand-subtitle {
  margin: 2px 0 0;
  font-size: 11px;
  color: #8894bc;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.sider-footer {
  margin: 12px;
  padding: 10px 12px;
  border-radius: 999px;
  background: rgba(113, 97, 239, 0.08);
  border: 1px solid rgba(113, 97, 239, 0.18);
  color: #5a6491;
  font-size: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.sider-footer.is-collapsed {
  justify-content: center;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #22b573;
  box-shadow: 0 0 0 4px rgba(34, 181, 115, 0.16);
}

.admin-shell-main {
  min-width: 0;
  border: 1px solid rgba(140, 155, 209, 0.24);
  border-radius: 28px;
  background: rgba(255, 255, 255, 0.68);
  backdrop-filter: blur(20px);
  box-shadow: 0 24px 64px rgba(76, 90, 154, 0.12);
  display: flex;
  flex-direction: column;
}

.admin-shell-header {
  height: 74px;
  padding: 0 22px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid rgba(140, 155, 209, 0.2);
}

.header-left,
.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.shell-action-btn {
  font-size: 16px;
  color: #566092;
}

.title-wrap {
  min-width: 0;
}

.page-title {
  margin: 0;
  font-size: 17px;
  font-weight: 700;
  color: #273156;
}

.page-subtitle {
  margin: 2px 0 0;
  color: #8a95bc;
  font-size: 12px;
}

.header-state {
  border: 1px solid rgba(113, 97, 239, 0.2);
  background: rgba(113, 97, 239, 0.08);
  padding: 6px 10px;
  border-radius: 999px;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}

.state-key {
  color: #7580ac;
}

.state-value {
  color: #584fd1;
}

.user-badge {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 8px 10px;
  border-radius: 999px;
  border: 1px solid rgba(137, 151, 208, 0.3);
  background: rgba(255, 255, 255, 0.82);
  transition: background 0.2s;
}

.user-badge:hover {
  background: rgba(255, 255, 255, 0.96);
}

.user-badge span {
  color: #455081;
  font-size: 13px;
  max-width: 120px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.admin-shell-content {
  min-height: calc(100vh - 110px);
  padding: 20px 22px 24px;
}

:deep(.n-menu) {
  --n-item-text-color: #6976a3;
  --n-item-text-color-active: #2f3965;
  --n-item-text-color-hover: #3f4a79;
  --n-item-color-active: linear-gradient(90deg, rgba(113, 97, 239, 0.18), rgba(113, 97, 239, 0.04));
  --n-item-color-hover: rgba(113, 97, 239, 0.1);
  --n-item-icon-color: #7b87b3;
  --n-item-icon-color-active: #5f54db;
  --n-item-icon-color-hover: #48558b;
  --n-group-header-text-color: #95a1c6;
  padding: 10px;
  gap: 4px;
}

:deep(.n-menu-item-content) {
  border-radius: 12px !important;
  font-size: 13px;
  height: 38px;
}

:deep(.n-menu-item-content__icon) {
  margin-right: 10px !important;
}

:deep(.n-menu-item-group-title) {
  text-transform: uppercase;
  letter-spacing: 0.06em;
  font-size: 11px;
}

.menu-icon {
  font-size: 17px;
}

@media (max-width: 1079px) {
  .admin-layout-shell {
    padding: 10px;
  }

  .admin-shell-frame {
    display: block;
    min-height: calc(100vh - 20px);
  }

  .admin-shell-main {
    border-radius: 22px;
  }

  .admin-shell-header {
    padding: 0 14px;
    height: 68px;
  }

  .page-subtitle,
  .header-state {
    display: none;
  }

  .admin-shell-content {
    padding: 12px;
    min-height: calc(100vh - 90px);
  }
}
</style>
