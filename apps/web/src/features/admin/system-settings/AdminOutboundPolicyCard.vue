<template>
  <SettingEditorCard title="私有地址访问" :status="statusLabel" :status-severity="statusSeverity">
    <template #default>
      <div class="flex flex-col gap-5">
        <Message v-if="error" severity="error" :closable="false">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>私有地址访问策略加载失败，请重试。</span>
            <Button
              label="重试"
              :class="adminActionButtonClass"
              severity="secondary"
              size="small"
              outlined
              @click="refetch()"
            />
          </div>
        </Message>
        <Message v-else-if="loading && !setting" severity="secondary" :closable="false">
          正在加载私有地址访问策略…
        </Message>

        <AdminFormField
          v-slot="{ describedbyId }"
          label="允许访问的私有主机"
          for="outbound-policy-allow-hosts"
          hint="每行一个 hostname、IP 或 host:port。公网 Endpoint 无需添加。"
        >
          <Textarea
            id="outbound-policy-allow-hosts"
            v-model="form.allow_hosts"
            :rows="5"
            fluid
            :disabled="controlsDisabled"
            :aria-describedby="describedbyId"
          />
        </AdminFormField>

        <Message v-if="validationError" severity="error" :closable="false">
          {{ validationError }}
        </Message>
      </div>
    </template>

    <template #actions>
      <Button
        :class="adminActionButtonClass"
        label="保存访问策略"
        severity="contrast"
        :disabled="controlsDisabled"
        :loading="saving"
        @click="save"
      />
      <Button
        :class="adminActionButtonClass"
        label="重置"
        severity="secondary"
        outlined
        :disabled="controlsDisabled"
        @click="reset"
      />
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Textarea from 'primevue/textarea'
import AdminFormField from '../shared/components/AdminFormField.vue'
import { adminActionButtonClass } from '../shared/lib/uiClasses'
import SettingEditorCard from './SettingEditorCard.vue'
import { useOutboundPolicy } from './useOutboundPolicy'
import { useAdminOutboundPolicyFeedback } from './useSystemSettingsFeedback'

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
