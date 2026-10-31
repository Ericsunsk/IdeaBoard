/**
 * Admin shared seam — sole horizontal entry for `features/admin`.
 *
 * Verticals (`users`, `models`, `monitor`, `statistics`, `storage`,
 * `prompts`, `payments`, `settings`, `credits`) and `app` consume shared
 * symbols through this barrel. New code must import from here; the P4
 * bridge shims and P1 compatibility shims were deleted after consumers
 * migrated to the canonical paths (see `docs/frontend-seam-inventory.md`).
 *
 * Layout: `core` is pure (no vue/query/transport), `session` is
 * orchestration (composables, resources, adapters), `ui` is presentational
 * (SFCs plus their direct helpers). Shared asset URLs are exported explicitly.
 */

import storageAlibabaCloudLogo from './ui/assets/storage-vendors/alibabacloud.svg'
import storageAmazonS3Logo from './ui/assets/storage-vendors/amazons3.svg'
import storageCloudflareLogo from './ui/assets/storage-vendors/cloudflare.svg'
import storageMinioLogo from './ui/assets/storage-vendors/minio.svg'
import storageTencentCloudLogo from './ui/assets/storage-vendors/tencentcloud.ico'

// core — pure domain rules, contracts, and guidance (leaf, pure zero-IO logic)
export * from './core/controlCopy'
export * from './core/controlGuidance'
export * from './core/reconcileStableRows'
export * from './core/secretSchema'
export * from './core/serviceAdminMeta'

// adapters — infrastructure: REST mutation and resource adapters
export * from './adapters/controlMutation'
export * from './adapters/controlResource'

// ui/styling — UI style classes, select skin, theme tokens, task metric tokens
export * from './ui/styling/adminSelectSkin'
export * from './ui/styling/adminThemeTokens'
export * from './ui/styling/cellClasses'
export * from './ui/styling/cellPlaceholders'
export * from './ui/styling/taskMetricTokens'
export * from './ui/styling/uiClasses'

// ui/table — table layout and column width tokens
export * from './ui/table/tableColumns'

// session — orchestration: composables, resources, adapters, aggregation
export * from './session/anomalyRules'
export * from './session/brand/brandLogoResource'
export * from './session/brand/brandLogos'
export * from './session/feedback/confirmSettingsSave'
export * from './session/feedback/defineFeedback'
export * from './session/timeRange'
export * from './session/feedback/toast'
export * from './session/collection/useAdminCollectionQuery'
export * from './session/collection/useAdminCursorCollection'
export * from './session/useAdminSubHeaderTarget'
export * from './session/useAnomalyRadar'
export * from './session/brand/useBrandLogoOverrides'
export * from './session/collection/useCursorPagination'
export * from './session/collection/useOptimisticToggle'
export * from './session/collection/usePagePagination'
export * from './session/collection/useQueryPolling'
export * from './session/collection/useRefetchOnClose'

// ui — presentational helpers (SFCs follow below)
export * from './ui/popover/adminPopoverAtoms'
export * from './ui/table/adminTableAtoms'
export { default as AdminServiceCell } from './ui/table/AdminServiceCell.vue'
export * from './ui/tabs/adminTabsAtoms'
export * from './ui/chart/chart'
export {
  storageAlibabaCloudLogo,
  storageAmazonS3Logo,
  storageCloudflareLogo,
  storageMinioLogo,
  storageTencentCloudLogo,
}

// ui — components (default plus named contract types where the SFC declares
// them; SFCs without named exports expose only the default to keep the
// public surface exact)
export { default as AdminAlert } from './ui/feedback/AdminAlert.vue'
export * from './ui/feedback/AdminAlert.vue'
export { default as AdminBadge } from './ui/AdminBadge.vue'
export * from './ui/AdminBadge.vue'
export { default as AdminButton } from './ui/AdminButton.vue'
export * from './ui/AdminButton.vue'
export { default as AdminChartCard } from './ui/chart/AdminChartCard.vue'
export { default as AdminCollectionPageShell } from './ui/shell/AdminCollectionPageShell.vue'
export { default as AdminColumn } from './ui/AdminColumn.vue'
export { default as AdminDialogActions } from './ui/shell/AdminDialogActions.vue'
export { default as AdminDialogShell } from './ui/shell/AdminDialogShell.vue'
export * from './ui/shell/AdminDialogShell.vue'
export { default as AdminDrawer } from './ui/shell/AdminDrawer.vue'
export { default as AdminEditorActions } from './ui/shell/AdminEditorActions.vue'
export { default as AdminEmptyState } from './ui/feedback/AdminEmptyState.vue'
export { default as AdminFilterPopover } from './ui/toolbar/AdminFilterPopover.vue'
export { default as AdminFormField } from './ui/form/AdminFormField.vue'
export { default as AdminInput } from './ui/form/AdminInput.vue'
export { default as AdminLogoCropDialog } from './ui/shell/AdminLogoCropDialog.vue'
export { default as AdminLogoPickerPopover } from './ui/popover/AdminLogoPickerPopover.vue'
export { default as AdminMono } from './ui/AdminMono.vue'
export { default as AdminNumberInput } from './ui/form/AdminNumberInput.vue'
export { default as AdminOptionFilter } from './ui/toolbar/AdminOptionFilter.vue'
export * from './ui/toolbar/AdminOptionFilter.vue'
export { default as AdminPagerControls } from './ui/toolbar/AdminPagerControls.vue'
export { default as AdminPopover } from './ui/popover/AdminPopover.vue'
export { default as AdminPopoverContent } from './ui/popover/AdminPopoverContent.vue'
export { default as AdminPopoverTrigger } from './ui/popover/AdminPopoverTrigger.vue'
export { default as AdminRangedLineChartCard } from './ui/chart/AdminRangedLineChartCard.vue'
export { default as AdminRefreshButton } from './ui/toolbar/AdminRefreshButton.vue'
export { default as AdminResourceToolbar } from './ui/toolbar/AdminResourceToolbar.vue'
export { default as AdminRowActionsPopover } from './ui/toolbar/AdminRowActionsPopover.vue'
export * from './ui/toolbar/AdminRowActionsPopover.vue'
export { default as AdminSelect } from './ui/form/AdminSelect.vue'
export * from './ui/form/AdminSelect.vue'
export { default as AdminSettingsDialogFrame } from './ui/shell/AdminSettingsDialogFrame.vue'
export { default as AdminSpinner } from './ui/AdminSpinner.vue'
export { default as StatusBadge } from '@/core/components/StatusBadge.vue'
export type { AppStatusTone } from '@/core/components/StatusBadge.vue'
export { default as AdminSubNavigationLayout } from './ui/shell/AdminSubNavigationLayout.vue'
export * from './ui/shell/AdminSubNavigationLayout.vue'
export { default as AdminSummaryCard } from './ui/shell/AdminSummaryCard.vue'
export { default as AdminSwitch } from './ui/form/AdminSwitch.vue'
export { default as AdminTable } from './ui/table/AdminTable.vue'
export * from './ui/table/AdminTable.vue'
export { default as AdminTablePanel } from './ui/table/AdminTablePanel.vue'
export { default as AdminTextarea } from './ui/form/AdminTextarea.vue'
export { default as AdminToolbarSearch } from './ui/toolbar/AdminToolbarSearch.vue'
export { default as SwitchRow } from './ui/form/SwitchRow.vue'
export * from './ui/form/SwitchRow.vue'
