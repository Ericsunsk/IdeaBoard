<template>
  <Card :class="[adminMasonryCardClass, 'min-w-0']" :pt="adminCardPt">
    <template #title>
      <div class="flex min-w-0 items-start justify-between gap-4">
        <div class="min-w-0">
          <h2 class="m-0 text-admin-title-md font-bold text-admin-title-color">Search Providers</h2>
        </div>
        <Button
          text
          :class="adminFramedIconButtonClass"
          aria-label="刷新搜索供应商配置"
          :disabled="loading"
          @click="load"
        >
          <template #icon><Refresh class="size-4 text-admin-text-2" /></template>
        </Button>
      </div>
    </template>
    <template #content>
      <div class="grid min-w-0 grid-cols-1 gap-3">
        <div
          v-for="row in rows"
          :key="row.provider"
          class="min-w-0 rounded-xl border border-admin-border-soft bg-admin-surface-muted p-4"
        >
          <div class="flex min-w-0 items-center justify-between gap-3">
            <div class="flex min-w-0 items-center gap-2.5">
              <div
                class="grid size-9 place-items-center rounded-lg bg-admin-accent text-sm font-bold text-admin-text-inverse"
              >
                {{ providerLabel(row.provider).slice(0, 1) }}
              </div>
              <div class="min-w-0">
                <div class="truncate text-admin-body font-bold text-admin-title-color">
                  {{ providerLabel(row.provider) }}
                </div>
                <div class="truncate text-admin-caption text-admin-text-3">
                  {{ providerHost(row.provider) }}
                </div>
              </div>
            </div>
            <Tag
              :value="statusLabel(row)"
              :severity="statusSeverity(row)"
              :class="[adminCompactTagClass, 'shrink-0']"
              rounded
            />
          </div>
          <div class="mt-4 flex flex-col gap-3">
            <InputText
              v-model="drafts[row.provider]"
              type="password"
              autocomplete="new-password"
              class="w-full"
              :placeholder="row.api_key_configured ? '已配置，留空保持不变' : '输入 API Key'"
              :disabled="savingProvider === row.provider || testingProvider === row.provider"
            />
            <template v-if="row.provider === 'vision'">
              <InputText
                v-model="endpointDrafts[row.provider]"
                class="w-full"
                placeholder="Endpoint URL"
                :disabled="savingProvider === row.provider || testingProvider === row.provider"
              />
              <InputText
                v-model="modelDrafts[row.provider]"
                class="w-full"
                placeholder="Model"
                :disabled="savingProvider === row.provider || testingProvider === row.provider"
              />
            </template>
            <div
              class="flex items-center justify-between rounded-lg border border-admin-border-soft bg-white px-3 py-2"
            >
              <span class="text-admin-meta font-semibold text-admin-text-1">启用搜索</span
              ><ToggleSwitch
                :model-value="row.enabled"
                :disabled="
                  !canEnable(row) ||
                  savingProvider === row.provider ||
                  testingProvider === row.provider
                "
                :aria-label="`${providerLabel(row.provider)} 启用状态`"
                @update:model-value="save(row, $event)"
              />
            </div>
            <div class="flex min-w-0 flex-col gap-2">
              <span v-if="row.status_message" class="truncate text-admin-caption text-admin-text-3">
                {{ row.status_message }}
              </span>
              <div class="flex flex-wrap justify-end gap-2">
                <Button
                  label="测试"
                  size="small"
                  severity="secondary"
                  outlined
                  :class="adminActionButtonClass"
                  :loading="testingProvider === row.provider"
                  :disabled="!row.api_key_configured || savingProvider === row.provider"
                  @click="test(row)"
                /><Button
                  label="保存"
                  size="small"
                  severity="contrast"
                  :class="adminActionButtonClass"
                  :loading="savingProvider === row.provider"
                  :disabled="testingProvider === row.provider"
                  @click="save(row, row.enabled)"
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </Card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputText from 'primevue/inputtext'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { Refresh } from '@iconoir/vue'
import { useAdminToast } from '../shared/lib/adminToast'
import { adminCardPt } from '../shared/lib/adminCardPt'
import {
  adminCompactTagClass,
  adminActionButtonClass,
  adminFramedIconButtonClass,
  adminMasonryCardClass,
} from '../shared/lib/adminUiClasses'
import {
  fetchSearchProviders,
  saveSearchProvider,
  testSearchProvider,
  type SearchProviderRecord,
} from './searchProviderResource'

const toast = useAdminToast()
const loading = ref(false)
const savingProvider = ref<SearchProviderRecord['provider'] | ''>('')
const testingProvider = ref<SearchProviderRecord['provider'] | ''>('')
const rows = ref<SearchProviderRecord[]>([])
const drafts = reactive<Record<string, string>>({})
const endpointDrafts = reactive<Record<string, string>>({})
const modelDrafts = reactive<Record<string, string>>({})

async function load() {
  loading.value = true
  try {
    rows.value = await fetchSearchProviders()
    for (const row of rows.value) {
      drafts[row.provider] = ''
      endpointDrafts[row.provider] = row.endpoint_url ?? ''
      modelDrafts[row.provider] = row.model ?? ''
    }
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '加载搜索供应商配置失败')
  } finally {
    loading.value = false
  }
}
async function save(row: SearchProviderRecord, enabled: boolean) {
  savingProvider.value = row.provider
  try {
    await saveSearchProvider(row.provider, {
      apiKey: drafts[row.provider],
      endpointUrl: endpointDrafts[row.provider],
      model: modelDrafts[row.provider],
      enabled,
    })
    drafts[row.provider] = ''
    toast.success(`${providerLabel(row.provider)} 配置已保存`)
    await load()
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '保存搜索供应商配置失败')
  } finally {
    savingProvider.value = ''
  }
}
async function test(row: SearchProviderRecord) {
  testingProvider.value = row.provider
  try {
    await testSearchProvider(row.provider)
    toast.success(`${providerLabel(row.provider)} 连接测试通过`)
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '搜索供应商连接测试失败')
  } finally {
    await load()
    testingProvider.value = ''
  }
}
function canEnable(row: SearchProviderRecord) {
  return (
    row.test_state === 'passed' &&
    row.api_key_configured &&
    (row.provider !== 'vision' || Boolean(row.endpoint_url && row.model))
  )
}
function providerLabel(provider: SearchProviderRecord['provider']) {
  if (provider === 'unsplash') return 'Unsplash'
  if (provider === 'pexels') return 'Pexels'
  return 'Vision Search'
}
function providerHost(provider: SearchProviderRecord['provider']) {
  if (provider === 'unsplash') return 'api.unsplash.com'
  if (provider === 'pexels') return 'api.pexels.com'
  return 'multipart Vision endpoint'
}
function statusLabel(row: SearchProviderRecord) {
  if (row.enabled) return '已启用'
  if (row.test_state === 'passed') return '测试通过'
  if (row.test_state === 'failed') return '测试失败'
  return row.api_key_configured ? '待测试' : '未配置'
}
function statusSeverity(row: SearchProviderRecord) {
  if (row.enabled || row.test_state === 'passed') return 'success'
  if (row.test_state === 'failed') return 'danger'
  return 'secondary'
}
onMounted(load)
</script>
