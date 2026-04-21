<template>
  <div class="admin-page kodo-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">Kodo 存储配置</h2>
        <p class="page-desc">管理七牛云存储接入与高级上传策略，开启后仅影响新上传媒体。</p>
      </div>
    </div>

    <div class="switch-card">
      <div class="switch-left">
        <div class="switch-icon">☁️</div>
        <div class="switch-text">
          <div class="switch-title">七牛云 Kodo 对象存储</div>
          <div class="switch-desc">{{ form.enabled ? '已开启 — 新上传的媒体文件将存储至七牛云' : '已关闭 — 媒体文件存储在服务器本地' }}</div>
        </div>
      </div>
      <n-switch v-model:value="form.enabled" :loading="loading" />
    </div>

    <!-- 提示条 -->
    <div class="tip-bar">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
      <span>建议绑定自定义域名用于访问存储空间。开启后仅影响新上传文件，已有本地文件不受影响。密钥请到 <a href="https://portal.qiniu.com/developer/user/key" target="_blank" class="tip-link">七牛云密钥管理</a> 获取。</span>
    </div>

    <!-- 配置表单卡片 -->
    <div class="config-card" :class="{ disabled: !form.enabled }">
      <div class="card-title">密钥配置</div>

      <div class="field-group">
        <div class="field-row">
          <div class="field">
            <label>Access Key</label>
            <n-input v-model:value="form.access_key" placeholder="请输入 Access Key" :disabled="!form.enabled" />
            <span class="field-hint">前往 <a href="https://portal.qiniu.com/developer/user/key" target="_blank">密钥管理</a> 获取</span>
          </div>
          <div class="field">
            <label>Secret Key</label>
            <n-input v-model:value="form.secret_key" type="password" show-password-on="click" :placeholder="hasExistingSecret ? '已配置，留空则保持不变' : '请输入 Secret Key'" :disabled="!form.enabled" />
            <span v-if="hasExistingSecret && !form.secret_key" class="field-hint" style="color:#6366f1;">已保存密钥，留空保存将保持原密钥不变</span>
          </div>
        </div>
      </div>

      <div class="divider"></div>

      <div class="card-title">存储空间配置</div>
      <div class="field-group">
        <div class="field">
          <label>存储空间名称</label>
          <n-input v-model:value="form.bucket_name" placeholder="请输入 Bucket 名称" :disabled="!form.enabled" />
          <span class="field-hint">前往 <a href="https://portal.qiniu.com/kodo/bucket" target="_blank">Kodo 控制台</a> 获取及管理</span>
        </div>

        <div class="field">
          <label>存储空间源站上传域名（可选）</label>
          <n-input v-model:value="form.upload_domain" placeholder="留空则自动检测区域，或手动填写如 up-z2.qiniup.com" :disabled="!form.enabled" />
          <span class="field-hint">留空时 SDK 会自动检测 Bucket 所在区域并选择正确的上传域名。手动填写需确保区域一致，参考 <a href="https://developer.qiniu.com/kodo/1671/region-endpoint-fq" target="_blank">区域及域名文档</a></span>
        </div>

        <div class="field">
          <div class="field-label-row">
            <label>存储空间访问域名</label>
            <n-button text type="primary" size="tiny" :loading="fetching" :disabled="!form.enabled || !form.access_key || !form.bucket_name" @click="fetchBucketDomains">
              <template #icon>
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
              </template>
              自动获取
            </n-button>
          </div>
          <n-input v-model:value="form.access_domain" placeholder="例如：https://xxx.clouddn.com" :disabled="!form.enabled" />
          <span class="field-hint">自定义域名或七牛云测试域名（自定义域名需要在 Kodo 控制台绑定可正常访问使用）</span>
          <div v-if="bucketDomains.length > 0" class="domain-list">
            <span class="domain-label">检测到的域名：</span>
            <span v-for="d in bucketDomains" :key="d" class="domain-tag" @click="form.access_domain = 'https://' + d">{{ d }}</span>
          </div>
        </div>
      </div>

      <div class="divider"></div>

      <div class="card-title">高级选项</div>
      <div class="option-row">
        <div class="option-info">
          <div class="option-label">直传模式（Fetch）</div>
          <div class="option-desc">开启后 AI 生成的图片将由七牛云服务器直接从上游 URL 抓取存储，不经过业务服务器中转，节省带宽和服务器流量。Base64 格式图片仍走中转上传。</div>
        </div>
        <n-switch v-model:value="form.direct_fetch" :disabled="!form.enabled" />
      </div>

      <div style="height:12px"></div>

      <div class="option-row">
        <div class="option-info">
          <div class="option-label">缩略图模式支持</div>
          <div class="option-desc">开启后绘画、画廊列表中的图片将以缩略图展示（七牛云测试域名是去参数保存的，会造成图片尺寸不一致，请务必绑定自定义域名。且此选项只适用于图片链接和【存储空间访问域名】配置中的域名相符才会生效）</div>
        </div>
        <n-switch v-model:value="form.thumbnail_mode" :disabled="!form.enabled" />
      </div>

      <div class="divider"></div>

      <!-- 操作按钮 -->
      <div class="action-bar">
        <n-button :loading="testing" :disabled="!form.enabled" @click="handleTest">
          <template #icon>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
          </template>
          测试连接
        </n-button>
        <n-button type="primary" :loading="saving" @click="handleSave">保存配置</n-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NInput, NSwitch, NButton, useMessage } from 'naive-ui'
import { kodoApi } from '../../api/index'
import type { KodoConfig } from '../../api/index'

const message = useMessage()

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const fetching = ref(false)

const hasExistingSecret = ref(false)
const bucketDomains = ref<string[]>([])

const form = ref<KodoConfig>({
  enabled: false,
  access_key: '',
  secret_key: '',
  bucket_name: '',
  upload_domain: '',
  access_domain: '',
  thumbnail_mode: false,
  direct_fetch: false,
})

async function loadConfig() {
  loading.value = true
  try {
    const res: any = await kodoApi.getConfig()
    if (res.code === 0 && res.data?.config) {
      const cfg = { ...res.data.config }
      if (cfg.secret_key && cfg.secret_key.includes('*')) {
        cfg.secret_key = ''
        hasExistingSecret.value = true
      }
      form.value = { ...form.value, ...cfg }
    }
  } catch (e) {
    console.error('Failed to load Kodo config', e)
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  saving.value = true
  try {
    const res: any = await kodoApi.saveConfig(form.value)
    if (res.code === 0) {
      message.success('配置已保存')
    } else {
      message.error(res.msg || '保存失败')
    }
  } catch (e: any) {
    message.error(e.msg || '保存失败')
  } finally {
    saving.value = false
  }
}

async function handleTest() {
  testing.value = true
  try {
    const res: any = await kodoApi.testConnection()
    if (res.code === 0) {
      message.success('连接成功')
    } else {
      message.error(res.msg || '连接失败')
    }
  } catch (e: any) {
    message.error(e.msg || '连接失败')
  } finally {
    testing.value = false
  }
}

async function fetchBucketDomains() {
  fetching.value = true
  try {
    // 先保存当前配置以便后端能读到 AK/SK/Bucket
    await kodoApi.saveConfig(form.value)
    const res: any = await kodoApi.getBucketInfo()
    if (res.code === 0 && res.data?.info?.domains?.length) {
      bucketDomains.value = res.data.info.domains
      message.success(`检测到 ${bucketDomains.value.length} 个域名`)
    } else {
      message.warning(res.msg || '未检测到绑定域名')
    }
  } catch (e: any) {
    message.error(e.msg || '获取失败')
  } finally {
    fetching.value = false
  }
}

onMounted(() => {
  loadConfig()
})
</script>

<style scoped>
@import './admin-common.css';

.kodo-page {
  gap: 14px;
  max-width: 720px;
}
.divider {
  height: 1px;
  background: #2d394b;
  margin: 20px 0;
}

.option-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}
.option-label {
  font-size: 13px;
  font-weight: 500;
  color: #d4dde8;
}
.option-desc {
  font-size: 12px;
  color: #8294a9;
  margin-top: 2px;
}
.field-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.domain-list {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 4px;
}
.domain-label {
  font-size: 11px;
  color: #7d8ea4;
}
.domain-tag {
  font-size: 12px;
  padding: 2px 10px;
  border-radius: 6px;
  background: rgba(60, 185, 194, 0.16);
  color: #95dfe5;
  cursor: pointer;
  transition: background 0.15s;
}
.domain-tag:hover {
  background: rgba(60, 185, 194, 0.25);
}
</style>
