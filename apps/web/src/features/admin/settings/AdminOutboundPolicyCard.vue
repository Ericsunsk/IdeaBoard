<template>
  <SettingEditorCard
    title="私有地址访问"
    :status="statusLabel"
    :status-severity="statusSeverity"
    :class="props.class"
  >
    <template #default>
      <div class="flex flex-col gap-5">
        <AdminAlert v-if="error" severity="error">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>私有地址访问策略加载失败，请重试。</span>
            <AdminButton
              :class="adminActionButtonClass"
              variant="outline"
              size="small"
              @click="refetch()"
            >
              重试
            </AdminButton>
          </div>
        </AdminAlert>
        <AdminAlert v-else-if="loading && !setting" severity="secondary">
          正在加载私有地址访问策略…
        </AdminAlert>

        <AdminFormField
          v-slot="{ describedbyId }"
          label="允许访问的私有主机"
          for="outbound-policy-allow-hosts"
          hint="每行一个 hostname、IP 或 host:port。公网 Endpoint 无需添加。"
        >
          <AdminTextarea
            id="outbound-policy-allow-hosts"
            v-model="form.allow_hosts"
            :rows="5"
            class="w-full"
            :disabled="controlsDisabled"
            :aria-describedby="describedbyId"
          />
        </AdminFormField>

        <AdminAlert v-if="validationError" severity="error">
          {{ validationError }}
        </AdminAlert>
      </div>
    </template>

    <template #actions>
      <AdminButton
        :class="adminActionButtonClass"
        :disabled="controlsDisabled"
        :loading="saving"
        @click="save"
      >
        保存访问策略
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        variant="outline"
        :disabled="controlsDisabled"
        @click="reset"
      >
        重置
      </AdminButton>
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import AdminButton from '../shared/components/AdminButton.vue'
import AdminAlert from '../shared/components/AdminAlert.vue'
import AdminTextarea from '../shared/components/AdminTextarea.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import { adminActionButtonClass } from '../shared/lib/uiClasses'
import SettingEditorCard from './SettingEditorCard.vue'
import { useOutboundPolicy } from './useOutboundPolicy'
import { useAdminOutboundPolicyFeedback } from './useSettingsFeedback'

const props = defineProps<{ class?: HTMLAttributes['class'] }>()

const feedback = useAdminOutboundPolicyFeedback()
const { error, form, loading, refetch, reset, save, saving, setting, validationError } =
  useOutboundPolicy({ feedback })

const controlsDisabled = computed(() => loading.value || saving.value || Boolean(error.value))
const statusLabel = computed(() => {
  if (error.value) return '加载失败'
  if (!setting.value) return '加载中'
  const count = setting.value.allow_hosts.length
  return count ? `已额外放行 ${count} 项` : '未额外放行'
})
const statusSeverity = computed(() => {
  if (error.value) return 'danger'
  if (!setting.value) return 'secondary'
  return setting.value.allow_hosts.length ? 'warn' : 'secondary'
})
</script>
