<template>
  <div class="flex w-full flex-col gap-3">
    <AdminAlert severity="secondary">
      {{ t('admin.billingAccount.platformAdminNote') }}
    </AdminAlert>

    <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
      <div class="flex min-w-0 flex-1 flex-wrap items-center gap-3">
        <div class="text-base font-bold text-foreground">
          {{ t('admin.billingAccount.title') }}
        </div>
        <div
          class="inline-flex items-center rounded-control border border-border bg-muted p-0.5"
          role="group"
          :aria-label="t('admin.billingAccount.scopeAria')"
        >
          <button
            v-for="opt in scopeOptions"
            :key="opt.value"
            type="button"
            class="rounded-control px-3 py-1 text-xs font-semibold transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer"
            :class="
              ownerScope === opt.value
                ? 'bg-card text-foreground shadow-raised'
                : 'text-muted-foreground hover:text-foreground'
            "
            @click="ownerScope = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>

      <div class="flex shrink-0 items-center gap-2">
        <ActionButton
          :label="t('admin.common.refresh')"
          variant="outline"
          :loading="isLoading || isFetching"
          :disabled="isLoading"
          @click="refreshAccounts"
        >
          <template #icon>
            <Refresh class="size-4 mr-1.5 text-muted-foreground" />
          </template>
        </ActionButton>
      </div>
    </div>

    <div class="overflow-hidden rounded-panel border border-border">
      <AdminTable
        :value="accounts"
        :loading="isLoading"
        data-key="billing_account_id"
        scrollable
        :table-aria-label="t('admin.billingAccount.tableAria')"
        :scroll-aria-label="t('admin.billingAccount.tableAria')"
        table-style="min-width: 780px"
      >
        <template #empty>
          <AdminEmptyState :title="t('admin.billingAccount.empty')" />
        </template>

        <AdminColumn :header="t('admin.billingAccount.accountColumn')" style="width: 220px">
          <template #body="{ data }">
            <AdminMono class="text-xs font-bold text-foreground">
              {{ data.billing_account_id }}
            </AdminMono>
          </template>
        </AdminColumn>

        <AdminColumn :header="t('admin.billingAccount.ownerColumn')" style="width: 140px">
          <template #body="{ data }">
            <AdminStatusPill
              :label="ownerLabel(data.owner_type)"
              :variant="data.owner_type === 'organization' ? 'warning' : 'success'"
              size="small"
            />
          </template>
        </AdminColumn>

        <AdminColumn :header="t('admin.billingAccount.linkageColumn')" style="min-width: 200px">
          <template #body="{ data }">
            <AdminMono
              class="block max-w-64 truncate text-xs text-muted-foreground"
              :title="data.workspace_id ?? data.organization_id ?? ''"
            >
              {{ data.workspace_id ?? data.organization_id ?? '' }}
            </AdminMono>
          </template>
        </AdminColumn>

        <AdminColumn :header="t('admin.billingAccount.createdColumn')" style="width: 170px">
          <template #body="{ data }">
            <AdminMono class="whitespace-nowrap text-xs tabular-nums text-muted-foreground">
              {{ formatAdminDateTimeWithSeconds(data.created_at) }}
            </AdminMono>
          </template>
        </AdminColumn>

        <AdminColumn :header="t('admin.common.actions')" style="width: 90px">
          <template #body="{ data }">
            <AdminButton
              :label="t('admin.billingAccount.view')"
              size="sm"
              variant="ghost"
              :aria-label="t('admin.billingAccount.viewAria', { id: data.billing_account_id })"
              @click="openAccount(data)"
            />
          </template>
        </AdminColumn>
      </AdminTable>
    </div>

    <div class="flex items-center justify-between">
      <span class="text-xs font-normal text-muted-foreground">
        {{ t('admin.billingAccount.footerCount', { count: accounts.length }) }}
      </span>
      <div class="flex items-center gap-2">
        <AdminButton
          :label="t('admin.billingAccount.prevPage')"
          size="sm"
          variant="ghost"
          :disabled="!canGoPrev || isFetching"
          @click="goPrev"
        />
        <AdminButton
          :label="t('admin.billingAccount.nextPage')"
          size="sm"
          variant="ghost"
          :disabled="!canGoNext || isFetching"
          @click="goNext"
        />
      </div>
    </div>
  </div>

  <AdminUserWalletDialog
    v-if="activeAccount"
    v-model:is-visible="isAccountDialogVisible"
    :target="{
      kind: 'billingAccount',
      id: activeAccount.billingAccountId,
      displayName: activeAccount.displayName,
    }"
    @refresh-users="refreshAccounts"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Refresh } from '@iconoir/vue'
import ActionButton from '../../../../../core/components/ActionButton.vue'
import { AdminAlert } from '../../../shared'
import { AdminButton } from '../../../shared'
import { AdminColumn } from '../../../shared'
import { AdminEmptyState } from '../../../shared'
import { AdminMono } from '../../../shared'
import { AdminStatusPill } from '../../../shared'
import { AdminTable } from '../../../shared'
import { formatAdminDateTimeWithSeconds } from '../../core/shared/format'
import {
  useAdminBillingAccountListResource,
  type AdminBillingAccountCriteria,
} from '../../session/accounts/resource'
import type { AdminBillingAccountListItem } from '../../session/shared/api'
import type { AdminBillingAccountOwnerScope } from '../../session/shared/api'
import AdminUserWalletDialog from '../wallets/AdminUserWalletDialog.vue'

const { t } = useI18n()

const ownerScope = ref<AdminBillingAccountOwnerScope>('all')
const cursorStack = ref<(string | undefined)[]>([undefined])
const cursorIndex = ref(0)
const criteria = computed<AdminBillingAccountCriteria>(() => ({
  ownerScope: ownerScope.value,
  ...(cursorStack.value[cursorIndex.value] === undefined
    ? {}
    : { cursor: cursorStack.value[cursorIndex.value] }),
}))

const resource = useAdminBillingAccountListResource(criteria)
const accounts = computed<AdminBillingAccountListItem[]>(() => resource.accounts.value)
const isLoading = computed(() => resource.query.isLoading.value)
const isFetching = computed(() => resource.query.isFetching.value)
const canGoNext = computed(() => resource.hasMore.value && resource.nextCursor.value != null)
const canGoPrev = computed(() => cursorIndex.value > 0)

watch(ownerScope, () => {
  cursorStack.value = [undefined]
  cursorIndex.value = 0
})

watch(
  () => resource.dataCriteria.value,
  (current) => {
    if (!current) return
    // Stale guard: a late response for a previous scope/cursor must not
    // append its cursor onto the fresh stack.
    if (current.ownerScope !== ownerScope.value) return
    const expected = cursorStack.value[cursorIndex.value]
    if ((current.cursor ?? undefined) !== (expected ?? undefined)) return
    if (
      resource.nextCursor.value != null &&
      cursorStack.value[cursorIndex.value + 1] === undefined
    ) {
      cursorStack.value = [
        ...cursorStack.value.slice(0, cursorIndex.value + 1),
        resource.nextCursor.value,
      ]
    }
  },
)

const scopeOptions = computed<Array<{ label: string; value: AdminBillingAccountOwnerScope }>>(
  () => [
    { label: t('admin.billingAccount.scopeAll'), value: 'all' },
    { label: t('admin.billingAccount.scopeOrganization'), value: 'organization' },
    { label: t('admin.billingAccount.scopeWorkspace'), value: 'workspace' },
  ],
)

function ownerLabel(ownerType: string) {
  return ownerType === 'organization'
    ? t('admin.billingAccount.scopeOrganization')
    : t('admin.wallet.ownerTypeWorkspace')
}

function refreshAccounts() {
  cursorStack.value = [undefined]
  cursorIndex.value = 0
  void resource.query.refetch()
}

function goNext() {
  if (!canGoNext.value) return
  cursorIndex.value += 1
}

function goPrev() {
  if (!canGoPrev.value) return
  cursorIndex.value -= 1
}

const activeAccount = ref<{ billingAccountId: string; displayName: string } | null>(null)
const isAccountDialogVisible = ref(false)

function openAccount(item: AdminBillingAccountListItem) {
  activeAccount.value = {
    billingAccountId: item.billing_account_id,
    displayName: item.billing_account_id,
  }
  isAccountDialogVisible.value = true
}
</script>
