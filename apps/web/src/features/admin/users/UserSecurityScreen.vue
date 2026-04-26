<template>
  <div class="admin-page user-security-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">用户安全管理</h2>
        <p class="page-desc">业务治理继续在 IdeaBoard Admin 操作，身份与安全治理统一在 Better Auth Studio 执行。</p>
      </div>
      <div class="header-side-note">
        <span>安全优先</span>
      </div>
    </div>

    <div class="security-card">
      <AdminSectionHeader title="Better Auth Studio" description="改密、会话吊销、物理删号等高敏操作建议统一在 Studio 完成。" />
      <div class="security-actions">
        <n-button type="primary" :disabled="!authStudioURL" @click="openStudio">
          打开 Better Auth Studio
        </n-button>
        <span class="security-hint">
          {{ authStudioURL ? authStudioURL : '未配置 VITE_AUTH_STUDIO_URL，按钮已禁用。' }}
        </span>
      </div>
    </div>

    <div class="security-card">
      <AdminSectionHeader title="职责边界" description="避免重复造轮子，降低认证演进的维护成本。" />
      <ul class="security-list">
        <li>业务运营：封禁/解封、角色与业务权限分配、业务资产治理（Admin）</li>
        <li>身份安全：密码重置、会话管理、物理删号（Better Auth Studio）</li>
        <li>生产默认关闭公开注册，按运营窗口临时开启</li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NButton, useMessage } from 'naive-ui'
import AdminSectionHeader from '../shared/components/AdminSectionHeader.vue'

const message = useMessage()

const authStudioURL = computed(() => (import.meta.env.VITE_AUTH_STUDIO_URL || '').trim())

function openStudio() {
  if (!authStudioURL.value) {
    message.warning('未配置 VITE_AUTH_STUDIO_URL')
    return
  }
  window.open(authStudioURL.value, '_blank', 'noopener,noreferrer')
}
</script>

<style scoped>
@import '../shared/adminCommon.css';

.user-security-page {
  gap: 14px;
}

.security-card {
  border: 1px solid var(--admin-border);
  border-radius: 14px;
  background: var(--admin-card);
  padding: 14px;
}

.security-actions {
  margin-top: 8px;
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.security-hint {
  color: var(--admin-text-3);
  font-size: 12px;
  word-break: break-all;
}

.security-list {
  margin: 10px 0 0;
  padding-left: 18px;
  color: var(--admin-text-2);
  line-height: 1.7;
  font-size: 13px;
}
</style>
