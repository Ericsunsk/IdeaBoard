<template>
  <n-layout has-sider class="admin-layout">
    <!-- 左侧菜单 -->
    <n-layout-sider
      bordered
      collapse-mode="width"
      :collapsed="collapsed"
      :collapsed-width="64"
      :width="220"
      :native-scrollbar="false"
      class="sider"
    >
      <div class="logo-area" @click="collapsed = !collapsed">
        <span class="logo-icon">{{ collapsed ? '🛡️' : '🛡️ 管理后台' }}</span>
      </div>
      <n-menu
        v-model:value="activeKey"
        :collapsed="collapsed"
        :collapsed-width="64"
        :collapsed-icon-size="22"
        :options="menuOptions"
        @update:value="handleMenuUpdate"
      />
    </n-layout-sider>

    <!-- 右侧内容 -->
    <n-layout>
      <!-- 顶部菜单 -->
      <n-layout-header class="top-header" bordered>
        <div class="header-left">
          <n-breadcrumb>
            <n-breadcrumb-item>管理后台</n-breadcrumb-item>
            <n-breadcrumb-item>{{ currentMenuLabel }}</n-breadcrumb-item>
          </n-breadcrumb>
        </div>
        <div class="header-right">
          <n-dropdown :options="userOptions" @select="handleUserAction">
            <div class="user-info">
              <n-avatar size="small" round>{{ userStore.user?.username?.charAt(0).toUpperCase() }}</n-avatar>
              <span>{{ userStore.user?.username }}</span>
            </div>
          </n-dropdown>
        </div>
      </n-layout-header>

      <!-- 内容区 -->
      <n-layout-content class="content">
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<script setup lang="ts">
import { ref, computed, h, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useUserStore } from '../../stores/user'
import { NLayout, NLayoutSider, NLayoutHeader, NLayoutContent, NMenu, NDropdown, NAvatar, NBreadcrumb, NBreadcrumbItem, NButton } from 'naive-ui'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const collapsed = ref(false)
const activeKey = ref('users')

// 同步路由到菜单高亮
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

const menuOptions = [
  {
    label: '用户管理',
    key: 'users',
    icon: () => h('span', { style: 'font-size:16px' }, '👥')
  },
  {
    label: '渠道商管理',
    key: 'channels',
    icon: () => h('span', { style: 'font-size:16px' }, '🔗')
  },
  {
    label: 'Banana 账号池',
    key: 'accounts',
    icon: () => h('span', { style: 'font-size:16px' }, '🍌')
  },
  { type: 'divider', key: 'd-content' },
  {
    label: '风格卡片',
    key: 'style-cards',
    icon: () => h('span', { style: 'font-size:16px' }, '🎨')
  },
  {
    label: '绘图记录',
    key: 'drawing-records',
    icon: () => h('span', { style: 'font-size:16px' }, '🖼️')
  },
  {
    label: '任务监控',
    key: 'monitor',
    icon: () => h('span', { style: 'font-size:16px' }, '📊')
  },
  { type: 'divider', key: 'd-config' },
  {
    label: 'Kodo 存储',
    key: 'oss-config',
    icon: () => h('span', { style: 'font-size:16px' }, '☁️')
  },
  {
    label: '分割抠图',
    key: 'seg-config',
    icon: () => h('span', { style: 'font-size:16px' }, '✂️')
  },
  {
    label: '线稿模式',
    key: 'line-draft-config',
    icon: () => h('span', { style: 'font-size:16px' }, '✏️')
  },
  {
    label: '人脸库',
    key: 'face-library',
    icon: () => h('span', { style: 'font-size:16px' }, '🧑')
  },
  {
    label: '背景库',
    key: 'bg-library',
    icon: () => h('span', { style: 'font-size:16px' }, '🖼️')
  },
  {
    label: '模特库',
    key: 'model-library',
    icon: () => h('span', { style: 'font-size:16px' }, '💃')
  },
  {
    label: '姿势库',
    key: 'pose-library',
    icon: () => h('span', { style: 'font-size:16px' }, '🕴️')
  },
  {
    label: '风格模型库',
    key: 'pattern-style-library',
    icon: () => h('span', { style: 'font-size:16px' }, '🎭')
  },
  {
    label: '功能提示词',
    key: 'feature-prompts',
    icon: () => h('span', { style: 'font-size:16px' }, '💬')
  }
] as any

const currentMenuLabel = computed(() => {
  const item = menuOptions.find((m: any) => m.key === activeKey.value)
  return item?.label || ''
})

function handleMenuUpdate(key: string) {
  router.push(`/admin/${key}`)
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
</script>

<style scoped>
.admin-layout {
  min-height: 100vh;
}

.sider {
  background: #18181b;
}

.logo-area {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-bottom: 1px solid #27272a;
}

.logo-icon {
  font-size: 20px;
  color: #fafafa;
  white-space: nowrap;
}

.top-header {
  height: 56px;
  padding: 0 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #18181b;
  position: sticky;
  top: 0;
  z-index: 100;
}

.header-left {
  display: flex;
  align-items: center;
}

.header-right {
  display: flex;
  align-items: center;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 8px 12px;
  border-radius: 8px;
  transition: background 0.2s;
}

.user-info:hover {
  background: #27272a;
}

.user-info span {
  color: #fafafa;
  font-size: 14px;
}

.content {
  background: #0c0c0f;
  min-height: calc(100vh - 56px);
  padding: 0;
}

/* Naive menu theme overrides */
:deep(.n-menu) {
  --n-item-text-color: #a1a1aa;
  --n-item-text-color-active: #e4e4e7;
  --n-item-text-color-hover: #e4e4e7;
  --n-item-color-active: rgba(99,102,241,0.1);
  --n-item-color-hover: rgba(255,255,255,0.04);
  --n-item-icon-color: #71717a;
  --n-item-icon-color-active: #818cf8;
  --n-item-icon-color-hover: #a1a1aa;
  padding: 8px;
}
:deep(.n-menu-item-content) {
  border-radius: 8px !important;
  font-size: 13px;
}
:deep(.n-menu-divider) {
  margin: 8px 16px;
  background: #27272a;
}
</style>
