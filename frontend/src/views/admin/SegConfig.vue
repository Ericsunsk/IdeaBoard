<template>
  <div class="seg-page">
    <div class="switch-card">
      <div class="switch-left">
        <div class="switch-icon">✂️</div>
        <div class="switch-text">
          <div class="switch-title">阿里云分割抠图</div>
          <div class="switch-desc">{{ form.enabled ? '已开启 — 用户可在大图预览中使用智能分割功能' : '已关闭 — 分割功能不可用' }}</div>
        </div>
      </div>
      <n-switch v-model:value="form.enabled" :loading="loading" />
    </div>

    <div class="tip-bar">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
      <span>请先在 <a href="https://common-buy.aliyun.com/?commodityCode=viapi_imageseg_public_cn" target="_blank" class="tip-link">阿里云</a> 开通分割抠图服务，并在 <a href="https://ram.console.aliyun.com/manage/ak" target="_blank" class="tip-link">AccessKey管理</a> 获取密钥。</span>
    </div>

    <div class="config-card" :class="{ disabled: !form.enabled }">
      <div class="card-title">密钥配置</div>
      <div class="field-group">
        <div class="field">
          <label>AccessKey ID</label>
          <n-input v-model:value="form.access_key_id" placeholder="请输入 AccessKey ID" :disabled="!form.enabled" />
        </div>
        <div class="field">
          <label>AccessKey Secret</label>
          <n-input v-model:value="form.access_key_secret" type="password" show-password-on="click" placeholder="请输入 AccessKey Secret" :disabled="!form.enabled" />
        </div>
        <div class="field">
          <label>Region ID</label>
          <n-input v-model:value="form.region_id" placeholder="默认 cn-shanghai" :disabled="!form.enabled" />
          <span class="field-hint">API 接入地域，默认 cn-shanghai</span>
        </div>
      </div>
    </div>

    <div class="action-bar">
      <n-button type="primary" :loading="saving" @click="handleSave">保存配置</n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NButton, NInput, NSwitch, useMessage } from 'naive-ui'
import api from '../../api/index'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)

const form = ref({
  enabled: false,
  access_key_id: '',
  access_key_secret: '',
  region_id: 'cn-shanghai',
})

async function loadConfig() {
  loading.value = true
  try {
    const res: any = await (api as any).get('/admin/seg/config')
    if (res.code === 0 && res.data?.config) {
      const cfg = res.data.config
      form.value = {
        enabled: cfg.enabled || false,
        access_key_id: cfg.access_key_id || '',
        access_key_secret: cfg.access_key_secret || '',
        region_id: cfg.region_id || 'cn-shanghai',
      }
    }
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  saving.value = true
  try {
    const res: any = await (api as any).post('/admin/seg/config', form.value)
    if (res.code === 0) {
      message.success('保存成功')
    } else {
      message.error(res.msg || '保存失败')
    }
  } catch (e) {
    message.error('保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(loadConfig)
</script>

<style scoped>
.seg-page {
  padding: 28px 32px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 720px;
}
.switch-card {
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 12px;
  padding: 20px 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.switch-left { display: flex; align-items: center; gap: 16px; }
.switch-icon {
  font-size: 28px;
  width: 48px; height: 48px;
  display: flex; align-items: center; justify-content: center;
  background: rgba(99,102,241,0.1);
  border-radius: 12px;
}
.switch-title { font-size: 15px; font-weight: 600; color: #fafafa; }
.switch-desc { font-size: 13px; color: #71717a; margin-top: 2px; }
.tip-bar {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 12px 16px;
  border-radius: 8px;
  background: rgba(234,179,8,0.08);
  border: 1px solid rgba(234,179,8,0.2);
  font-size: 12px;
  color: #ca8a04;
  line-height: 1.6;
}
.tip-bar svg { flex-shrink: 0; margin-top: 2px; stroke: #ca8a04; }
.tip-link { color: #ca8a04; text-decoration: underline; text-underline-offset: 2px; }
.config-card {
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 12px;
  padding: 24px;
  transition: opacity 0.2s;
}
.config-card.disabled { opacity: 0.45; pointer-events: none; }
.card-title {
  font-size: 14px; font-weight: 600; color: #a1a1aa;
  text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 16px;
}
.field-group { display: flex; flex-direction: column; gap: 16px; }
.field { display: flex; flex-direction: column; gap: 6px; flex: 1; }
.field label { font-size: 13px; font-weight: 500; color: #d4d4d8; }
.field-hint { font-size: 11px; color: #52525b; }
.action-bar { display: flex; justify-content: flex-end; gap: 10px; }
:deep(.n-input) {
  --n-border: 1px solid #27272a;
  --n-border-hover: 1px solid #6366f1;
  --n-border-focus: 1px solid #6366f1;
  --n-color: #09090b;
  --n-color-focus: #09090b;
  --n-text-color: #e4e4e7;
  --n-placeholder-color: #3f3f46;
  border-radius: 8px;
}
:deep(.n-button--primary-type) {
  --n-color: #6366f1;
  --n-color-hover: #818cf8;
  --n-color-pressed: #4f46e5;
  border-radius: 8px;
}
:deep(.n-switch) {
  --n-rail-color: #3f3f46;
  --n-rail-color-active: #6366f1;
}
</style>
