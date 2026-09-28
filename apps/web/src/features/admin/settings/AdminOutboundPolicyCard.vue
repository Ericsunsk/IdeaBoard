<template>
  <SettingEditorCard
    :title="SETTINGS_MESSAGES.outboundTitle"
    :status="statusLabel"
    :status-severity="statusSeverity"
    :class="props.class"
  >
    <template #default>
      <div class="flex flex-col gap-5">
        <AdminAlert v-if="error" severity="error">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>{{ SETTINGS_MESSAGES.outboundLoadFailed }}</span>
            <AdminButton
              :class="adminActionButtonClass"
              variant="outline"
              size="small"
              @click="refetch()"
            >
              {{ SETTINGS_MESSAGES.retry }}
            </AdminButton>
          </div>
        </AdminAlert>
        <AdminAlert v-else-if="loading && !setting" severity="secondary">
          {{ SETTINGS_MESSAGES.outboundLoading }}
        </AdminAlert>

        <AdminFormField
          v-slot="{ describedbyId }"
          :label="SETTINGS_MESSAGES.outboundHostsLabel"
          for="outbound-policy-allow-hosts"
          :hint="SETTINGS_MESSAGES.outboundHostsHint"
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
        {{ SETTINGS_MESSAGES.outboundSave }}
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        variant="outline"
        :disabled="controlsDisabled"
        @click="reset"
      >
        {{ SETTINGS_MESSAGES.reset }}
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
import { SETTINGS_MESSAGES } from './pages/messages'
import { useAdminConfirm } from '../shared/composables/useAdminConfirm'
import { useOutboundPolicy } from './useOutboundPolicy'
import { useAdminOutboundPolicyFeedback } from './useSettingsFeedback'

const props = defineProps<{ class?: HTMLAttributes['class'] }>()

const feedback = useAdminOutboundPolicyFeedback()
const confirm = useAdminConfirm()
const { error, form, loading, refetch, reset, save, saving, setting, validationError } =
  useOutboundPolicy({ feedback, confirm })

const controlsDisabled = computed(() => loading.value || saving.value || Boolean(error.value))
const statusLabel = computed(() => {
  if (error.value) return SETTINGS_MESSAGES.providerLoadFailed
  if (!setting.value) return SETTINGS_MESSAGES.providerLoading
  const count = setting.value.allow_hosts.length
  return count ? SETTINGS_MESSAGES.outboundExtraAllowed(count) : SETTINGS_MESSAGES.outboundNoneExtra
})
const statusSeverity = computed(() => {
  if (error.value) return 'danger'
  if (!setting.value) return 'secondary'
  return setting.value.allow_hosts.length ? 'warn' : 'secondary'
})
</script>
